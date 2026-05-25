package node

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
)

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func short(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func unique(values []string) []string {
	if len(values) < 2 {
		return values
	}
	sort.Strings(values)
	out := values[:0]
	last := ""
	for _, value := range values {
		if value == last {
			continue
		}
		out = append(out, value)
		last = value
	}
	return out
}
