package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stream-exec-input-*.json")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// TC-33: JSON lines file is processed record by record.
func TestInputFileJSONLines(t *testing.T) {
	path := writeTemp(t, `{"name":"alice"}`+"\n"+`{"name":"bob"}`+"\n")
	r := run(t, "", "--exec", "echo $name", "--input-json-file", path)
	require.Equal(t, 0, r.exitCode, "stderr: %s", r.stderr)
	assert.Contains(t, r.stdout, "alice")
	assert.Contains(t, r.stdout, "bob")
}

// TC-34: top-level JSON array is exploded into one record per element.
func TestInputFileJSONArray(t *testing.T) {
	path := writeTemp(t, `[{"name":"carol"},{"name":"dave"},{"name":"eve"}]`)
	r := run(t, "", "--exec", "echo $name", "--input-json-file", path)
	require.Equal(t, 0, r.exitCode, "stderr: %s", r.stderr)
	assert.Contains(t, r.stdout, "carol")
	assert.Contains(t, r.stdout, "dave")
	assert.Contains(t, r.stdout, "eve")
}

// TC-35: JSON array with leading whitespace is still detected correctly.
func TestInputFileJSONArrayLeadingWhitespace(t *testing.T) {
	path := writeTemp(t, "\n  [\n{\"x\":\"1\"},{\"x\":\"2\"}\n]\n")
	r := run(t, "", "--exec", "echo $x", "--input-json-file", path)
	require.Equal(t, 0, r.exitCode, "stderr: %s", r.stderr)
	assert.Contains(t, r.stdout, "1")
	assert.Contains(t, r.stdout, "2")
}

// TC-36: a non-existent file produces a non-zero exit and a useful error.
func TestInputFileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	r := run(t, "", "--exec", "echo $x", "--input-json-file", path)
	assert.NotEqual(t, 0, r.exitCode)
	assert.Contains(t, r.stderr, "cannot open input file")
}
