package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	streamexec "github.com/davidporter-id-au/stream-exec/stream-exec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startBackground launches stream-exec in the background, returning the
// *exec.Cmd and a function to wait for it to exit.
func startBackground(t *testing.T, stdin string, args ...string) (*exec.Cmd, func() result) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Stdin = strings.NewReader(stdin)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}
	wait := func() result {
		err := cmd.Wait()
		code := 0
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				code = exit.ExitCode()
			}
		}
		return result{exitCode: code}
	}
	return cmd, wait
}

// waitForSocket polls until the socket file for pid exists, or times out.
func waitForSocket(t *testing.T, pid int) {
	t.Helper()
	path := streamexec.SocketPath(pid)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("socket %s did not appear within timeout", path)
}

// TestIPCStatus verifies that a running process responds to a status query.
func TestIPCStatus(t *testing.T) {
	// Use a slow command so the process is still alive when we query it
	const count = 5
	var lines []string
	for i := 0; i < count; i++ {
		lines = append(lines, fmt.Sprintf(`{"i":%d}`, i))
	}

	cmd, wait := startBackground(t,
		strings.Join(lines, "\n"),
		"-exec", "sleep 0.3 && echo $i",
		"-concurrency", "2",
	)

	waitForSocket(t, cmd.Process.Pid)

	sock := streamexec.SocketPath(cmd.Process.Pid)
	resp, err := streamexec.QuerySocket(sock, "status")
	require.NoError(t, err)
	require.True(t, resp.OK)
	require.NotNil(t, resp.Status)

	assert.Equal(t, cmd.Process.Pid, resp.Status.PID)
	assert.Contains(t, resp.Status.ExecString, "sleep 0.3")
	assert.False(t, resp.Status.StartTime.IsZero())

	wait()
}

// TestIPCStop verifies that --stop causes the target process to exit cleanly.
// Uses long-running commands (sleep 10) so that without context cancellation
// the process would take minutes. With exec.CommandContext, cancel() kills
// in-flight bash processes immediately.
func TestIPCStop(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&sb, `{"i":%d}`+"\n", i)
	}

	cmd, wait := startBackground(t,
		sb.String(),
		"-exec", "sleep 10",
		"-concurrency", "3",
		"-continue",
	)

	waitForSocket(t, cmd.Process.Pid)

	stopOut, err := exec.Command(binaryPath, "-stop", fmt.Sprintf("%d", cmd.Process.Pid)).CombinedOutput()
	require.NoError(t, err, "stop command failed: %s", stopOut)
	assert.Contains(t, string(stopOut), "sent stop signal")

	done := make(chan result, 1)
	go func() { done <- wait() }()

	select {
	case <-done:
		// exited — exit code may be non-zero because killed execs look like failures
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("process did not exit after stop signal within timeout")
	}
}

// TestIPCList verifies that --list finds a running process and displays it.
func TestIPCList(t *testing.T) {
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines, fmt.Sprintf(`{"i":%d}`, i))
	}

	cmd, wait := startBackground(t,
		strings.Join(lines, "\n"),
		"-exec", "sleep 0.3 && echo $i",
		"-concurrency", "2",
	)
	defer wait()

	waitForSocket(t, cmd.Process.Pid)

	listOut, err := exec.Command(binaryPath, "-list").CombinedOutput()
	require.NoError(t, err)

	output := string(listOut)
	assert.Contains(t, output, fmt.Sprintf("%d", cmd.Process.Pid))
	assert.Contains(t, output, "sleep 0.3")

	wait()
}

// TestIPCStaleSocketCleaned verifies that a stale socket (from a crashed process)
// is silently removed when --list encounters it.
func TestIPCStaleSocketCleaned(t *testing.T) {
	// Write a fake socket file that nothing is listening on
	require.NoError(t, os.MkdirAll(streamexec.SocketDir(), 0755))
	stalePath := filepath.Join(streamexec.SocketDir(), "999999.sock")
	f, err := os.Create(stalePath)
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(stalePath) })

	// --list should not error; it should skip/remove the stale socket
	out, err := exec.Command(binaryPath, "-list").CombinedOutput()
	assert.NoError(t, err, "output: %s", out)

	// The stale file should have been removed
	_, statErr := os.Stat(stalePath)
	assert.True(t, os.IsNotExist(statErr), "stale socket should have been removed")
}

// TestIPCStatusJSON verifies the raw JSON shape of the status response.
func TestIPCStatusJSON(t *testing.T) {
	var lines []string
	for i := 0; i < 3; i++ {
		lines = append(lines, fmt.Sprintf(`{"i":%d}`, i))
	}

	cmd, wait := startBackground(t,
		strings.Join(lines, "\n"),
		"-exec", "sleep 0.3 && echo $i",
		"-concurrency", "2",
	)
	defer wait()

	waitForSocket(t, cmd.Process.Pid)

	resp, err := streamexec.QuerySocket(streamexec.SocketPath(cmd.Process.Pid), "status")
	require.NoError(t, err)
	require.True(t, resp.OK)

	b, err := json.Marshal(resp.Status)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(b, &raw))

	for _, field := range []string{"pid", "start_time", "exec_string", "processed", "failed", "in_flight"} {
		assert.Contains(t, raw, field, "status JSON missing field %q", field)
	}

	wait()
}
