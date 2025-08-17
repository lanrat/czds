package czds

import "strings"

// slice2LowerMap converts a slice of strings to a map with lowercase keys for fast lookup.
func slice2LowerMap(array []string) map[string]bool {
	out := make(map[string]bool)

	for _, s := range array {
		out[strings.ToLower(s)] = true
	}

	return out
}
