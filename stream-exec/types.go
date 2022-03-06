package streamexec

import (
	"encoding/json"
	"fmt"
	"strings"
)

const gray = "\033[0;37m"
const darkGray = "\033[1;30m"
const green = "\033[0;32m"
const red = "\033[0;31m"
const nc = "\033[0m"

const greenTick = "\033[0;32\u2714\033[0m"
const redCross = "\u274c"

type Result struct {
	Envvars   []string `json:",omitempty"`
	Params    Params   `json:",omitempty"`
	Stderr    string   `json:",omitempty"`
	Stdout    string   `json:",omitempty"`
	ExitCode  int      `json:",omitempty"`
	Succeeded bool
}

func (r Result) Text(debug bool) string {
	var out string
	stdout := strings.TrimRight(r.Stdout, "\n")
	stderr := strings.TrimRight(r.Stderr, "\n")
	if debug {
		out += fmt.Sprintf("with envvars: %v\n", r.Envvars)
		out += fmt.Sprintf("with params: %v\n", r.Params)
	}
	if r.Succeeded {
		out += fmt.Sprintf("%s - %v\n", greenTick, stdout)
		if stderr != "" {
			out += fmt.Sprintf("%s%s%s\n", red, stderr, nc)
		}
	} else {
		out += fmt.Sprintf("%s - %s\n", redCross, stdout)
		if stderr != "" {
			out += fmt.Sprintf("%s%s%s\n", red, stderr, nc)
		}
		out += fmt.Sprintf("%sexit code: %d%s\n", darkGray, r.ExitCode, nc)
	}
	return out
}

func (r Result) Structured() string {
	d, _ := json.Marshal((r))
	return string(d)
}

func (r Result) Error() string {
	d, _ := json.Marshal((r))
	return string(d)
}
