package echo

import (
	"regexp"
	"strings"
)

// IsMatch checks if a hostname matches a pattern
// Supports:
// - Exact match: "example.com"
// - Wildcard: "*.example.com"
// - Substring: "example" (matches "example.com", "test.example.com", etc.)
func IsMatch(hostname, pattern string) bool {
	// Exact match
	if hostname == pattern {
		return true
	}
	if pattern == "*" {
		return true
	}
	// Wildcard pattern
	if strings.Contains(pattern, "*") {
		regexPattern := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), "\\*", ".*") + "$"
		matched, err := regexp.MatchString(regexPattern, hostname)
		if err == nil && matched {
			return true
		}
	}

	// Substring match
	if strings.Contains(hostname, pattern) {
		return true
	}

	return false
}
