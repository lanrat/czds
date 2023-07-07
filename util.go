package czds

import "strings"

func slice2LowerMap(array []string) map[string]bool {
	out := make(map[string]bool)

	for _, s := range array {
		out[strings.ToLower(s)] = true
	}

	return out
}
