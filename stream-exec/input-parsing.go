package streamexec

import (
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
)

type streams struct {
	input  io.ReadCloser
	output io.WriteCloser
	err    io.WriteCloser
}

type StreamExec struct {
	streams     streams
	errors      chan error
	incoming    chan string
	readWG      sync.WaitGroup
	writeWG     sync.WaitGroup
	options     Options
	moreContent bool
}

func New(inputstream io.ReadCloser, outputstream io.WriteCloser, errStream io.WriteCloser, o Options) *StreamExec {
	incomingBuffer := make(chan string, o.IncomingBufferSize)
	errChan := make(chan error)
	var wg sync.WaitGroup

	if o.Concurrency == 0 {
		o.Concurrency = defaultConcurrency
	}
	if o.IncomingBufferSize == 0 {
		o.IncomingBufferSize = defaultInputByteLen
	}

	return &StreamExec{
		streams: streams{
			input:  inputstream,
			output: outputstream,
			err:    errStream,
		},
		readWG:      wg,
		errors:      errChan,
		incoming:    incomingBuffer,
		options:     o,
		moreContent: true,
	}
}

func (j *StreamExec) Run() error {

	j.readWG.Add(1)
	go j.readInput(j.streams.input)
	go j.handleErrors()

	for i := 0; i < j.options.Concurrency; i++ {
		j.writeWG.Add(1)
		go j.process(i)
	}

	j.readWG.Wait()
	j.writeWG.Wait()
	j.drain()
	close(j.errors)
	return nil
}

// takes a block of data and joins it from the incoming datastream
func (s *StreamExec) process(i int) {
	for {
		if !s.moreContent {
			break
		}
		line := <-s.incoming
		if len(line) == 0 {
			// channel is likely closed, nil data
			// so sust hang tight and loop again in sec
			// to check if the content's done
			continue
		}
		envvars, err := formatEnvString(line)
		if err != nil {
			s.errors <- fmt.Errorf("%v, original data: %q", err, line)
			continue
		}
		result, err := s.exec(envvars)
		if err != nil {
			s.errors <- fmt.Errorf("%v, original data: %q", err, line)
			continue
		}
		err = s.writeOutResult(*result, line)
		if err != nil {
			s.errors <- err
			continue
		}
	}
	s.writeWG.Done()
}

func (s *StreamExec) drain() {
	for i := 0; i < len(s.incoming); i++ {
		line := <-s.incoming

		envvars, err := formatEnvString(line)
		if err != nil {
			s.errors <- fmt.Errorf("%v, original data: %q", err, line)
			continue
		}
		s.exec(envvars)
	}
}

func (j *StreamExec) writeOutResult(res Result, leftRow string) error {
	fmt.Fprintf(j.streams.output, "%v\n", res.String())
	return nil
}

func (j StreamExec) debugPrint(debugMsg string, fmtStr string, args ...interface{}) {
	if j.options.OutputDebugMode {
		// todo either use a real logging framework
		// or use string builder properly
		j.streams.err.Write([]byte(fmt.Sprintf("\033[33m"+debugMsg+"\033[0m "+fmtStr, args...)))
	}
}

func (s StreamExec) handleErrors() {
	for {
		err := <-s.errors
		if err == nil {
			break // closing & cleaning up
		}
		if !s.options.ContinueOnErr {
			log.Fatalf("Fatal error: %v", err)
		} else {
			s.streams.err.Write([]byte(fmt.Sprintf("\033[31mError:\033[0m '%v' \n", err.Error())))
		}
	}
}

// streams the input
func (j *StreamExec) readInput(inputStream io.ReadCloser) error {
	var d = make([]byte, defaultInputByteLen)
	var remainder string
	defer inputStream.Close()
	for {
		n, err := inputStream.Read(d)
		if io.EOF == err {
			if remainder != "" {
				for _, line := range strings.Split(remainder, "\n") {
					j.incoming <- line
				}
			}
			break
		}
		if err != nil {
			panic(err)
		}

		data, newRemainder := splitInputBytes(remainder, d[:n])
		remainder = newRemainder
		for _, line := range data {
			j.incoming <- line
		}
	}
	close(j.incoming)
	j.moreContent = false
	j.readWG.Done()
	return nil
}

// finds the last newline and separates it out since we can't
// use a half-written line
func splitInputBytes(prevRemainder string, data []byte) ([]string, string) {
	dataString := string(data)
	cleanBlockIdx := strings.LastIndex(dataString, "\n")
	if cleanBlockIdx < 0 {
		return nil, dataString
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
