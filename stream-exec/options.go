package streamexec

const defaultConcurrency = 10
const defaultInputByteLen = 5000

type Options struct {
	ErrorLog           string
	OutputLog          string
	IncomingBufferSize int
	Concurrency        int
	ContinueOnErr      bool
	OutputDebugMode    bool
	Params             Params
}

type Params struct {
	ExecString string
	Retries    int
}
