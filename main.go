package main

import (
	"flag"
	"log"
	"os"

	streamexec "github.com/davidporter-id-au/stream-exec/stream-exec"
)

func main() {

	var execString string
	var concurrency int
	var continueOnError bool
	flag.StringVar(&execString, "exec", "", "the thing to run")
	flag.BoolVar(&continueOnError, "continue", false, "continue on error")
	flag.IntVar(&concurrency, "concurrency", 10, "number of concurrent operations")

	flag.Parse()

	if execString == "" {
		log.Fatal("Exec string can't be empty")
	}

	options := streamexec.Options{
		// ErrorLog: errorlog,
		Params: streamexec.Params{
			ExecString: execString,
		},
		Concurrency:   concurrency,
		ContinueOnErr: continueOnError,
	}

	exec := streamexec.New(os.Stdin, os.Stdout, os.Stderr, options)
	err := exec.Run()
	if err != nil {
		panic(err)
	}
}
