// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"regexp"
)

// The live serving of object-backed entries (decision):
// the process hosting CWM projects every object carrying
// `contributes` (EntryOfObject) and PUSHES the entry to the hub, which serves
// it in the project's member beside its folder files - the hub never reads CWM.

// Push operations.
const (
	// PushPut is an object's entry, replacing the one it published before
	// (under any kind).
	PushPut = "put"
	// PushRemove is an object that publishes nothing any more: deleted, moved
	// away, or no longer carrying the capability.
	PushRemove = "remove"
	// PushSnapshot is every object entry of the project: it replaces the
	// whole set the member held.
	PushSnapshot = "snapshot"
)

// EntriesPush is one message on PushSubject: a put, a remove or a snapshot of
// the object entries a project publishes under its provider token.
type EntriesPush struct {
	Op    string `json:"op"`
	Token string `json:"token"`
	// Object is the object a put or a remove is about.
	Object string `json:"object,omitempty"`
	// Entry is a put's entry.
	Entry *ObjectEntry `json:"entry,omitempty"`
	// Entries are a snapshot's entries (none: the project publishes no object).
	Entries []ObjectEntry `json:"entries,omitempty"`
}

// Validate refuses a push its member cannot apply.
func (p EntriesPush) Validate() error {
	if !pushToken.MatchString(p.Token) {
		return fmt.Errorf("push: token %q is not a kebab token", p.Token)
	}
	switch p.Op {
	case PushPut:
		if p.Entry == nil || p.Object == "" || p.Entry.Object != p.Object {
			return fmt.Errorf("push put: an object and its entry are required")
		}
	case PushRemove:
		if p.Object == "" {
			return fmt.Errorf("push remove: an object is required")
		}
	case PushSnapshot:
	default:
		return fmt.Errorf("push: unknown op %q (put; remove; snapshot)", p.Op)
	}
	return nil
}

var pushToken = regexp.MustCompile(`^[a-z0-9-]{1,63}$`)

func pushMustToken(name, value string) {
	if !pushToken.MatchString(value) {
		panic(fmt.Sprintf("project-contributions subject builder: invalid %s %q (must match [a-z0-9-]{1,63})", name, value))
	}
}

func pushPrefix(tenantID, universeID string) string {
	pushMustToken("tenantID", tenantID)
	pushMustToken("universeID", universeID)
	return fmt.Sprintf("ktw.%s.universe.%s.project-contributions", tenantID, universeID)
}

// PushSubject - `ktw.<t>.universe.<u>.project-contributions.<token>.push`: the
// object entries of the project whose provider token is token (EntriesPush).
func PushSubject(tenantID, universeID, token string) string {
	pushMustToken("token", token)
	return fmt.Sprintf("%s.%s.push", pushPrefix(tenantID, universeID), token)
}

// PushSubjectAll - `ktw.<t>.universe.<u>.project-contributions.*.push`: the
// pushes of every project of the universe (a hub serving several).
func PushSubjectAll(tenantID, universeID string) string {
	return pushPrefix(tenantID, universeID) + ".*.push"
}

// SnapshotRequestSubject - `ktw.<t>.universe.<u>.project-contributions.<token>.snapshot`:
// the hub asks for the project's whole set (it started, or lost it); the
// process hosting CWM answers with a PushSnapshot on PushSubject. The payload
// is empty.
func SnapshotRequestSubject(tenantID, universeID, token string) string {
	pushMustToken("token", token)
	return fmt.Sprintf("%s.%s.snapshot", pushPrefix(tenantID, universeID), token)
}

// SnapshotRequestSubjectAll - `ktw.<t>.universe.<u>.project-contributions.*.snapshot`:
// what the process hosting CWM listens to, whatever project is asked.
func SnapshotRequestSubjectAll(tenantID, universeID string) string {
	return pushPrefix(tenantID, universeID) + ".*.snapshot"
}
