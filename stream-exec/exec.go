package streamexec

import (
	"errors"
	"os/exec"
)

func (s *StreamExec) exec(envvars []string) (*Result, error) {
	cmd := exec.Command("bash", "-c", s.options.Params.ExecString)
	cmd.Env = envvars
	stdout, err := cmd.CombinedOutput()

	if err != nil {
		var e *exec.ExitError
		if errors.As(err, &e) {
			code := e.ProcessState.ExitCode()
			return nil, &Result{
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
		Params:    s.options.Params,
		Stdout:    string(stdout),
		ExitCode:  0,
		Succeeded: true,
	}, nil
}
