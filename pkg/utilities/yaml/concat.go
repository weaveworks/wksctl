package yaml

import "bytes"

var separator = []byte("---\n")

// Concat concatenates the provided YAML documents.
func Concat(yamls ...[]byte) []byte {
	return bytes.Join(yamls, separator)
}
