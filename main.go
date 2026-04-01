package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	streamexec "github.com/davidporter-id-au/stream-exec/stream-exec"
)

func main() {
	var execString string
	var errorLogPath string
	var outputLogPath string
	var concurrency int
	var retries int
	var continueOnError bool
	var dryRun bool
	var debug bool
	var list bool
	var stopPID int

	flag.StringVar(&execString, "exec", "", "the thing to run")
	flag.BoolVar(&continueOnError, "continue", false, "continue on error")
	flag.IntVar(&concurrency, "concurrency", 1, "number of concurrent operations")
	flag.IntVar(&retries, "retries", 0, "the number of attempts to retry failures")
	flag.BoolVar(&dryRun, "dry-run", false, "show what would run")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.StringVar(&outputLogPath, "output-log-path", "", "where to write the output log, leave as '' for none")
	flag.StringVar(&errorLogPath, "err-log-path", "", "where to write the error log, leave as '' for none")
	flag.BoolVar(&list, "list", false, "list running stream-exec processes")
	flag.IntVar(&stopPID, "stop", 0, "gracefully stop the stream-exec process with the given PID")

	flag.Parse()

	if list {
		listInstances()
		return
	}

	if stopPID != 0 {
		stopInstance(stopPID)
		return
	}

	if execString == "" {
		log.Fatal("Exec string can't be empty")
	}

	options := streamexec.Options{
		ErrorLog:  errorLogPath,
		OutputLog: outputLogPath,
		Params: streamexec.Params{
			ExecString: execString,
			Retries:    retries,
		},
		Concurrency:   concurrency,
		ContinueOnErr: continueOnError,
		DebugMode:     debug,
		DryRun:        dryRun,
	}

	exec := streamexec.New(os.Stdin, os.Stdout, os.Stderr, options)
	err := exec.Run()
	if err != nil {
		panic(err)
	}
}

func listInstances() {
	sockets, _ := filepath.Glob(filepath.Join(streamexec.SocketDir(), "*.sock"))
	if len(sockets) == 0 {
		fmt.Println("no running stream-exec processes found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PID\tRUNNING\tDONE\tFAILED\tIN-FLIGHT\tEXEC")

	for _, sock := range sockets {
		resp, err := streamexec.QuerySocket(sock, "status")
		if err != nil {
			os.Remove(sock) // stale socket from a crashed process
			continue
		}
		if !resp.OK || resp.Status == nil {
			continue
		}
		st := resp.Status
		exec := st.ExecString
		if len(exec) > 50 {
			exec = exec[:47] + "..."
		}
		running := time.Since(st.StartTime).Round(time.Second).String()
		fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%d\t%s\n",
			st.PID, running, st.Processed, st.Failed, st.InFlight, exec)
	}
	w.Flush()
}

func stopInstance(pid int) {
	sock := streamexec.SocketPath(pid)
	resp, err := streamexec.QuerySocket(sock, "stop")
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not connect to process %d: %v\n", pid, err)
		os.Exit(1)
	}
	if !resp.OK {
		fmt.Fprintf(os.Stderr, "stop failed: %s\n", resp.Error)
		os.Exit(1)
	}
	fmt.Printf("sent stop signal to process %d\n", pid)
}
