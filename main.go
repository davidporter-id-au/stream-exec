package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"

	streamexec "github.com/davidporter-id-au/stream-exec/stream-exec"
)

const (
	_flagExecCmd       = "exec-command"
	_flagConcurrency   = "concurrency"
	_flagRetries       = "retries"
	_flagContinue      = "continue"
	_flagDryRun        = "dry-run"
	_flagDebug         = "debug"
	_flagOutputLogPath = "output-log-path"
	_flagErrLogPath    = "err-log-path"
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
				Aliases: []string{"x"},
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
				Name:  _flagErrLogPath,
				Usage: "write failures as JSON lines to `file`",
			},
		},
		Action: func(c *cli.Context) error {
			options := streamexec.Options{
				ErrorLog:  c.String(_flagErrLogPath),
				OutputLog: c.String(_flagOutputLogPath),
				Params: streamexec.Params{
					ExecString: c.String(_flagExecCmd),
					Retries:    c.Int(_flagRetries),
				},
				Concurrency:   c.Int(_flagConcurrency),
				ContinueOnErr: c.Bool(_flagContinue),
				DebugMode:     c.Bool(_flagDebug),
				DryRun:        c.Bool(_flagDryRun),
			}
			ex := streamexec.New(os.Stdin, os.Stdout, os.Stderr, options)
			return ex.Run()
		},
	}
}

func cmdList() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "list running stream-exec processes",
		Action: func(c *cli.Context) error {
			sockets, _ := filepath.Glob(filepath.Join(streamexec.SocketDir(), "*.sock"))
			if len(sockets) == 0 {
				fmt.Println("no running stream-exec processes found")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "PID\tRUNNING\tDONE\tFAILED\tIN-FLIGHT\tEXEC")
			for _, sock := range sockets {
				resp, err := streamexec.QuerySocket(sock, "status")
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
				fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%d\t%s\n",
					st.PID, running, st.Processed, st.Failed, st.InFlight, execStr)
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
					resp, err := streamexec.QuerySocket(sock, "stop")
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
		},
	}
}
