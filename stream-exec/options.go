package streamexec

const defaultConcurrency = 10
const defaultInputByteLen = 5000

type Options struct {
	ErrorLog           string
	OutputLog          string
	IncomingBufferSize int
	Concurrency        int
	ContinueOnErr      bool
	DryRun             bool
	DebugMode          bool
	RPS                float64 // max executions per second; 0 = unlimited
	Params             Params
}

type Params struct {
	ExecString string
	Retries    int
}
