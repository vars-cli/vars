// Package format produces shell-safe output lines for different shell formats.
//
// Each format function takes a key and value and returns a single line
// that is safe to eval/source in the target shell. Values are properly
// escaped for the target format.
package format

import (
	"fmt"
	"strings"
)

// Posix returns an export line for bash/zsh: export KEY='value'
// Uses single-quote wrapping. Embedded single quotes are escaped as '\''
// (end single quote, escaped literal single quote, restart single quote).
func Posix(key string, value string) string {
	escaped := strings.ReplaceAll(value, "'", "'\\''")
	return fmt.Sprintf("export %s='%s'", key, escaped)
}

// Fish returns a set line for fish: set -x KEY 'value'
// Fish single quotes allow no escapes except \\ and \'.
func Fish(key string, value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")
	return fmt.Sprintf("set -x %s '%s'", key, escaped)
}

// Dotenv returns a dotenv line: KEY="value"
// Uses double-quote wrapping with backslash escaping for
// double quotes, backslashes, dollar signs, backticks, and newlines.
func Dotenv(key string, value string) string {
	escaped := value
	escaped = strings.ReplaceAll(escaped, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "$", "\\$")
	escaped = strings.ReplaceAll(escaped, "`", "\\`")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	return fmt.Sprintf("%s=\"%s\"", key, escaped)
}

// FormatFunc is the type for a format function.
type FormatFunc func(key, value string) string

// Get returns the FormatFunc for the given format name.
// Valid names: "posix" (default), "fish", "dotenv".
func Get(name string) (FormatFunc, error) {
	switch name {
	case "posix", "":
		return Posix, nil
	case "fish":
		return Fish, nil
	case "dotenv":
		return Dotenv, nil
	default:
		return nil, fmt.Errorf("unknown format %q (valid: posix, fish, dotenv)", name)
	}
}
