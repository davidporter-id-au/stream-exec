package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"

	streamexec "github.com/davidporter-id-au/stream-exec/stream-exec"
)

const (
	_flagExecCmd          = "exec"
	_flagConcurrency      = "concurrency"
	_flagRetries          = "retries"
	_flagContinue         = "continue"
	_flagDryRun           = "dry-run"
	_flagDebug            = "debug"
	_flagOutputLogPath    = "output-log-path"
	_flagInputFile        = "input-json-file"
	_flagRPS              = "rps"
	_flagPID              = "pid"
	_flagSetConcurrency   = "concurrency"
	_ipcCmdStatus         = "status"
	_ipcCmdStop           = "stop"
	_ipcCmdSetConcurrency = "set-concurrency"
)

func main() {
	app := &cli.App{
		Name:  "stream-exec",
		Usage: "execute a command for each JSON line read from stdin",
		Commands: []*cli.Command{
			cmdRun(),
			cmdList(),
			cmdSignal(),
		},
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func cmdRun() *cli.Command {
	return &cli.Command{
		Name:      "run",
		Usage:     "read JSON lines from stdin and execute a command for each",
		ArgsUsage: " ", // stdin is the input
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    _flagExecCmd,
				Aliases: []string{"x", "exec-command"},
				Usage: `bash command to run for each record (required). 
JSON variables for each line of input will be available as their key name. For example, given an input of JSON lines:

{"user": "alice"} 
{"user": "bob"} 

will make the envvar '$user' available and in the first instance, it'll be set to 'alice', then 'bob'. 

Therefore, for a quick way to download all the user info from an API: 

The command -x 'curl http://somesite/$user' should output each line to stdout.

Keys are normalised to make them safe for use in shell, so the key 'a-b' is available in shell as 'a_b'. All non-alphanumerics are replaced with underscore.`,
				Required: true,
			},
			&cli.IntFlag{
				Name:    _flagConcurrency,
				Aliases: []string{"c"},
				Usage:   "How many bash commands to run concurrently.",
				Value:   1,
			},
			&cli.IntFlag{
				Name:    _flagRetries,
				Aliases: []string{"r"},
				Usage:   "number of times to retry a failed command (if the command exit-codes is not zero)",
				Value:   0,
			},
			&cli.BoolFlag{
				Name:    _flagContinue,
				Aliases: []string{"k"},
				Usage:   "continue processing after a failure (if the bash command exits with a non-zero exit code)",
			},
			&cli.BoolFlag{
				Name:  _flagDryRun,
				Usage: "print what would run without executing",
			},
			&cli.BoolFlag{
				Name:  _flagDebug,
				Usage: "print env vars and parameters for each execution",
			},
			&cli.StringFlag{
				Name:  _flagOutputLogPath,
				Usage: "write successful results as JSON lines to `file`",
			},
			&cli.StringFlag{
				Name:  _flagInputFile,
				Usage: "read input from a JSON `file` instead of stdin (JSON lines or a top-level JSON array)",
			},
			&cli.Float64Flag{
				Name:  _flagRPS,
				Usage: "max executions per second across all workers (0 = unlimited)",
				Value: 0,
			},
		},
		Action: func(c *cli.Context) error {
			options := streamexec.Options{
				OutputLog: c.String(_flagOutputLogPath),
				Params: streamexec.Params{
					ExecString: c.String(_flagExecCmd),
					Retries:    c.Int(_flagRetries),
				},
				Concurrency:   c.Int(_flagConcurrency),
				ContinueOnErr: c.Bool(_flagContinue),
				DebugMode:     c.Bool(_flagDebug),
				DryRun:        c.Bool(_flagDryRun),
				RPS:           c.Float64(_flagRPS),
			}
			input := io.ReadCloser(os.Stdin)
			if path := c.String(_flagInputFile); path != "" {
				f, err := openInputFile(path)
				if err != nil {
					return cli.Exit(fmt.Sprintf("cannot open input file: %v", err), 1)
				}
				input = f
			}
			ex := streamexec.New(input, os.Stdout, os.Stderr, options)
			return ex.Run()
		},
	}
}

func cmdList() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list running stream-exec processes",
		Action: func(c *cli.Context) error {
			sockets := listSockets()
			if len(sockets) == 0 {
				fmt.Println("no running stream-exec processes found")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "PID\tRUNNING\tDONE\tFAILED\tIN-FLIGHT\tCONCURRENCY\tEXEC")
			for _, sock := range sockets {
				resp, err := streamexec.QuerySocket(sock, _ipcCmdStatus)
				if err != nil {
					os.Remove(sock)
					continue
				}
				if !resp.OK || resp.Status == nil {
					continue
				}
				st := resp.Status
				execStr := st.ExecString
				if len(execStr) > 50 {
					execStr = execStr[:47] + "..."
				}
				running := time.Since(st.StartTime).Round(time.Second).String()
				fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%d\t%d\t%s\n",
					st.PID, running, st.Processed, st.Failed, st.InFlight, st.Concurrency, execStr)
			}
			w.Flush()
			return nil
		},
	}
}

func cmdSignal() *cli.Command {
	return &cli.Command{
		Name:  "signal",
		Usage: "send a signal to a running stream-exec process",
		Subcommands: []*cli.Command{
			{
				Name:      "stop",
				Usage:     "gracefully stop a running process (drains in-flight work)",
				ArgsUsage: "<pid>",
				Action: func(c *cli.Context) error {
					if c.NArg() != 1 {
						return cli.Exit("usage: stream-exec signal stop <pid>", 1)
					}
					pid := c.Args().First()
					sock := filepath.Join(streamexec.SocketDir(), pid+".sock")
					resp, err := streamexec.QuerySocket(sock, _ipcCmdStop)
					if err != nil {
						return cli.Exit(fmt.Sprintf("could not connect to process %s: %v", pid, err), 1)
					}
					if !resp.OK {
						return cli.Exit(fmt.Sprintf("stop failed: %s", resp.Error), 1)
					}
					fmt.Printf("sent stop signal to process %s\n", pid)
					return nil
				},
			},
			{
				Name:  "concurrency",
				Usage: "change the number of concurrent workers for a running process",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:  _flagPID,
						Usage: "PID of the target stream-exec process",
					},
					&cli.IntFlag{
						Name:     _flagSetConcurrency,
						Aliases:  []string{"c"},
						Usage:    "new concurrency value (must be > 0)",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					pid := c.Int(_flagPID)
					n := c.Int(_flagSetConcurrency)
					if n <= 0 {
						return cli.Exit("concurrency must be a positive integer", 1)
					}

					// grab the running processes, if there's only one, we'll signal it for ergonomics
					if pid <= 0 {
						sockets := listSockets()
						if len(sockets) != 1 {
							return cli.Exit("the pid of the running instance needs to be specified with --pid", 1)
						}
						pid = getOnlyRunningInstance(sockets[0])
					}

					sock := streamexec.SocketPath(pid)

					resp, err := streamexec.QuerySocket(sock, _ipcCmdSetConcurrency, n)
					if err != nil {
						return cli.Exit(fmt.Sprintf("could not connect to process %d: %v", pid, err), 1)
					}
					if !resp.OK {
						return cli.Exit(fmt.Sprintf("set-concurrency failed: %s", resp.Error), 1)
					}
					if resp.Status != nil {
						fmt.Printf("concurrency set. scaling up/down make take a moment (pid %d)\n", pid)
					} else {
						fmt.Printf("concurrency updated for process %d\n", pid)
					}
					return nil
				},
			},
		},
	}
}

func getOnlyRunningInstance(socket string) int {
	resp, err := streamexec.QuerySocket(socket, _ipcCmdStatus)
	if err != nil {
		cli.Exit("coulkdn't connect to pid automatically, specify the pid manually with --pid", 1)
	}
	if !resp.OK || resp.Status == nil {
		cli.Exit("coulkdn't connect to pid automatically, specify the pid manually with --pid", 1)
	}
	return resp.Status.PID
}

func listSockets() []string {
	sockets, _ := filepath.Glob(filepath.Join(streamexec.SocketDir(), "*.sock"))
	return sockets
}

// openInputFile opens path and returns a ReadCloser that emits JSON lines.
// If the file begins with '[' it is treated as a JSON array and each element
// is streamed as an individual JSON line; otherwise the file is returned as-is
// (JSON lines format).
func openInputFile(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	br := bufio.NewReader(f)

	// Peek past leading whitespace to detect array vs JSON lines.
	var first byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("reading input file: %w", err)
		}
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			first = b
			br.UnreadByte()
			break
		}
	}

	if first != '[' {
		// JSON lines — return the buffered reader; closing closes the file.
		return struct {
			io.Reader
			io.Closer
		}{br, f}, nil
	}

	// JSON array — stream each element as a JSON line through a pipe so we
	// never load the whole array into memory.
	pr, pw := io.Pipe()
	go func() {
		defer f.Close()
		dec := json.NewDecoder(br)
		if _, err := dec.Token(); err != nil { // consume '['
			pw.CloseWithError(fmt.Errorf("parsing JSON array: %w", err))
			return
		}
		enc := json.NewEncoder(pw)
		enc.SetEscapeHTML(false)
		for dec.More() {
			var elem json.RawMessage
			if err := dec.Decode(&elem); err != nil {
				pw.CloseWithError(fmt.Errorf("parsing JSON array element: %w", err))
				return
			}
			if err := enc.Encode(elem); err != nil {
				pw.CloseWithError(err)
				return
			}
		}
		pw.Close()
	}()
	return pr, nil
}
