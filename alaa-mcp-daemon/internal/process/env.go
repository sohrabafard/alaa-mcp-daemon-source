package process

import (
	"os"
	"sort"
	"strings"
)

func mergedEnv(overrides map[string]string) []string {
	values := make(map[string]string)
	keys := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := splitEnvEntry(item)
		if !ok {
			continue
		}
		folded := strings.ToUpper(key)
		keys[folded] = key
		values[folded] = value
	}
	for key, value := range overrides {
		folded := strings.ToUpper(key)
		keys[folded] = key
		values[folded] = value
	}
	ordered := make([]string, 0, len(keys))
	for folded := range keys {
		ordered = append(ordered, folded)
	}
	sort.Strings(ordered)
	result := make([]string, 0, len(ordered))
	for _, folded := range ordered {
		result = append(result, keys[folded]+"="+values[folded])
	}
	return result
}

func splitEnvEntry(item string) (key, value string, ok bool) {
	if strings.HasPrefix(item, "=") {
		if relative := strings.IndexByte(item[1:], '='); relative >= 0 {
			separator := relative + 1
			return item[:separator], item[separator+1:], true
		}
	}
	return strings.Cut(item, "=")
}
