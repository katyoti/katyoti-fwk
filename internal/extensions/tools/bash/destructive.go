// SPDX-License-Identifier: Apache-2.0

package bash

import "regexp"

// destructivePattern matches a command against a known destructive operation
// and provides a human-readable warning for the permission UI.
type destructivePattern struct {
	pattern *regexp.Regexp
	warning string
	// exclude is an optional secondary check: if the command also matches
	// exclude, the pattern is NOT considered destructive. Used for dry-run
	// exclusions where Go's regexp lacks negative lookahead.
	exclude *regexp.Regexp
}

// destructivePatterns is the list of known destructive command patterns.
// Ported from Claude Code destructiveCommandWarning.ts.
var destructivePatterns = []destructivePattern{
	// Git — data loss / hard to reverse
	{regexp.MustCompile(`\bgit\s+reset\s+--hard\b`), "Note: may discard uncommitted changes", nil},
	{regexp.MustCompile(`\bgit\s+push\b[^;&|]*[ \t](--force|--force-with-lease|-f)\b`), "Note: may overwrite remote history", nil},
	// git clean -f but NOT git clean -n (dry-run)
	{regexp.MustCompile(`\bgit\s+clean\b[^;&|]*-[a-zA-Z]*f`), "Note: may permanently delete untracked files",
		regexp.MustCompile(`\bgit\s+clean\b[^;&|]*(-[a-zA-Z]*n|--dry-run)`)},
	{regexp.MustCompile(`\bgit\s+checkout\s+(--\s+)?\.[ \t]*($|[;&|])`), "Note: may discard all working tree changes", nil},
	{regexp.MustCompile(`\bgit\s+restore\s+(--\s+)?\.[ \t]*($|[;&|])`), "Note: may discard all working tree changes", nil},
	{regexp.MustCompile(`\bgit\s+stash[ \t]+(drop|clear)\b`), "Note: may permanently remove stashed changes", nil},
	{regexp.MustCompile(`\bgit\s+branch\s+(-D[ \t]|--delete\s+--force|--force\s+--delete)\b`), "Note: may force-delete a branch", nil},

	// Git — safety bypass
	{regexp.MustCompile(`\bgit\s+(commit|push|merge)\b[^;&|]*--no-verify\b`), "Note: may skip safety hooks", nil},
	{regexp.MustCompile(`\bgit\s+commit\b[^;&|]*--amend\b`), "Note: may rewrite the last commit", nil},

	// File deletion — order matters: most specific first
	{regexp.MustCompile(`(^|[;&|]\s*)rm\s+-[a-zA-Z]*[rR][a-zA-Z]*f|(^|[;&|]\s*)rm\s+-[a-zA-Z]*f[a-zA-Z]*[rR]`), "Note: may recursively force-remove files", nil},
	{regexp.MustCompile(`(^|[;&|]\s*)rm\s+-[a-zA-Z]*[rR]`), "Note: may recursively remove files", nil},
	{regexp.MustCompile(`(^|[;&|]\s*)rm\s+-[a-zA-Z]*f`), "Note: may force-remove files", nil},

	// Database
	{regexp.MustCompile(`(?i)\b(DROP|TRUNCATE)\s+(TABLE|DATABASE|SCHEMA)\b`), "Note: may drop or truncate database objects", nil},
	{regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+\w+[ \t]*(;|"|'|$)`), "Note: may delete all rows from a database table", nil},

	// Infrastructure
	{regexp.MustCompile(`\bkubectl\s+delete\b`), "Note: may delete Kubernetes resources", nil},
	{regexp.MustCompile(`\bterraform\s+destroy\b`), "Note: may destroy Terraform infrastructure", nil},
}

// GetDestructiveWarning checks if a command matches known destructive patterns.
// Returns a human-readable warning string, or empty if no match.
func GetDestructiveWarning(command string) string {
	for _, dp := range destructivePatterns {
		if dp.pattern.MatchString(command) {
			// Check exclusion pattern (e.g. dry-run negates destructive).
			if dp.exclude != nil && dp.exclude.MatchString(command) {
				continue
			}
			return dp.warning
		}
	}
	return ""
}
