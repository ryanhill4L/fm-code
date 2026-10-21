package main

import "strings"

func legacyFormat(s string) string {
	return "[" + strings.ToUpper(s) + "]"
}

func format(s string) string {
	return "<" + s + ">"
}
