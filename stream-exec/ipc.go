package streamexec

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// StatusResponse is the payload returned by the IPC status command.
// It is also used by the --list client to display running instances.
type StatusResponse struct {
	PID         int       `json:"pid"`
	StartTime   time.Time `json:"start_time"`
	ExecString  string    `json:"exec_string"`
	Processed   int64     `json:"processed"`
	Failed      int64     `json:"failed"`
	InFlight    int64     `json:"in_flight"`
	Concurrency int64     `json:"concurrency"`
}

// IPCResponse is the envelope returned for every IPC request.
type IPCResponse struct {
	OK     bool            `json:"ok"`
	Status *StatusResponse `json:"status,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type ipcRequest struct {
	Cmd   string `json:"cmd"`   // "status" | "stop" | "set-concurrency"
	Value int    `json:"value"` // used by set-concurrency
}

// SocketDir returns the directory that holds per-process Unix sockets.
func SocketDir() string {
	return filepath.Join(os.TempDir(), "stream-exec")
}

// SocketPath returns the socket path for the given PID.
func SocketPath(pid int) string {
	return filepath.Join(SocketDir(), fmt.Sprintf("%d.sock", pid))
}

func (s *StreamExec) startIPCServer(stopFn func()) (cleanup func(), err error) {
	path := SocketPath(os.Getpid())
	if err := os.MkdirAll(SocketDir(), 0755); err != nil {
		return nil, fmt.Errorf("creating socket dir: %w", err)
	}
	os.Remove(path) // clear any stale socket from a previous crash

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listening on unix socket %s: %w", path, err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener was closed
			}
			go s.handleIPCConn(conn, stopFn)
		}
	}()

	return func() {
		ln.Close()
		os.Remove(path)
	}, nil
}

func (s *StreamExec) handleIPCConn(conn net.Conn, stopFn func()) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}
	var req ipcRequest
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		writeIPCResponse(conn, IPCResponse{OK: false, Error: "invalid JSON request"})
		return
	}
	switch req.Cmd {
	case "status":
		st := s.currentStatus()
		writeIPCResponse(conn, IPCResponse{OK: true, Status: &st})
	case "stop":
		writeIPCResponse(conn, IPCResponse{OK: true})
		stopFn()
	case "set-concurrency":
		if req.Value <= 0 {
			writeIPCResponse(conn, IPCResponse{OK: false, Error: "concurrency must be > 0"})
			return
		}
		s.SetConcurrency(req.Value)
		st := s.currentStatus()
		writeIPCResponse(conn, IPCResponse{OK: true, Status: &st})
	default:
		writeIPCResponse(conn, IPCResponse{OK: false, Error: "unknown command: " + req.Cmd})
	}
}

func writeIPCResponse(conn net.Conn, r IPCResponse) {
	b, _ := json.Marshal(r)
	conn.Write(append(b, '\n'))
}

func (s *StreamExec) currentStatus() StatusResponse {
	return StatusResponse{
		PID:         os.Getpid(),
		StartTime:   s.startTime,
		ExecString:  s.options.Params.ExecString,
		Processed:   atomic.LoadInt64(&s.processed),
		Failed:      atomic.LoadInt64(&s.failed),
		InFlight:    atomic.LoadInt64(&s.inFlight),
		Concurrency: atomic.LoadInt64(&s.currentConcurrency),
	}
}

// QuerySocket sends a request to the socket at path and returns the response.
// The optional value parameter is forwarded as ipcRequest.Value (used by set-concurrency).
// Returns an error if the process is gone or the socket is stale.
func QuerySocket(path string, cmd string, value ...int) (*IPCResponse, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	r := ipcRequest{Cmd: cmd}
	if len(value) > 0 {
		r.Value = value[0]
	}
	req, _ := json.Marshal(r)
	conn.Write(append(req, '\n'))

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return nil, fmt.Errorf("no response from socket")
	}
	var resp IPCResponse
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	return &resp, nil
}
