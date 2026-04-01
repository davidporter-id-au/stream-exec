package integration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TC-21: a single trailing newline (the common case from echo/editors) is handled correctly.
// splitInputBytes absorbs the trailing \n into nextRemainder which becomes "".
func TestSingleTrailingNewlineIsHandledCorrectly(t *testing.T) {
	input := `{"a":1}` + "\n"
	r := run(t, input, "--exec", "echo $a")
	assert.Equal(t, 0, r.exitCode, "single trailing newline should not cause an error")
	assert.Contains(t, r.stdout, "1")
}

// TC-22: blank lines between records cause errors.
// This documents the same empty-line bug as TC-21 for a different common input shape.
func TestBlankLinesBetweenRecordsCauseErrors(t *testing.T) {
	input := "{\"a\":1}\n\n{\"a\":2}"

	// With --continue: both records should be processed and 2 errors emitted for the
	// blank lines. Without it: exits after the first blank line.
	r := run(t, input, "--exec", "echo $a", "--continue")
	// BUG: blank lines should be silently skipped, not treated as parse errors.
	if strings.Contains(r.stderr, "invalid character") || strings.Contains(r.stderr, "unexpected end") {
		t.Logf("KNOWN BUG (TC-22): blank lines produce JSON parse errors on stderr: %s", r.stderr)
		t.Fail()
	}
	assert.Contains(t, r.stdout, "1")
	assert.Contains(t, r.stdout, "2")
}

// TC-23: line longer than the 5000-byte read buffer.
//
// BUG in splitInputBytes (stream-exec.go:258-260): when the current read chunk has no
// newline, it returns (nil, dataString) — discarding prevRemainder entirely. So for a
// line spanning N reads, only the last chunk survives; earlier chunks are silently lost.
// The truncated JSON is then rejected by the parser.
func TestLineLongerThanReadBuffer(t *testing.T) {
	longValue := strings.Repeat("x", 8000)
	input := fmt.Sprintf(`{"val":"%s"}`, longValue)

	r := run(t, input, "--exec", "echo ${#val}") // bash: print byte length of $val
	if r.exitCode != 0 {
		t.Logf("KNOWN BUG (TC-23): line longer than read buffer loses data — stderr: %s", r.stderr)
		t.Fail()
		return
	}
	assert.Contains(t, r.stdout, "8000")
}

// TC-24: large volume of records
func TestLargeVolumeNoLostRecords(t *testing.T) {
	const count = 2000
	var sb strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&sb, `{"n":%d}`+"\n", i)
	}

	r := run(t, sb.String(), "--exec", "echo $n", "--concurrency", "10", "--continue")
	require.Equal(t, 0, r.exitCode, "stderr: %s", r.stderr)

	outputLines := 0
	for _, line := range strings.Split(strings.TrimSpace(r.stdout), "\n") {
		if strings.TrimSpace(line) != "" {
			outputLines++
		}
	}
	// BUG TC-21/22: trailing newlines may inflate errors; use --continue and count successes
	assert.Equal(t, count, outputLines, "expected %d output lines, got %d", count, outputLines)
}

// TC-25: empty input — stdin closed immediately
func TestEmptyInputExitsCleanly(t *testing.T) {
	r := run(t, "", "--exec", "echo hello")
	assert.Equal(t, 0, r.exitCode)
	// no output expected
	assert.Empty(t, strings.TrimSpace(r.stdout))
}

// TC-26: single record with no trailing newline
func TestSingleRecordNoTrailingNewline(t *testing.T) {
	input := `{"x":"hello"}` // no \n
	r := run(t, input, "--exec", "echo $x")
	require.Equal(t, 0, r.exitCode, "stderr: %s", r.stderr)
	assert.Contains(t, r.stdout, "hello")
}

// TC-27: keys with special characters are sanitised to valid env var names
func TestKeysWithSpecialCharacters(t *testing.T) {
	// hyphens and dots should become underscores
	r := run(t, `{"my-key":"v1","my.key2":"v2"}`, "--exec", "echo $my_key $my_key2")
	require.Equal(t, 0, r.exitCode, "stderr: %s", r.stderr)
	assert.Contains(t, r.stdout, "v1")
	assert.Contains(t, r.stdout, "v2")
}

// TC-28: shell metacharacters in values should not be executed when the variable is quoted
func TestShellMetacharactersInValues(t *testing.T) {
	// $(...) in a value must not execute when the envvar is quoted in the command
	r := run(t, `{"cmd":"$(echo injected)"}`, "--exec", `echo "$cmd"`)
	require.Equal(t, 0, r.exitCode, "stderr: %s", r.stderr)
	// The literal string should appear, not the result of execution
	assert.Contains(t, r.stdout, `$(echo injected)`)
	assert.NotContains(t, r.stdout, "injected\n")
}

// TC-29: very large integer near float64 precision boundary
// JSON numbers are float64; 2^53+1 cannot be represented exactly.
// This test documents the known precision loss rather than asserting correct behaviour.
func TestLargeIntegerPrecisionLoss(t *testing.T) {
	// 2^53 = 9007199254740992 is the last exactly-representable integer in float64.
	// 2^53 + 1 = 9007199254740993 is NOT exactly representable.
	r := run(t, `{"id":9007199254740993}`, "--exec", "echo $id")
	require.Equal(t, 0, r.exitCode, "stderr: %s", r.stderr)

	output := strings.TrimSpace(r.stdout)
	if output != "9007199254740993" {
		t.Logf("KNOWN LIMITATION (TC-29): float64 precision loss: input 9007199254740993, got %s", output)
		// Not a hard failure — document the limitation
	}
}

// keys with leading digits are not valid POSIX env var names; the tool replaces
// non-alphanumeric/underscore characters in keys but does not handle the leading-digit case.
func TestKeyStartingWithDigit(t *testing.T) {
	r := run(t, `{"1abc":"val"}`, "--exec", "echo done")
	// Just verify the process doesn't crash; the env var name will be "1abc" which
	// bash will silently ignore when used as $1abc.
	assert.Equal(t, 0, r.exitCode)
}
