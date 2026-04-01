package streamexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type streamgroup struct {
	output io.WriteCloser
	err    io.WriteCloser
}

type streams struct {
	input      io.ReadCloser
	text       streamgroup
	structured streamgroup
}

type StreamExec struct {
	processed int64
	failed    int64
	inFlight  int64

	streams    streams
	errors     chan error
	incoming   chan string
	readWG     sync.WaitGroup
	writeWG    sync.WaitGroup
	errWG      sync.WaitGroup
	options    Options
	startTime  time.Time
	cancel     context.CancelFunc
	ipcCleanup func()
}

func New(inputstream io.ReadCloser, outputstream io.WriteCloser, errStream io.WriteCloser, o Options) *StreamExec {
	if o.Concurrency == 0 {
		o.Concurrency = defaultConcurrency
	}
	if o.IncomingBufferSize == 0 {
		o.IncomingBufferSize = defaultInputByteLen
	}

	incomingBuffer := make(chan string, o.IncomingBufferSize)
	errChan := make(chan error)
	var wg sync.WaitGroup

	var outputFile io.WriteCloser
	if o.OutputLog != "" {
		f, err := os.OpenFile(o.OutputLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("attempted to open a file for output logging, but couldn't: %v", err)
		}
		outputFile = f
	}

	var errFile io.WriteCloser
	if o.ErrorLog != "" {
		e, err := os.OpenFile(o.ErrorLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("attempted to open a file for error logging, but couldn't: %v", err)
		}
		errFile = e
	}

	return &StreamExec{
		streams: streams{
			input: inputstream,
			text: streamgroup{
				output: outputstream,
				err:    errStream,
			},
			structured: streamgroup{
				output: outputFile,
				err:    errFile,
			},
		},
		readWG:   wg,
		errors:   errChan,
		incoming: incomingBuffer,
		options:  o,
	}
}

func (s *StreamExec) Run() error {
	s.startTime = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	ipcCleanup, err := s.startIPCServer(cancel)
	if err != nil {
		log.Printf("warning: IPC server unavailable: %v", err)
	} else {
		s.ipcCleanup = ipcCleanup
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	defer signal.Stop(sigCh)

	defer s.closeAll()
	s.readWG.Add(1)
	go s.readInput(ctx, s.streams.input)

	s.errWG.Add(1)
	go s.handleErrors()

	for i := 0; i < s.options.Concurrency; i++ {
		s.writeWG.Add(1)
		go s.process(ctx, i)
	}

	s.readWG.Wait()
	s.writeWG.Wait()
	s.drain(ctx)
	s.errWG.Wait()
	return nil
}

// takes a block of data and joins it from the incoming datastream
func (s *StreamExec) process(ctx context.Context, i int) {
	for {
		// Prioritised cancellation check: if ctx is already done, exit
		// immediately without picking up another item.
		select {
		case <-ctx.Done():
			s.writeWG.Done()
			return
		default:
		}

		// Block until a line arrives or cancellation fires.
		select {
		case <-ctx.Done():
			s.writeWG.Done()
			return
		case line, ok := <-s.incoming:
			if !ok {
				s.writeWG.Done()
				return
			}
			if line == "" {
				continue
			}
			envvars, err := formatEnvString(line)
			if err != nil {
				s.errors <- fmt.Errorf("%v, original data: %q", err, line)
				continue
			}
			atomic.AddInt64(&s.inFlight, 1)
			result, err := s.exec(ctx, envvars)
			atomic.AddInt64(&s.inFlight, -1)
			if err != nil {
				atomic.AddInt64(&s.failed, 1)
				s.errors <- err
				continue
			}
			if result == nil {
				continue
			}
			atomic.AddInt64(&s.processed, 1)
			err = s.writeOutput(*result)
			if err != nil {
				s.errors <- err
			}
		}
	}
}

func (s *StreamExec) drain(ctx context.Context) {
	for i := 0; i < len(s.incoming); i++ {
		if ctx.Err() != nil {
			break
		}
		line := <-s.incoming
		if line == "" {
			continue
		}
		envvars, err := formatEnvString(line)
		if err != nil {
			s.errors <- err
			continue
		}
		atomic.AddInt64(&s.inFlight, 1)
		res, err := s.exec(ctx, envvars)
		atomic.AddInt64(&s.inFlight, -1)
		if err != nil {
			atomic.AddInt64(&s.failed, 1)
			s.errors <- err
			continue
		}
		if res == nil {
			continue
		}
		atomic.AddInt64(&s.processed, 1)
		err = s.writeOutput(*res)
		if err != nil {
			s.errors <- err
		}
	}
	close(s.errors)
}

func (s *StreamExec) writeOutput(res Result) error {
	if s.streams.structured.output != nil {
		s.streams.structured.output.Write([]byte(fmt.Sprintf("%v\n", res.Structured())))
	}
	if s.streams.text.output != nil {
		s.streams.text.output.Write([]byte(fmt.Sprintf("%v", res.Text(s.options.DebugMode))))
	}
	return nil
}

func (s *StreamExec) debugPrint(debugMsg string) {
	if s.options.DebugMode {
		// stdout
		s.streams.text.output.Write([]byte(fmt.Sprintf("%s\n", debugMsg)))
	}
}

func (s *StreamExec) writeErrors(err error) {
	var result *Result
	if s.streams.structured.err != nil {
		if errors.As(err, &result) {
			s.streams.structured.err.Write([]byte(fmt.Sprintf("%v\n", result.Structured())))
		} else {
			s.streams.structured.err.Write([]byte(fmt.Sprintf("%v\n", err.Error())))
		}
	}
	if s.streams.text.err != nil {
		var result *Result
		if errors.As(err, &result) {
			s.streams.text.err.Write([]byte(fmt.Sprintf("%v\n", result.Text(s.options.DebugMode))))
		} else {
			s.streams.text.err.Write([]byte(fmt.Sprintf("%v\n", err.Error())))
		}
	} else {
		log.Println("Warning.... unhandled errors")
	}
}

func (s *StreamExec) handleErrors() {
	for {
		err := <-s.errors
		if err == nil {
			s.errWG.Done()
			break // closing & cleaning up
		}
		s.writeErrors(err)
		if !s.options.ContinueOnErr {
			s.closeAll()
			os.Exit(1)
		}
	}
}

func (s *StreamExec) closeAll() {
	if s.ipcCleanup != nil {
		s.ipcCleanup()
		s.ipcCleanup = nil
	}
	if s.streams.structured.err != nil {
		s.streams.structured.err.Close()
	}
	if s.streams.structured.output != nil {
		s.streams.structured.output.Close()
	}
	if s.streams.text.err != nil {
		s.streams.text.err.Close()
	}
	if s.streams.text.output != nil {
		s.streams.text.output.Close()
	}
}

// streams the input
func (s *StreamExec) readInput(ctx context.Context, inputStream io.ReadCloser) error {
	var closeOnce sync.Once
	closeStream := func() { closeOnce.Do(func() { inputStream.Close() }) }

	// Unblock a pending Read when the context is cancelled (e.g. --stop).
	go func() {
		<-ctx.Done()
		closeStream()
	}()
	defer closeStream()

	// send forwards a line to the incoming channel but returns false and
	// aborts if the context is cancelled while the channel is full.
	send := func(line string) bool {
		select {
		case s.incoming <- line:
			return true
		case <-ctx.Done():
			return false
		}
	}

	var d = make([]byte, defaultInputByteLen)
	var remainder string
loop:
	for {
		n, err := inputStream.Read(d)
		if err == io.EOF {
			if remainder != "" {
				for _, line := range strings.Split(remainder, "\n") {
					if !send(line) {
						break loop
					}
				}
			}
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				break // clean stop — context was cancelled
			}
			panic(err)
		}

		data, newRemainder := splitInputBytes(remainder, d[:n])
		remainder = newRemainder
		for _, line := range data {
			if !send(line) {
				break loop
			}
		}
	}
	close(s.incoming)
	s.readWG.Done()
	return nil
}

// finds the last newline and separates it out since we can't
// use a half-written line
func splitInputBytes(prevRemainder string, data []byte) ([]string, string) {
	dataString := string(data)
	cleanBlockIdx := strings.LastIndex(dataString, "\n")
	if cleanBlockIdx < 0 {
		return nil, prevRemainder + dataString
	}
	// a clean block is a block of text which
	// finishes with newline, it may or may not
	// start partway thorough an existing line
	cleanBlock := dataString[0:cleanBlockIdx]

	nextRemainder := dataString[cleanBlockIdx:]
	out := strings.Split(prevRemainder+cleanBlock, "\n")

	// this will have a leading newline, so remove it
	nextRemainder = strings.Replace(nextRemainder, "\n", "", 1)

	// remove whitespace on lines while are finished
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out, nextRemainder
}
