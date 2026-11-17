// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// EventURI is the typed, validated form of a subscribable lifecycle
// event identifier — the third URI flavour alongside contribapi.ResourceURI
// (readable) and ToolURI (callable). cwm-v2-providers US-15 (2026-06-08).
//
// Canonical wire form: `<provider>:!<scope>/<path>`.
//
// Discriminator from the other two flavours: the byte immediately after
// `:`. `/` → resource; `!` → event; any other token byte → tool. Single-
// byte lookahead, same parse cost as the existing two-arm dispatch.
//
// Scope ∈ {tenant, universe, workspace, session, room} — matches the
// existing NATS subject scope hierarchy declared in
// `modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md`. The scope token
// determines which contextual ID `Subject()` consumes:
//
//   - tenant    → uses only tenantID
//   - universe  → uses tenantID + universeID
//   - workspace → uses tenantID + universeID + scopeID (= workspaceID)
//   - session   → uses tenantID + universeID + scopeID (= sessionID)
//   - room      → uses tenantID + universeID + scopeID (= roomID)
//
// Path segments after the scope are kebab tokens joined by `/`. Each
// segment may also be the literal `*` (NATS single-token wildcard) to
// build subscription URIs that match many concrete events. The
// `>`-style rest-of-path wildcard is NOT accepted at the URI surface
// in v1 — for that, use the underlying subject string directly.
//
// Examples:
//
//	cwm:!workspace/object/abc123/created                    # specific
//	cwm:!workspace/object/* /created                         # any node, this action
//	cwm:!session/task/run-xyz/started                     # task lifecycle
//	cwm:!session/task/* /done                              # any task completion
//	cwm:!universe/connstate-changed                       # universe-scoped event
//	cwm:!tenant/billing-changed                           # tenant-scoped event
//
// Grammar: [modules/framework/kts/uri-grammar-and-vocabulary.md](../../../../modules/framework/kts/uri-grammar-and-vocabulary.md).
type EventURI struct {
	Provider string   // owning binary (e.g. "cwm"); empty = current binary
	Scope    string   // tenant | universe | workspace | session | room
	Path     []string // kebab tokens (or `*` wildcards) after the scope
}

// EventScope is the strict set of allowed Scope values. Matches the
// substrate's NATS subject hierarchy 1:1 — never introduce a new scope
// without updating the corresponding subject builders in
// `internal/extensions/cwm/api/subjects.go` +
// `internal/extensions/task/api/engine/subjects.go`.
var validEventScopes = map[string]struct{}{
	"tenant":    {},
	"universe":  {},
	"workspace": {},
	"session":   {},
	"room":      {},
}

// NewEventURI validates inputs and returns the typed URI.
func NewEventURI(provider, scope string, path []string) (EventURI, error) {
	if provider != "" {
		if err := contribapi.ValidateKebabToken("provider", provider); err != nil {
			return EventURI{}, fmt.Errorf("%w: %s", ErrInvalidEventURI, err)
		}
	}
	if _, ok := validEventScopes[scope]; !ok {
		return EventURI{}, fmt.Errorf("%w: unknown scope %q (want one of tenant|universe|workspace|session|room)", ErrInvalidEventURI, scope)
	}
	if len(path) == 0 {
		return EventURI{}, fmt.Errorf("%w: empty path after scope", ErrInvalidEventURI)
	}
	for i, seg := range path {
		if err := validateEventSegment(seg); err != nil {
			return EventURI{}, fmt.Errorf("%w: path[%d]: %s", ErrInvalidEventURI, i, err)
		}
	}
	return EventURI{
		Provider: provider,
		Scope:    scope,
		Path:     append([]string(nil), path...), // defensive copy
	}, nil
}

// validateEventSegment accepts strict kebab tokens OR the literal `*`
// (NATS single-token wildcard). Empty segments are rejected.
func validateEventSegment(seg string) error {
	if seg == "*" {
		return nil
	}
	return contribapi.ValidateKebabToken("segment", seg)
}

// ErrInvalidEventURI is returned by ParseEventURI / EventURIFromSubject
// when the string doesn't match the expected shape.
var ErrInvalidEventURI = fmt.Errorf("event URI: invalid format")

// String returns the canonical wire form.
func (u EventURI) String() string {
	return u.Provider + ":!" + u.Scope + "/" + strings.Join(u.Path, "/")
}

// IsZero reports whether the URI is the zero value.
func (u EventURI) IsZero() bool {
	return u.Provider == "" && u.Scope == "" && len(u.Path) == 0
}

// ParseEventURI parses a string into a typed EventURI. The string MUST
// be an event URI (the byte after `:` is `!`).
//
// Grammar:
//
//	uri      := PROVIDER ':!' SCOPE '/' PATH
//	PROVIDER := [a-z0-9-]{0,63}                       (empty = caller resolves)
//	SCOPE    := tenant | universe | workspace | session | room
//	PATH     := SEGMENT ('/' SEGMENT)*
//	SEGMENT  := [a-z0-9-]{1,63} | '*'                  (`*` is the NATS single-token wildcard)
func ParseEventURI(s string) (EventURI, error) {
	colon := strings.IndexByte(s, ':')
	if colon < 0 {
		return EventURI{}, fmt.Errorf("%w: %q missing ':'", ErrInvalidEventURI, s)
	}
	provider := s[:colon]
	rest := s[colon+1:]
	if !strings.HasPrefix(rest, "!") {
		return EventURI{}, fmt.Errorf("%w: %q is not an event URI (byte after ':' is not '!')", ErrInvalidEventURI, s)
	}
	rest = rest[1:]
	scopeSlash := strings.IndexByte(rest, '/')
	if scopeSlash < 0 {
		return EventURI{}, fmt.Errorf("%w: %q missing '/<path>' after scope", ErrInvalidEventURI, s)
	}
	scope := rest[:scopeSlash]
	pathStr := rest[scopeSlash+1:]
	if pathStr == "" {
		return EventURI{}, fmt.Errorf("%w: %q empty path", ErrInvalidEventURI, s)
	}
	path := strings.Split(pathStr, "/")
	return NewEventURI(provider, scope, path)
}

// IsEventURI is the cheap classifier: true iff the byte after `:` is
// `!`. Does NOT validate tokens.
func IsEventURI(s string) bool {
	colon := strings.IndexByte(s, ':')
	if colon < 0 || colon == len(s)-1 {
		return false
	}
	return s[colon+1] == '!'
}

// Subject maps the EventURI to its NATS subject. scopeID is consumed
// per-scope:
//
//   - scope=tenant    → scopeID unused (pass "")
//   - scope=universe  → scopeID unused (pass "")
//   - scope=workspace → scopeID = workspaceID
//   - scope=session   → scopeID = sessionID
//   - scope=room      → scopeID = roomID
//
// Path segments are joined with `.` so NATS wildcards apply per
// segment. Subscribers pass the result directly to
// `EventBus.Subscribe(topic, handler)`.
//
// Subject shapes (matching the substrate's existing builders):
//
//	tenant    → ktw.<t>.<path>
//	universe  → ktw.<t>.universe.<u>.<path>
//	workspace → ktw.<t>.universe.<u>.workspace.<ws>.<path>
//	session   → ktw.<t>.universe.<u>.session.<sid>.<path>
//	room      → ktw.<t>.universe.<u>.room.<rid>.<path>
func (u EventURI) Subject(tenantID, universeID, scopeID string) string {
	pathSubject := strings.Join(u.Path, ".")
	switch u.Scope {
	case "tenant":
		return "ktw." + tenantID + "." + pathSubject
	case "universe":
		return "ktw." + tenantID + ".universe." + universeID + "." + pathSubject
	case "workspace":
		return "ktw." + tenantID + ".universe." + universeID + ".workspace." + scopeID + "." + pathSubject
	case "session":
		return "ktw." + tenantID + ".universe." + universeID + ".session." + scopeID + "." + pathSubject
	case "room":
		return "ktw." + tenantID + ".universe." + universeID + ".room." + scopeID + "." + pathSubject
	}
	// Unreachable for validated EventURI; defensive empty.
	return ""
}
