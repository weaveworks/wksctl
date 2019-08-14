package resource

import (
	"bufio"
	"strings"
)

// A few utility function to parse program outputs

// line return the first line of output.
func line(output string) string {
	stringReader := strings.NewReader(output)
	r := bufio.NewReader(stringReader)
	l, err := r.ReadString('\n')
	if err != nil {
		return output
	}
	return strings.TrimRight(l, "\n")
}

// keyval parses key=val lines and return the val for the corresponding key.
func keyval(output, key string) string {
	keyequal := key + "="

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, keyequal) {
			return strings.Trim(line[len(keyequal):], "\"")
		}
	}

	return ""
}
