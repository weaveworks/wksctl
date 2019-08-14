package qjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	log "github.com/sirupsen/logrus"
)

const debug = false // Temporarily switch this to true if you want to see execution traces.

func init() {
	if debug {
		log.SetLevel(log.DebugLevel)
	}
}

// CollectStrings processes the provided JSON bytes and collects the values of
// string fields that match this query trie.
// Example of query string: "spec.containers.#.image", s.t. "#" represents a JSON array.
// nolint: gocyclo
func CollectStrings(queryString string, jsonBytes []byte) ([]string, error) {
	query := strings.Split(queryString, ".")
	decoder := json.NewDecoder(bytes.NewReader(jsonBytes))

	positions := []int{0} // ... to keep track of where we are in the query.
	tree := []string{}    // ... to keep track of where we are in the JSON tree.
	keys := []string{}    // ... to keep track of where we are in the JSON tree.
	key := false          // ... to keep track of keys vs. values in JSON objects.

	results := []string{}

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		current := positions[len(positions)-1]

		if debug {
			log.Debugf("token: %s, current: %d, positions: %v, tree: %v, keys: %v", token, current, positions, tree, keys)
		}

		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				tree = append(tree, "{") // "push" the fact we enter an object
				key = true               // next string, if any, will necessarily be a "key"
			case '}':
				if len(keys) > 0 && !isObjectWithinArray(tree) {
					positions = positions[:len(positions)-1] // "pop" the current position as we're done processing a JSON (object) "value"
					keys = keys[:len(keys)-1]                // "pop" the current key, since we've consumed the corresponding "value".
				}
				tree = tree[:len(tree)-1] // "pop" the previously pushed '}', as we are leaving this object
				key = true                // next string, if any, will necessarily be a "key"
			case '[':
				tree = append(tree, "[") // "push" the fact we enter an array
				key = false              // next string, if any, will necessarily be a "value"
				if query[current] == "#" {
					positions[len(positions)-1]++ // match: increment the current position
				} else {
					positions[len(positions)-1] = 0 // mismatch: reset the current position
				}
			case ']':
				if len(keys) > 0 {
					positions = positions[:len(positions)-1] // "pop" the current position as we're done processing a JSON (array) "value"
					keys = keys[:len(keys)-1]                // "pop" the current key, since we've consumed the corresponding "value".
				}
				tree = tree[:len(tree)-1] // "pop" the previously pushed ']', as we are leaving this array
				key = true                // next string, if any, will necessarily be a "key"
			default:
				// This should never happen for a well-formed JSON object, but just in case:
				return nil, fmt.Errorf("unknown delimiter: %v", value)
			}
		case string:
			if len(tree) == 0 {
				// JSON scalar string, nothing to match and collect:
				break
			} else if isWithinObject(tree) {
				if key { // current string is a "key":
					if query[current] == value {
						positions = append(positions, current+1) // match: increment & "push" the current position
					} else {
						positions = append(positions, 0) // mismatch: reset & "push" the current position
					}
					keys = append(keys, value) // "push" the current key.
				} else { // current string is a "value"
					if current == len(query) {
						results = append(results, value) // full match, we collect the current "value"
					}
					positions = positions[:len(positions)-1] // "pop" the current position as we're done processing a JSON (scalar) "value"
					keys = keys[:len(keys)-1]                // "pop" the current key, since we've consumed the corresponding "value".
				}
				// In an object,
				// - if current string is a "key", next string would be a "value",
				// - if current string is a "value", next string would be a "key".
				key = !key
			} else { // is within array:
				if current == len(query) {
					results = append(results, value) // full match, we collect the current "value"
				}
			}
		default: // int | bool | long | etc.
			positions = positions[:len(positions)-1] // "pop" the current position as we're done processing a JSON (scalar) "value"
			keys = keys[:len(keys)-1]                // "pop" the current key, since we've consumed the corresponding "value".
			if isWithinObject(tree) {
				key = true // Given the current token is necessarily a "value", the next one will be a "key" (and a string).
			} else {
				key = false // We are within an array, so the next token will either be another "value" or "]".
			}
		}
	}
	return results, nil
}

func isWithinObject(tree []string) bool {
	return tree[len(tree)-1] == "{"
}

func isObjectWithinArray(tree []string) bool {
	l := len(tree)
	return l > 1 && tree[l-1] == "{" && tree[l-2] == "["
}
