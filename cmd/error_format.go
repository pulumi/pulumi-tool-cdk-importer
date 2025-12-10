package cmd

import "strings"

// formatCLIError removes noisy stdout/stderr dumps that Automation API attaches to errors.
// Up errors are already collapsed to a generic message, but other Automation API
// calls (e.g., preview/import skeleton generation) can still embed stdout/stderr.
func formatCLIError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimRight(err.Error(), "\n")
	// The Automation API formats errors as:
	// <message>
	// code: <code>
	// stdout: ...
	// stderr: ...
	if i := strings.Index(msg, "\ncode: "); i != -1 {
		return msg[:i]
	}
	if i := strings.Index(msg, "\nstdout: "); i != -1 {
		return msg[:i]
	}
	if i := strings.Index(msg, "\nstderr: "); i != -1 {
		return msg[:i]
	}
	return msg
}
