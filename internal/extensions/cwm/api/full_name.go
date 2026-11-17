// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"strings"
)

// The FULL NAME of an object (identity-and-index.md §1):
//
//	<project-id> . <module> . <local>
//
// The project id is the UUIDv7 the project object carries — the one computer
// id, minted once and committed, so a `git clone` anywhere carries the same
// full names. The full name is used only where a reference LEAVES the
// project: logs, session records, events, exports. Inside the project every
// reference is the authored `module.local`.
//
// ONE PARSER, here, beside SubjectToken: publisher, resolver and log reader
// all call it. A bare `module.local` is NOT a full name and ParseFullName
// refuses it — the in-project resolver accepts it, which is a different
// question with a different answer.

// FullName joins a project id and an authored id.
func FullName(projectID, id string) string {
	return projectID + "." + id
}

// ParseFullName splits a full name into its project id and its authored id.
// The project id must be a UUID (36 characters, 8-4-4-4-12 hex); anything
// else is refused as "not a full name" rather than guessed at.
func ParseFullName(s string) (projectID, id string, err error) {
	s = strings.TrimSpace(s)
	if len(s) < 38 || s[36] != '.' {
		return "", "", fmt.Errorf("%w: %q is not a full name (<project-uuid>.<module>.<local>)", ErrInvalidInput, s)
	}
	projectID, id = s[:36], s[37:]
	if !isUUID(projectID) {
		return "", "", fmt.Errorf("%w: %q does not start with a project uuid", ErrInvalidInput, s)
	}
	if _, err := ModuleOfObjectID(id); err != nil {
		return "", "", fmt.Errorf("%w: %q: the part after the project id is not a module-qualified id: %v", ErrInvalidInput, s, err)
	}
	return projectID, id, nil
}

// IsFullName reports whether s parses as a full name.
func IsFullName(s string) bool {
	_, _, err := ParseFullName(s)
	return err == nil
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
				return false
			}
		}
	}
	return true
}
