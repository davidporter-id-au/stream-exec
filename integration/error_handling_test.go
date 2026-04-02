package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TC-30: a failing command (non-zero exit) without --continue must cause
// stream-exec to exit non-zero and write the error to stderr.
func TestErrorExitsNonZeroAndReportsOnStderr(t *testing.T) {
	r := run(t, `{"n":"1"}`, "--exec", "exit 1")
	assert.NotEqual(t, 0, r.exitCode, "should exit non-zero on command failure")
	assert.NotEmpty(t, r.stderr, "error should be reported on stderr")
}

// TC-31: with --continue, a failing command must still be reported on stderr
// and remaining records must be processed.
func TestContinueOnErrorReportsAndContinues(t *testing.T) {
	var sb strings.Builder
	for _, v := range []string{"1", "2", "3"} {
		sb.WriteString(`{"n":"` + v + `"}` + "\n")
	}

	// middle record fails; first and last should still produce output
	r := run(t, sb.String(),
		"--exec", `if [ "$n" = "2" ]; then exit 1; fi; echo $n`,
		"--continue",
	)
	require.Equal(t, 0, r.exitCode, "should exit 0 with --continue")
	assert.Contains(t, r.stdout, "1")
	assert.Contains(t, r.stdout, "3")
	assert.NotEmpty(t, r.stderr, "error for record 2 should be reported on stderr")
}

// TC-32: without --continue, only the records processed before the first
// failure should produce output (i.e. execution stops).
func TestStopsProcessingAfterFirstError(t *testing.T) {
	// 5 records: first always fails; with concurrency=1 and no --continue,
	// stream-exec must exit before processing subsequent records.
	var sb strings.Builder
	for i := 1; i <= 5; i++ {
		sb.WriteString(`{"n":"` + string(rune('0'+i)) + `"}` + "\n")
	}

	r := run(t, sb.String(), "--exec", "exit 1", "--concurrency", "1")
	assert.NotEqual(t, 0, r.exitCode, "should exit non-zero")
	// Without --continue, not all 5 records should have been echoed to stdout.
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	nonEmpty := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			nonEmpty++
		}
	}
	assert.Less(t, nonEmpty, 5, "should stop before processing all records")
}
