package main

import (
	"regexp"
	"strings"
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s,;]+`),
	regexp.MustCompile(`(?i)(github_token|gh_token|aws_secret_access_key|aws_session_token|password|passwd|token|secret)\s*([=:])\s*[^\s,;]+`),
	regexp.MustCompile(`(?i)(x-api-key\s*:\s*)[^\s,;]+`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
}

// RedactSecrets removes common credentials while preserving useful command logs.
func RedactSecrets(input string) string {
	result := input
	for _, pattern := range secretPatterns {
		result = pattern.ReplaceAllStringFunc(result, func(match string) string {
			lower := strings.ToLower(match)
			if strings.Contains(lower, "bearer") {
				idx := strings.LastIndex(lower, "bearer")
				return match[:idx] + "bearer [REDACTED]"
			}
			if strings.Contains(match, "=") {
				return match[:strings.Index(match, "=")+1] + "[REDACTED]"
			}
			if strings.Contains(match, ":") {
				return match[:strings.Index(match, ":")+1] + " [REDACTED]"
			}
			return "[REDACTED]"
		})
	}
	return result
}
