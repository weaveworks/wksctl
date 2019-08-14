package utilities

import "strings"

func Indent(s, prefix string) string {
	trimmed := strings.TrimRight(s, "\n")
	return prefix + strings.Replace(trimmed, "\n", "\n"+prefix, -1)
}
