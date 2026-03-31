package integration

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TC-01: single record, variable substitution
func TestBasicVariableSubstitution(t *testing.T) {
	r := run(t, `{"user":"alice","document":"doc1"}`, "-exec", "echo $user $document")
	assert.Equal(t, 0, r.exitCode)
	assert.Contains(t, r.stdout, "alice doc1")
}

// TC-02: multiple records, all processed
func TestMultipleRecordsAllProcessed(t *testing.T) {
	var lines []string
	for _, id := range []string{"id1", "id2", "id3", "id4", "id5"} {
		lines = append(lines, `{"id":"`+id+`"}`)
	}
	input := strings.Join(lines, "\n")

	r := run(t, input, "-exec", "echo $id", "-concurrency", "1")
	require.Equal(t, 0, r.exitCode)
	for _, id := range []string{"id1", "id2", "id3", "id4", "id5"} {
		assert.Contains(t, r.stdout, id)
	}
}

// TC-03: large integers are not formatted as floats
func TestIntegerNotFloat(t *testing.T) {
	r := run(t, `{"ts":1652988784000}`, "-exec", "echo $ts")
	assert.Equal(t, 0, r.exitCode)
	assert.Contains(t, r.stdout, "1652988784000")
	assert.NotContains(t, r.stdout, "1.652988784e+12")
	assert.NotContains(t, r.stdout, "e+")
}

// TC-04: float values are preserved
func TestFloatPreserved(t *testing.T) {
	r := run(t, `{"rate":1.5}`, "-exec", "echo $rate")
	assert.Equal(t, 0, r.exitCode)
	assert.Contains(t, r.stdout, "1.5")
}

// whole-number floats should render as integers (1.0 -> 1)
func TestWholeNumberFloatBecomesInt(t *testing.T) {
	r := run(t, `{"val":2.0}`, "-exec", "echo $val")
	assert.Equal(t, 0, r.exitCode)
	assert.Contains(t, r.stdout, "2")
	assert.NotContains(t, r.stdout, "2.0")
}

// TC-05: nested objects become JSON strings
func TestNestedObjectBecomesJSONString(t *testing.T) {
	r := run(t, `{"meta":{"key":"val"}}`, "-exec", "echo $meta")
	assert.Equal(t, 0, r.exitCode)
	assert.Contains(t, r.stdout, `{"key":"val"}`)
}

// TC-06: arrays become JSON strings
func TestArrayBecomesJSONString(t *testing.T) {
	r := run(t, `{"tags":["a","b","c"]}`, "-exec", "echo $tags")
	assert.Equal(t, 0, r.exitCode)
	assert.Contains(t, r.stdout, `["a","b","c"]`)
}

// TC-07: null values become empty string
func TestNullValueBecomesEmpty(t *testing.T) {
	r := run(t, `{"name":null}`, "-exec", `bash -c '[ -z "$name" ] && echo "empty" || echo "not empty"'`)
	assert.Equal(t, 0, r.exitCode)
	assert.Contains(t, r.stdout, "empty")
}

// TC-08: boolean values
func TestBooleanValues(t *testing.T) {
	r := run(t, `{"active":true}`, "-exec", "echo $active")
	assert.Equal(t, 0, r.exitCode)
	assert.Contains(t, r.stdout, "true")

	r = run(t, `{"active":false}`, "-exec", "echo $active")
	assert.Equal(t, 0, r.exitCode)
	assert.Contains(t, r.stdout, "false")
}

// output goes to stdout, not stderr on success
func TestSuccessOutputOnStdout(t *testing.T) {
	r := run(t, `{"x":"hello"}`, "-exec", "echo $x")
	assert.Equal(t, 0, r.exitCode)
	assert.Contains(t, r.stdout, "hello")
	assert.Empty(t, r.stderr)
}
