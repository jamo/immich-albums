package cmd_helper

import "strings"

// Repeat returns a string with n copies of the input string
func Repeat(s string, n int) string {
	return strings.Repeat(s, n)
}
