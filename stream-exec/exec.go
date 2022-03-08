package streamexec

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"time"
)

func (s *StreamExec) exec(envvars []string) (*Result, error) {

	if s.options.DryRun {
		log.Printf("Dry-run: bash -c '%s'\n", s.options.Params.ExecString)
		log.Printf("with envvars: %v", envvars)
		return nil, nil
	}

	stdout, err := execWithRetries(s.options.Params.Retries, func() ([]byte, error) {
		cmd := exec.Command("bash", "-c", s.options.Params.ExecString)
		cmd.Env = append(cmd.Env, envvars...)
		return cmd.CombinedOutput()
	}, s.debugPrint,
		time.Second) // todo, make this configurable

	if err != nil {
		var e *exec.ExitError
		if errors.As(err, &e) {
			code := e.ProcessState.ExitCode()
			return nil, &Result{
				Envvars:   envvars,
				Params:    s.options.Params,
				Stderr:    string(e.Stderr),
				Stdout:    string(stdout),
				ExitCode:  code,
				Succeeded: false,
			}
		} else {
			return nil, err
		}
	}
	return &Result{
		Envvars:   envvars,
		Params:    s.options.Params,
		Stdout:    string(stdout),
		ExitCode:  0,
		Succeeded: true,
	}, nil
}

// simple retry mechanism with exponential backoff
func execWithRetries(retries int, f func() ([]byte, error), debugPrintFn func(string), sleepTime time.Duration) ([]byte, error) {
	retryLen := 0
	var lastStdout []byte
	var lastErr error
	for i := 0; i <= retries; i++ {
		res, err := f()
		lastErr = err
		lastStdout = res
		if err == nil {
			// we're done, complete
			return res, nil
		}
		debugPrintFn(fmt.Sprintf("retry attempt %d", i))
		time.Sleep(1 + time.Duration(retryLen)*sleepTime)
		retryLen = retryLen * retryLen
	}
	return lastStdout, lastErr
}
