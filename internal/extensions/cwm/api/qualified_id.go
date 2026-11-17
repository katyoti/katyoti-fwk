// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"strings"
)

// A QUALIFIED ID names an object of ANOTHER project (
//
//
//	<key> : <module> . <local>
//
// The key is that project's id - its project object's id, unique per tenant,
// never reused. A bare `module.local` is relative to its container: it names
// an object of the project whose folder holds the text that wrote it, so every
// id, edge and commit subject written before several projects existed keeps
// its meaning unchanged. A reference that leaves its project is qualified; one
// inside it stays bare (Reference).
//
// It is not the full name (full_name.go): the full name carries the project's
// uuid and is used where a reference leaves the universe (logs, events,
// exports); the qualified id carries the readable key and is what an author
// writes. ONE PARSER, here: the store set's resolver and every writer of a
// cross-project reference call it.
//
// The parser is called only where an object reference is expected: out of
// that context `<sidecar>:<token>` is a tool URI, which a
// qualified module id (`<key>:<module>`) resembles.

// QualifiedID joins a project key and a module-qualified id.
func QualifiedID(key, id string) string {
	return key + ":" + id
}

// ParseQualifiedID splits a qualified id into its project key and its
// module-qualified id. A bare id (no `:`) is refused - it is not qualified;
// SplitReference answers both forms.
func ParseQualifiedID(s string) (key, id string, err error) {
	s = strings.TrimSpace(s)
	i := strings.Index(s, ":")
	if i < 0 {
		return "", "", fmt.Errorf("%w: %q is not a qualified id (<key>:<module>.<local>)", ErrInvalidInput, s)
	}
	key, id = s[:i], s[i+1:]
	if err := checkProjectKey(key); err != nil {
		return "", "", fmt.Errorf("%w: %q: %v", ErrInvalidInput, s, err)
	}
	if strings.Contains(id, ":") {
		return "", "", fmt.Errorf("%w: %q: one project key only (<key>:<module>.<local>)", ErrInvalidInput, s)
	}
	if _, err := ModuleOfObjectID(id); err != nil {
		return "", "", fmt.Errorf("%w: %q: the part after the project key is not an object id: %v", ErrInvalidInput, s, err)
	}
	return key, id, nil
}

// IsQualifiedID reports whether s parses as a qualified id.
func IsQualifiedID(s string) bool {
	_, _, err := ParseQualifiedID(s)
	return err == nil
}

// SplitReference reads a reference as written: a qualified id answers its key
// and id, a bare id answers "" and itself (relative to its container). A
// malformed qualified id is refused, never read as a bare one.
func SplitReference(s string) (key, id string, err error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, ":") {
		if _, err := ModuleOfObjectID(s); err != nil {
			return "", "", err
		}
		return "", s, nil
	}
	return ParseQualifiedID(s)
}

// Reference is the writer of a reference from an object of project `from` to
// the object id of project `to`: bare inside one project (or when `to` is not
// named), qualified across projects.
func Reference(from, to, id string) string {
	if to == "" || to == from {
		return id
	}
	return QualifiedID(to, id)
}

// checkProjectKey refuses what cannot be a project key: empty, or holding a
// character that separates a reference's parts or a path's.
func checkProjectKey(key string) error {
	if key == "" {
		return fmt.Errorf("empty project key")
	}
	if strings.ContainsAny(key, ".:/\\ \t") {
		return fmt.Errorf("project key %q holds a separator (. : / \\ or a space)", key)
	}
	return nil
}
