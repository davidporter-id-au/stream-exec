package streamexec

import "encoding/json"

type Result struct {
	Envvars []string `json:",omitempty"`
	Params    Params `json:",omitempty"`
	Stderr    string `json:",omitempty"`
	Stdout    string `json:",omitempty"`
	ExitCode  int    `json:",omitempty"`
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
