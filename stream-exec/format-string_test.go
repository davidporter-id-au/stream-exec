package streamexec

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormattingEnvvarString(t *testing.T) {
	input := map[string]struct {
		input          string
		expectedOutput []string
		expectedErr    error
	}{
		"happy path example": {
			input: `{"a": 1, "b": "c", "d": [1, 2, 3], "e": {"z":"x"}}`,
			expectedOutput: []string{
				"a=1",
				"b=c",
				"d=[1,2,3]",
				"e={\"z\":\"x\"}",
			},
		},
		"invalid envvar chars": {
			input: `{" a space filed key": 1, "candleßrasser": 1}`,
			expectedOutput: []string{
				"_a_space_filed_key=1",
				"candle_rasser=1",
			},
		},
	}

	for name, td := range input {
		t.Run(name, func(t *testing.T) {
			res, err := formatEnvString(td.input)
			sort.Strings(td.expectedOutput)
			sort.Strings(res)
			assert.Equal(t, td.expectedOutput, res, name)
			assert.Equal(t, td.expectedErr, err, name)
		})
	}
}
