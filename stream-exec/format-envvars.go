package streamexec

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// apologies for the parochialism, non-ascii support would be preferred
// but I have no idea what support they have as envvars
var invalidEnvarKey = regexp.MustCompile("[^a-zA-Z0-9_]")

func formatEnvString(incoming string) ([]string, error) {
	data := make(map[string]interface{})
	err := json.Unmarshal([]byte(incoming), &data)
	if err != nil {
		return nil, err
	}
	var out []string
	for k, v := range data {
		out = append(out, fmt.Sprintf("%s=%v", invalidEnvarKey.ReplaceAllString(k, "_"), convert(v)))
	}
	return out, nil
}

func convert(val interface{}) string {

	switch v := val.(type) {
	case nil:
		return ""
	case int, int8, int16, int32, int64:
		// probably will never work because json doesn't do ints
		return fmt.Sprintf("%d", v)
	case float32, float64:

		// attempt to cast to int just in case
		f, ok := v.(float64) 
		if ok {
			i := int(f)	
			if float64(i) == f {
				// actually fine as an integer
				return fmt.Sprintf("%d", i)
			}
		}

		return fmt.Sprintf("%v", v)
	case string:
		return v
	default:
		d, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		return string(d)
	}
}
