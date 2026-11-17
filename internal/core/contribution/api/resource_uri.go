// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"errors"
	"fmt"
	"strings"
)

// ResourceURI is the typed, validated form of a resource identifier.
// Canonical wire form: `<provider>:/<type>/<id>`.
//
// Charset rules:
//   - Provider + Type: strict kebab — [a-z0-9-]{1,63}. These tokens flow
//     into NATS subjects + queue groups; the tight charset matches what
//     `task/api/engine/subjects.go::mustToken` already enforces.
//   - ID: loose — any printable Unicode except '\n' and ':' (the ':'
//     restriction keeps URI parsing unambiguous). Tree paths with '/'
//     and file extensions are explicitly allowed:
//     ktw-docs:/reference/diagrams/c4-context.md
//     ktw-xxx:/data-type/EmailMessage
//
// Construct via ParseURI for strings from the wire; construct via the
// struct literal (validated by NewResourceURI) for code-generated URIs.
type ResourceURI struct {
	Provider string
	Type     string
	ID       string
}

// NewResourceURI validates inputs and returns the typed URI. Provider
// may be empty (caller resolves to current binary via context); Type
// + ID are required.
func NewResourceURI(provider, typ, id string) (ResourceURI, error) {
	if provider != "" {
		if err := ValidateKebabToken("provider", provider); err != nil {
			return ResourceURI{}, err
		}
	}
	if err := ValidateKebabToken("type", typ); err != nil {
		return ResourceURI{}, err
	}
	if err := validateID(id); err != nil {
		return ResourceURI{}, err
	}
	return ResourceURI{Provider: provider, Type: typ, ID: id}, nil
}

// String returns the canonical wire form `<provider>:/<type>/<id>`.
func (u ResourceURI) String() string {
	return u.Provider + ":/" + u.Type + "/" + u.ID
}

// IsZero reports whether the URI is the zero value (every field empty).
// Useful for "not found" sentinels without a separate flag.
func (u ResourceURI) IsZero() bool {
	return u.Provider == "" && u.Type == "" && u.ID == ""
}

// ErrInvalidURI is returned by ParseURI when the string doesn't match
// the `<provider>:/<type>/<id>` shape.
var ErrInvalidURI = errors.New("resource URI: invalid format")

// ParseResourceURI parses a resource-URI string into a typed
// ResourceURI. Strict: wildcards are REJECTED — a resource URI names one
// entry; listing by pattern is the query surface's filter, not a URI form.
// left loose.
//
// To auto-detect resource vs tool URI, use the unified ParseURI in
// tool_uri.go (returns the typed URI discriminated by Kind).
//
// Grammar:
//
//	uri      := PROVIDER ':' '/' TYPE '/' ID
//	PROVIDER := [a-z0-9-]{1,63}
//	TYPE     := [a-z0-9-]{1,63}
//	ID       := any non-':' non-'\n' chars, ≥ 1 char
//
// Examples (all valid):
//
//	ktw-docs:/skill/doc-create
//	ktw-docs:/reference/diagrams/c4-context.md
//	ktw-xxx:/data-type/EmailMessage
//	ktw-example:/file/config/prod.yaml
func ParseResourceURI(s string) (ResourceURI, error) {
	colonSlash := strings.Index(s, ":/")
	if colonSlash < 0 {
		return ResourceURI{}, fmt.Errorf("%w: %q missing ':/' separator", ErrInvalidURI, s)
	}
	provider := s[:colonSlash]
	rest := s[colonSlash+2:]
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return ResourceURI{}, fmt.Errorf("%w: %q missing '/<id>' after type", ErrInvalidURI, s)
	}
	typ := rest[:slash]
	id := rest[slash+1:]
	return NewResourceURI(provider, typ, id)
}

// stdlib `path.Match` rejects '/' as a separator for '*' — we don't want
// that behavior since IDs are path-like by design. Roll our own simple
// recursive impl: fast enough for filter use (small ID strings, glob
// applied once per entry on a list).
func globMatch(glob, s string) bool {
	// Special case: lone "*" trivially matches anything.
	if glob == "*" {
		return true
	}
	// stdlib path.Match enforces that '*' does NOT cross '/'. We want
	// it to cross '/' for tree-like IDs (e.g. "diagrams/c4-*"). So we
	// implement our own.
	gi, si := 0, 0
	starG, starS := -1, -1
	for si < len(s) {
		switch {
		case gi < len(glob) && (glob[gi] == s[si] || glob[gi] == '?'):
			gi++
			si++
		case gi < len(glob) && glob[gi] == '*':
			starG = gi
			starS = si
			gi++
		case starG != -1:
			gi = starG + 1
			starS++
			si = starS
		default:
			return false
		}
	}
	for gi < len(glob) && glob[gi] == '*' {
		gi++
	}
	return gi == len(glob)
}

// ValidateKebabToken enforces [a-z0-9-]{1,63} on a single token.
func ValidateKebabToken(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s: empty", name)
	}
	if len(value) > 63 {
		return fmt.Errorf("%s %q: exceeds 63 chars", name, value)
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return fmt.Errorf("%s %q: invalid char %q (kebab charset is [a-z0-9-])", name, value, r)
		}
	}
	return nil
}

// validateID rejects only the two characters that would break URI parsing:
// '\n' (used to delimit JSON-RPC lines etc.) and ':' (the provider/type
// separator). Everything else — including '/', '.', spaces, PascalCase,
// Unicode — is permitted.
func validateID(id string) error {
	if id == "" {
		return errors.New("id: empty")
	}
	for _, r := range id {
		if r == '\n' || r == ':' {
			return fmt.Errorf("id %q: forbidden char %q", id, r)
		}
	}
	return nil
}

// ListFilter is the criteria struct consumed by the ListURIs / index-listing
// (ListContributions) filters. Lives in the same `api` package as ResourceURI so the
// dependency goes one direction (URI → Filter, never the reverse).
//
// Semantics: empty fields = no filter on that dimension. Multiple fields
// = AND intersection. Within a field with multiple values (Providers /
// Types) = OR (any-of).
type ListFilter struct {
	// Providers restricts to entries whose `Provider` field matches
	// any value in this slice. Empty = no provider filter.
	Providers []string `json:"providers,omitempty"`

	// Types restricts to entries whose `Type` field matches any
	// value in this slice. Empty = no type filter.
	Types []string `json:"types,omitempty"`

	// IDGlob is a shell-style glob applied to entries' `ID` field.
	// Empty = no ID filter. Supports `*` (any sequence including '/')
	// and `?` (one char). Concrete strings match exactly.
	IDGlob string `json:"idGlob,omitempty"`
}

// IsEmpty reports whether the filter has no restrictions — when true,
// ListURIs / ListContributions should return every entry.
func (f ListFilter) IsEmpty() bool {
	return len(f.Providers) == 0 && len(f.Types) == 0 && f.IDGlob == ""
}

// Matches reports whether a ResourceURI satisfies the filter. Used by
// LocalCatalogReader to apply the filter against in-memory entries; the
// bus impl (BusCatalogReader) hands the filter to the hub which uses the
// same helper server-side.
func (f ListFilter) Matches(u ResourceURI) bool {
	if len(f.Providers) > 0 && !contains(f.Providers, u.Provider) {
		return false
	}
	if len(f.Types) > 0 && !contains(f.Types, u.Type) {
		return false
	}
	if f.IDGlob != "" && f.IDGlob != "*" && !globMatch(f.IDGlob, u.ID) {
		return false
	}
	return true
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
