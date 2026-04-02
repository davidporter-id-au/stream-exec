package streamexec

import (
	"context"
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

	"golang.org/x/time/rate"
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
	processed          int64
	failed             int64
	inFlight           int64
	currentConcurrency int64

	streams     streams
	errors      chan error
	incoming    chan string
	scaleDn     chan struct{}
	rateLimiter *rate.Limiter // nil when RPS is unlimited
	readWG      sync.WaitGroup
	writeWG     sync.WaitGroup
	errWG       sync.WaitGroup
	options     Options
	startTime   time.Time
	ctx         context.Context
	cancel      context.CancelFunc
	ipcCleanup  func()
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

	return &StreamExec{
		streams: streams{
			input: inputstream,
			text: streamgroup{
				output: outputstream,
				err:    errStream,
			},
			structured: streamgroup{
				output: outputFile,
			},
		},
		readWG:   wg,
		errors:   errChan,
		incoming: incomingBuffer,
		scaleDn:  make(chan struct{}, 1024),
		options:  o,
	}
}

func (s *StreamExec) Run() error {
	s.startTime = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
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

	if s.options.RPS > 0 {
		s.rateLimiter = rate.NewLimiter(rate.Limit(s.options.RPS), 1)
	}

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
	atomic.AddInt64(&s.currentConcurrency, 1)
	defer func() {
		atomic.AddInt64(&s.currentConcurrency, -1)
		s.writeWG.Done()
	}()

	for {
		// Prioritised check: honour cancellation or a scale-down signal
		// before picking up another item.
		select {
		case <-ctx.Done():
			return
		case <-s.scaleDn:
			return
		default:
		}

		// Block until a line arrives, cancellation fires, or we're scaled down.
		select {
		case <-ctx.Done():
			return
		case <-s.scaleDn:
			return
		case line, ok := <-s.incoming:
			if !ok {
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
			if s.rateLimiter != nil {
				if err := s.rateLimiter.Wait(ctx); err != nil {
					return // context cancelled
				}
			}
			atomic.AddInt64(&s.inFlight, 1)
			resultErr := s.exec(ctx, envvars)
			atomic.AddInt64(&s.inFlight, -1)
			if resultErr == nil {
				continue
			}
			if !resultErr.Succeeded {
				atomic.AddInt64(&s.failed, 1)
				s.errors <- resultErr
			} else {
				atomic.AddInt64(&s.processed, 1)
			}
			err = s.writeOutput(*resultErr)
			if err != nil {
				s.errors <- err
			}
		}
	}
}

// SetConcurrency adjusts the number of active worker goroutines.
// Safe to call from any goroutine while Run() is executing.
func (s *StreamExec) SetConcurrency(n int) {
	if n <= 0 || s.ctx == nil || s.ctx.Err() != nil {
		return
	}
	current := int(atomic.LoadInt64(&s.currentConcurrency))
	if n > current {
		for i := 0; i < n-current; i++ {
			s.writeWG.Add(1)
			go s.process(s.ctx, current+i)
		}
	} else if n < current {
		for i := 0; i < current-n; i++ {
			s.scaleDn <- struct{}{}
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
		resultErr := s.exec(ctx, envvars)
		atomic.AddInt64(&s.inFlight, -1)
		if resultErr == nil {
			continue
		}
		if !resultErr.Succeeded {
			atomic.AddInt64(&s.failed, 1)
			s.errors <- resultErr
		} else {
			atomic.AddInt64(&s.processed, 1)
		}
		err = s.writeOutput(*resultErr)
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
	if res.Succeeded {
		s.streams.text.output.Write([]byte(fmt.Sprintf("%v", res.Text(s.options.DebugMode))))
	} else {
		// write to sterr
		s.streams.text.err.Write([]byte(fmt.Sprintf("%v\n", res.Error())))
	}
	return nil
}

func (s *StreamExec) debugPrint(debugMsg string) {
	if s.options.DebugMode {
		// stdout
		s.streams.text.output.Write([]byte(fmt.Sprintf("%s\n", debugMsg)))
	}
}

func (s *StreamExec) handleErrors() {
	for {
		err := <-s.errors
		if err == nil {
			s.errWG.Done()
			break // closing & cleaning up
		}
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
