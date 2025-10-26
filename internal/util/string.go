package util

import "strings"

func SplitLines(s string) []string {
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}

func SplitFields(s string) []string {
	return strings.Fields(s)
}
