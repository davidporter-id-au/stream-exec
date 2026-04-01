package integration

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

var binaryPath string

func TestMain(m *testing.M) {
	tmp, err := os.CreateTemp("", "stream-exec-*")
	if err != nil {
		panic(err)
	}
	tmp.Close()
	binaryPath = tmp.Name()

	build := exec.Command("go", "build", "-o", binaryPath, "../")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("failed to build stream-exec: " + err.Error())
	}

	code := m.Run()
	os.Remove(binaryPath)
	os.Exit(code)
}

type result struct {
	stdout   string
	stderr   string
	exitCode int
}

// run executes the binary's "run" subcommand with the given args and stdin content.
func run(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binaryPath, append([]string{"run"}, args...)...)
	cmd.Stdin = bytes.NewBufferString(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatalf("unexpected error running binary: %v", err)
		}
	}
	return result{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: code,
	}
}
