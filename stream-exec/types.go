package streamexec

import "encoding/json"

type Result struct {
	Params    Params
	Stderr    string
	Stdout    string
	ExitCode  int
	Succeeded bool
}

func (r Result) String() string {
	d, _ := json.Marshal((r))
	return string(d)
}

func (r Result) Error() string {
	d, _ := json.Marshal((r))
	return string(d)
}
