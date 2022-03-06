package main

import (
	"flag"
	"fmt"
	"log"
	"os"
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

	flag.StringVar(&execString, "exec", "", "the thing to run")
	flag.BoolVar(&continueOnError, "continue", false, "continue on error")
	flag.IntVar(&concurrency, "concurrency", 10, "number of concurrent operations")
	flag.IntVar(&retries, "retries", 0, "the number of attempts to retry failures")
	flag.BoolVar(&dryRun, "dry-run", false, "show what would run")
	flag.BoolVar(&debug, "debug", false, "enable debug logging")
	flag.StringVar(&outputLogPath, "output-log-path", "", "where to write the output log, leave as '' for none")
	flag.StringVar(&errorLogPath, "err-log-path", fmt.Sprintf("error-output-%s.log", time.Now().Format("2006-01-02__15_04_05Z07")), "where to write the error log, leave as '' for none")

	flag.Parse()

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
