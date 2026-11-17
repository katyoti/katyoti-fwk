// SPDX-License-Identifier: Apache-2.0

// Package permrules implements the permission rule matching engine used
// by the kernel permission gate (applyPermissionGate in
// internal/extensions/agent/internal/runtime/permission_gate.go) to
// evaluate user-configured permission profiles against tool calls.
//
// A Rule has the format:
//
//   - — match any tool, any input
//     <tool>:**                         — match this tool, any input
//     <tool>.<field>:<glob>             — match this tool when dimensions[field] matches glob
//     *.<field>:<glob>                  — match any tool that has the named dimension
//
// The glob is doublestar-compatible (`*`, `**`, `?`, `[...]`, brace
// alternation). Tool name supports the literal `*` wildcard for "any
// tool" but not partial-name globs (tool names have no separators
// where partial matching would be useful).
//
// Tools populate `PermissionResult.Dimensions` (a map[string]string)
// when they want to participate in rule matching. The kernel falls
// back to a default extractor (top-level JSON string fields) when a
// tool doesn't populate Dimensions itself.
//
// Rules live in two places:
//   - Permission profiles, published as `permission-profile`
//     contributions (ProfileSpec) and read from the catalog as static
//     baselines.
//   - Tool-proposed `PermissionSuggestion.Rule` strings, which the
//     user can promote to "always allow / always deny" decisions
//     via the PromptPermission UI. Promoted rules are added to the
//     active session profile in front of the base rules so they
//     shadow them (first-match-wins).
package permrules

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Rule is a parsed permission rule. Construct via ParseRule; the
// zero-value is invalid (use IsValid to check).
type Rule struct {
	// Raw is the original rule string. Preserved for round-tripping
	// (YAML save, log messages, "this rule fired" diagnostics).
	Raw string

	// ToolName is the name of the tool this rule applies to. The
	// literal "*" means "any tool".
	ToolName string

	// Field is the dimension name to match against, or "" if the
	// rule applies to any input for the tool. When Field is empty,
	// ValueGlob MUST be "**" (per ParseRule's validation).
	Field string

	// ValueGlob is the glob applied to dimensions[Field]. When Field
	// is empty, ValueGlob is "**" by construction. The glob uses
	// doublestar syntax — `*` matches any non-separator chars, `**`
	// matches any sequence including separators.
	ValueGlob string
}

// IsValid reports whether the rule is non-empty (has at least a tool name).
func (r Rule) IsValid() bool {
	return r.ToolName != ""
}

// String returns the canonical rule form. Always equal to the Raw
// input for any rule produced by ParseRule.
func (r Rule) String() string {
	if r.Raw != "" {
		return r.Raw
	}
	return r.canonical()
}

// canonical reconstructs the rule string from the parsed fields.
// Used by String when Raw is missing (e.g. rules constructed
// programmatically rather than parsed).
func (r Rule) canonical() string {
	switch {
	case r.ToolName == "*" && r.Field == "":
		return "*"
	case r.Field == "":
		return r.ToolName + ":" + r.ValueGlob
	default:
		return r.ToolName + "." + r.Field + ":" + r.ValueGlob
	}
}

// errInvalidRule is returned by ParseRule for malformed input.
var errInvalidRule = errors.New("permrules: invalid rule")

// ParseRule parses a rule string into a Rule.
//
// Accepted forms:
//
//   - → ToolName="*", Field="", ValueGlob="**"
//     <tool>:**                  → ToolName=<tool>, Field="", ValueGlob="**"
//     <tool>:<glob>              → ToolName=<tool>, Field="", ValueGlob=<glob> ONLY when glob == "**"
//     <tool>.<field>:<glob>      → ToolName=<tool>, Field=<field>, ValueGlob=<glob>
//     *.<field>:<glob>           → ToolName="*", Field=<field>, ValueGlob=<glob>
//
// A rule with no field MUST use "**" as the value (any input). Any
// other value with no field is rejected because it's almost certainly
// a typo (e.g. `file-read:foo` looks like the user meant a path but
// forgot the `.workspace_path` part).
//
// Tool names may contain `_` and alphanumerics; the parser is
// permissive (it doesn't validate the tool name beyond rejecting
// empty strings) so future tool naming conventions don't break
// existing rules.
func ParseRule(s string) (Rule, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Rule{}, fmt.Errorf("%w: empty rule", errInvalidRule)
	}

	// Bare `*` shorthand for any-tool-any-input.
	if s == "*" {
		return Rule{Raw: s, ToolName: "*", ValueGlob: "**"}, nil
	}

	// Every other rule MUST contain `:`.
	colon := strings.Index(s, ":")
	if colon < 0 {
		return Rule{}, fmt.Errorf("%w: missing ':' in %q (use 'tool:**' for any input)", errInvalidRule, s)
	}
	key := s[:colon]
	value := s[colon+1:]
	if key == "" {
		return Rule{}, fmt.Errorf("%w: empty tool name in %q", errInvalidRule, s)
	}
	if value == "" {
		return Rule{}, fmt.Errorf("%w: empty value in %q", errInvalidRule, s)
	}

	// Split key on the first `.` into tool name + optional field.
	var toolName, field string
	if dot := strings.Index(key, "."); dot >= 0 {
		toolName = key[:dot]
		field = key[dot+1:]
		if field == "" {
			return Rule{}, fmt.Errorf("%w: empty field name after '.' in %q", errInvalidRule, s)
		}
	} else {
		toolName = key
	}

	// When no field is given, the only meaningful value is "**".
	// Reject other values to catch typos like `file-read:/etc/foo`
	// (user almost certainly meant `file_read.absolute_path:/etc/foo`).
	if field == "" && value != "**" {
		return Rule{}, fmt.Errorf("%w: rule %q has no field but value %q is not '**' — did you mean to add a '.<field>' between the tool name and ':'?", errInvalidRule, s, value)
	}

	// Validate the glob early so we don't have to error-check on
	// every match call. doublestar.Match returns an error only on
	// malformed patterns; we trigger it here with a sentinel input.
	if _, err := doublestar.Match(value, ""); err != nil {
		return Rule{}, fmt.Errorf("%w: invalid glob %q: %v", errInvalidRule, value, err)
	}

	return Rule{
		Raw:       s,
		ToolName:  toolName,
		Field:     field,
		ValueGlob: value,
	}, nil
}

// MustParseRule is the panicking variant of ParseRule, useful in
// tests and for hard-coded constants.
func MustParseRule(s string) Rule {
	r, err := ParseRule(s)
	if err != nil {
		panic(err)
	}
	return r
}

// Match reports whether the rule matches the given tool call.
//
// The tool name must equal Rule.ToolName (or Rule.ToolName must be
// "*"). When Rule.Field is set, the rule additionally requires
// dimensions[Field] to exist AND to match Rule.ValueGlob via
// doublestar. When Rule.Field is empty, the rule matches any input
// for the tool (Rule.ValueGlob is "**" by construction).
//
// Match never returns an error — the glob was validated by ParseRule
// at construction time, and a malformed glob would have failed
// there. We swallow the unused error from doublestar.Match here for
// the same reason.
func (r Rule) Match(toolName string, dimensions map[string]string) bool {
	if !r.IsValid() {
		return false
	}

	// Tool name check.
	if r.ToolName != "*" && r.ToolName != toolName {
		return false
	}

	// No field → match any input for the tool.
	if r.Field == "" {
		return true
	}

	// Field check: dimensions must contain the named field, and the
	// stored value must match the glob.
	value, ok := dimensions[r.Field]
	if !ok {
		return false
	}
	matched, _ := doublestar.Match(r.ValueGlob, value)
	return matched
}
