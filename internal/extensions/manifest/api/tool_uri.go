// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"fmt"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// ToolURI is the typed, validated form of a callable slot-method
// identifier — the callable-lane counterpart to contribapi.ResourceURI.
//
// Canonical wire form: `<provider>:<slot>[(<instance>)]/<method>`.
//
// Discriminator from contribapi.ResourceURI: the byte immediately after `:`. If
// it's `/`, the string is a resource URI; otherwise it's a tool URI.
//
// Defaults:
//   - Empty Provider → current binary (per-context resolution)
//   - Missing `(instance)` parens → Instance == "default"
//
// Charset: every component is strict kebab — [a-z0-9-]{1,63} —
// matching the subject-token rules in
// `internal/extensions/task/api/engine/subjects.go::mustToken`.
//
// METHOD MAY BE MULTI-SEGMENT (cwm-v2-providers US-15, 2026-06-08):
// the method may be a path of kebab tokens joined by `/`, e.g.
// `ktw-example:tasks/dossier/refine-scope`. Each segment is
// independently kebab-validated. On the NATS-subject side, the `/`
// separators become `.` separators so wildcard subscriptions work at
// each segment level (`slot.ktw-example.tasks.dossier.*` matches
// every dossier task). Single-segment methods are byte-identical
// pre- and post-change.
//
// Grammar: [modules/framework/kts/uri-grammar-and-vocabulary.md](../../../../modules/framework/kts/uri-grammar-and-vocabulary.md).
// Locked by the URI grammar.
type ToolURI struct {
	Provider string // owning binary (e.g. "ktw-docs"); empty = current binary
	SlotType string // KTSP slot-type name in kebab form (e.g. "leanix-service")
	Instance string // named-service instance (e.g. "default"); missing = "default"
	Method   string // method name on the slot (e.g. "search")
}

// ErrInvalidToolURI is returned by ParseToolURI / ToolURIFromSubject
// when the string doesn't match the expected shape or any token fails
// the kebab grammar.
var ErrInvalidToolURI = errors.New("tool URI: invalid format")

// instanceDefault is the implicit instance name when `(instance)` is
// omitted from a tool URI.
const instanceDefault = "default"

// validateMethodPath validates a method path: one or more kebab tokens
// separated by `/`. Empty segments are rejected. Each segment must
// satisfy `contribapi.ValidateKebabToken`. cwm-v2-providers US-15
// (2026-06-08): replaces the prior single-token validation so methods
// can carry path semantics (e.g. `tasks/dossier/refine-scope`).
func validateMethodPath(method string) error {
	if method == "" {
		return fmt.Errorf("method: empty")
	}
	for i, seg := range strings.Split(method, "/") {
		if err := contribapi.ValidateKebabToken(fmt.Sprintf("method[%d]", i), seg); err != nil {
			return err
		}
	}
	return nil
}

// NewToolURI validates inputs and returns the typed URI. Empty
// Instance is normalized to "default". Empty Provider is permitted
// (caller resolves to current binary via context).
func NewToolURI(provider, slotType, instance, method string) (ToolURI, error) {
	if provider != "" {
		if err := contribapi.ValidateKebabToken("provider", provider); err != nil {
			return ToolURI{}, fmt.Errorf("%w: %s", ErrInvalidToolURI, err)
		}
	}
	if err := contribapi.ValidateKebabToken("slotType", slotType); err != nil {
		return ToolURI{}, fmt.Errorf("%w: %s", ErrInvalidToolURI, err)
	}
	if instance == "" {
		instance = instanceDefault
	}
	if err := contribapi.ValidateKebabToken("instance", instance); err != nil {
		return ToolURI{}, fmt.Errorf("%w: %s", ErrInvalidToolURI, err)
	}
	if err := validateMethodPath(method); err != nil {
		return ToolURI{}, fmt.Errorf("%w: %s", ErrInvalidToolURI, err)
	}
	return ToolURI{
		Provider: provider,
		SlotType: slotType,
		Instance: instance,
		Method:   method,
	}, nil
}

// String returns the canonical wire form. Omits the parens entirely
// when Instance is "default" — the common case.
//
//	ktw-example:example-service/search                (default instance)
//	ktw-example:example-service(prod)/search          (explicit instance)
//	:weather-service/get-current                      (provider defaulted to current)
func (u ToolURI) String() string {
	instSuffix := ""
	if u.Instance != "" && u.Instance != instanceDefault {
		instSuffix = "(" + u.Instance + ")"
	}
	return u.Provider + ":" + u.SlotType + instSuffix + "/" + u.Method
}

// IsZero reports whether the URI is the zero value.
func (u ToolURI) IsZero() bool {
	return u.Provider == "" && u.SlotType == "" && u.Instance == "" && u.Method == ""
}

// ParseToolURI parses a string into a typed ToolURI. The string MUST
// be a tool URI (i.e. the byte after `:` is NOT `/`); resource URIs
// error here — use contribapi.ParseResourceURI or ParseURI for the auto-detect
// variant.
//
// Grammar:
//
//	uri      := PROVIDER ':' SLOT [ '(' INSTANCE ')' ] '/' METHOD
//	PROVIDER := [a-z0-9-]{0,63}   (empty = caller resolves)
//	SLOT     := [a-z0-9-]{1,63}
//	INSTANCE := [a-z0-9-]{1,63}   (missing parens → "default")
//	METHOD   := SEGMENT ('/' SEGMENT)*
//	SEGMENT  := [a-z0-9-]{1,63}
//
// Examples (all valid):
//
//	ktw-weather:weather-service/get-forecast
//	ktw-docs:script-service/doc-create-build
//	ktw-example:catalog-service(prod)/search
//	:weather-service/get-current
//	ktw-example:tasks/dossier/refine-scope          (multi-segment method)
//	ktw-agents:tasks/inbox/seed-default-items        (multi-segment method)
func ParseToolURI(s string) (ToolURI, error) {
	provider, slot, instance, method, err := splitToolURI(s)
	if err != nil {
		return ToolURI{}, err
	}
	return NewToolURI(provider, slot, instance, method)
}

// splitToolURI splits the wire form `<provider>:<slot>[(<instance>)]/<method>`
// into its raw components WITHOUT validating any token. It only enforces the
// structural shape (the `:` discriminator, the slot/method `/`, and balanced
// instance parens). Token-charset validation is the caller's job — this lets
// ParseToolURI validate strictly while ParseToolPattern relaxes the method to
// allow glob metacharacters.
func splitToolURI(s string) (provider, slot, instance, method string, err error) {
	colon := strings.IndexByte(s, ':')
	if colon < 0 {
		return "", "", "", "", fmt.Errorf("%w: %q missing ':'", ErrInvalidToolURI, s)
	}
	provider = s[:colon]
	rest := s[colon+1:]
	if strings.HasPrefix(rest, "/") {
		return "", "", "", "", fmt.Errorf("%w: %q is a resource URI (has '/' after ':')", ErrInvalidToolURI, s)
	}
	// Find the '/' separating slot[(instance)] from method.
	methodSlash := strings.IndexByte(rest, '/')
	if methodSlash < 0 {
		return "", "", "", "", fmt.Errorf("%w: %q missing '/<method>' after slot", ErrInvalidToolURI, s)
	}
	slotPart := rest[:methodSlash]
	method = rest[methodSlash+1:]
	// Optional (instance) suffix on slot part.
	slot = slotPart
	if openParen := strings.IndexByte(slotPart, '('); openParen >= 0 {
		if !strings.HasSuffix(slotPart, ")") {
			return "", "", "", "", fmt.Errorf("%w: %q malformed instance parens", ErrInvalidToolURI, s)
		}
		slot = slotPart[:openParen]
		instance = slotPart[openParen+1 : len(slotPart)-1]
	}
	return provider, slot, instance, method, nil
}

// globMetachars are the doublestar metacharacters that mark a METHOD as a
// glob pattern for bulk slot-method selection (e.g. `*`, `prefix-*`, `{a,b}`).
const globMetachars = "*?[{"

// ParseToolPattern is ParseToolURI relaxed to accept a glob METHOD for bulk
// slot-method selection — the form an `agent:` block's `tools` and task `tools:`
// use to pull "every method of a slot" (`ktw-example:example/*`). Provider, slot
// and instance stay strict kebab; a glob method is returned VERBATIM (not kebab-
// validated) so the caller can match it against the slot's advertised method
// names. A non-glob method is validated exactly like ParseToolURI, so callers
// may use ParseToolPattern uniformly for both exact and wildcard entries.
//
// This is the parser the tool proxy MUST use: ParseToolURI rejects `*` (it is
// not a kebab char), which would otherwise make the wildcard branch unreachable.
func ParseToolPattern(s string) (ToolURI, error) {
	provider, slot, instance, method, err := splitToolURI(s)
	if err != nil {
		return ToolURI{}, err
	}
	if !strings.ContainsAny(method, globMetachars) {
		// Exact method — validate strictly, identical to ParseToolURI.
		return NewToolURI(provider, slot, instance, method)
	}
	// Glob method — validate the structural tokens, leave the method verbatim.
	if provider != "" {
		if err := contribapi.ValidateKebabToken("provider", provider); err != nil {
			return ToolURI{}, fmt.Errorf("%w: %s", ErrInvalidToolURI, err)
		}
	}
	if err := contribapi.ValidateKebabToken("slotType", slot); err != nil {
		return ToolURI{}, fmt.Errorf("%w: %s", ErrInvalidToolURI, err)
	}
	if instance == "" {
		instance = instanceDefault
	}
	if err := contribapi.ValidateKebabToken("instance", instance); err != nil {
		return ToolURI{}, fmt.Errorf("%w: %s", ErrInvalidToolURI, err)
	}
	return ToolURI{Provider: provider, SlotType: slot, Instance: instance, Method: method}, nil
}

// Subject returns the NATS subject that addresses this tool method in
// the given universe. Mirrors `subjects.SlotSubject` — inlined here to
// keep manifest/api free of cross-extension imports. Round-trip
// pinned by test.
//
//	ktw.<tenant>.universe.<u>.slot.<provider>.<slot>.<instance>.<method>
//
// For multi-segment methods (e.g. `tasks/dossier/refine-scope`), the
// `/` separators become `.` in the subject so wildcard subscriptions
// work at each segment level. Single-segment methods round-trip
// byte-identical. cwm-v2-providers US-15 (2026-06-08).
//
// Caller must supply a non-empty provider (use the binary's own
// provider name if Provider is empty).
func (u ToolURI) Subject(tenantID, universeID, provider string) string {
	p := u.Provider
	if p == "" {
		p = provider
	}
	inst := u.Instance
	if inst == "" {
		inst = instanceDefault
	}
	// Multi-segment methods: `/` in the URI becomes `.` in the
	// subject so NATS wildcards apply per segment.
	methodSubject := strings.ReplaceAll(u.Method, "/", ".")
	return "ktw." + tenantID + ".universe." + universeID +
		".slot." + p + "." + u.SlotType + "." + inst + "." + methodSubject
}

// ToolURIFromSubject is the reverse of Subject. Parses a NATS subject
// shaped like `ktw.<t>.universe.<u>.slot.<provider>.<slot>.<instance>.<method>`
// and returns the corresponding ToolURI (dropping tenant + universe).
//
// Instance == "default" is preserved on the URI for round-trip
// stability; ToolURI.String() collapses it back to the parens-less
// form.
func ToolURIFromSubject(subject string) (ToolURI, error) {
	parts := strings.Split(subject, ".")
	// 9 tokens for single-segment methods; 10+ for multi-segment (each
	// extra method segment adds one token). cwm-v2-providers US-15.
	if len(parts) < 9 {
		return ToolURI{}, fmt.Errorf("%w: subject %q has %d tokens, want at least 9 (ktw.<t>.universe.<u>.slot.<provider>.<slot>.<instance>.<method>[.<segment>…])", ErrInvalidToolURI, subject, len(parts))
	}
	if parts[0] != "ktw" || parts[2] != "universe" || parts[4] != "slot" {
		return ToolURI{}, fmt.Errorf("%w: subject %q fixed-prefix tokens don't match scheme", ErrInvalidToolURI, subject)
	}
	method := strings.Join(parts[8:], "/")
	return NewToolURI(parts[5], parts[6], parts[7], method)
}

// IsToolURI reports whether s is shaped as a tool URI: contains ':'
// followed by a token byte that is neither `/` (resource) nor `!`
// (event). Cheap classifier; does not validate tokens. cwm-v2-providers
// US-15 (2026-06-08): tightened to exclude the new event-URI shape.
func IsToolURI(s string) bool {
	colon := strings.IndexByte(s, ':')
	if colon < 0 {
		return false
	}
	rest := s[colon+1:]
	if rest == "" {
		return false
	}
	if rest[0] == '/' || rest[0] == '!' {
		return false
	}
	return true
}

// ===== Unified URI (resource, tool, or event) =====

// URIKind discriminates the three URI shapes. cwm-v2-providers US-15
// (2026-06-08): added KindEvent alongside the original two.
type URIKind int

const (
	KindResource URIKind = iota
	KindTool
	KindEvent
)

// URI is a typed view of either a contribapi.ResourceURI, a ToolURI, or
// an EventURI, with the Kind field as the discriminator. Use this when
// consumers don't know at parse-time which shape they got (e.g.
// catalog walkers, agent prompt builders that mix readable + callable
// + subscribable lists).
type URI struct {
	Kind     URIKind
	Resource contribapi.ResourceURI // valid when Kind == KindResource
	Tool     ToolURI                // valid when Kind == KindTool
	Event    EventURI               // valid when Kind == KindEvent
}

// ParseURI auto-detects the kind from the byte after ':'. The
// dispatcher is single-byte lookahead:
//
//   - `:/` → resource URI
//   - `:!` → event URI
//   - `:<token>` → tool URI
func ParseURI(s string) (URI, error) {
	colon := strings.IndexByte(s, ':')
	if colon < 0 {
		return URI{}, fmt.Errorf("%w: %q missing ':'", ErrInvalidToolURI, s)
	}
	rest := s[colon+1:]
	if strings.HasPrefix(rest, "/") {
		r, err := contribapi.ParseResourceURI(s)
		if err != nil {
			return URI{}, err
		}
		return URI{Kind: KindResource, Resource: r}, nil
	}
	if strings.HasPrefix(rest, "!") {
		e, err := ParseEventURI(s)
		if err != nil {
			return URI{}, err
		}
		return URI{Kind: KindEvent, Event: e}, nil
	}
	t, err := ParseToolURI(s)
	if err != nil {
		return URI{}, err
	}
	return URI{Kind: KindTool, Tool: t}, nil
}

// String returns the canonical wire form for the active kind.
func (u URI) String() string {
	switch u.Kind {
	case KindResource:
		return u.Resource.String()
	case KindTool:
		return u.Tool.String()
	case KindEvent:
		return u.Event.String()
	}
	return ""
}

// IsResource reports whether the URI carries a resource (path-style).
func (u URI) IsResource() bool { return u.Kind == KindResource }

// IsTool reports whether the URI carries a tool (method-style).
func (u URI) IsTool() bool { return u.Kind == KindTool }

// IsEvent reports whether the URI carries an event (subscribable).
func (u URI) IsEvent() bool { return u.Kind == KindEvent }
