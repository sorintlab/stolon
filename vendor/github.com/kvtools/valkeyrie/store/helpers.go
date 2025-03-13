package store

import (
	"strings"
)

// CreateEndpoints creates a list of endpoints given the right scheme.
func CreateEndpoints(addrs []string, scheme string) (entries []string) {
	for _, addr := range addrs {
		entries = append(entries, scheme+"://"+addr)
	}

	return entries
}

// SplitKey splits the key to extract path information.
func SplitKey(key string) (path []string) {
	if strings.Contains(key, "/") {
		return strings.Split(key, "/")
	}

	return []string{key}
}
