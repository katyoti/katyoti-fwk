// SPDX-License-Identifier: Apache-2.0

// Package meta provides pure data types for describing service APIs.
// No framework imports — safe to use in api/ packages.
package meta

import (
	"strings"
	"unicode"
)

// ServiceDef describes a service interface: its identity, methods, and metadata.
// Declared in the api/ package next to the interface definition.
// Used by PublishService, KTSP CLI, HTTP endpoints, agent plugins, and docs.
//
// Two name fields, intentionally separate:
//   - Name    — PascalCase Go-source form (e.g. "LLMProfiles"). Stable
//     identifier for in-process service resolution and docs.
//   - BusName — kebab-case wire form (e.g. "llm-profiles"). The token
//     that appears in NATS slot subjects per the named-slot-addressing
//     scheme `slot.<provider>.<bus-name>.<instance>.<method>`.
//
// BusName is stored explicitly (rather than mechanically derived on every
// use) because mechanical kebab→PascalCase round-trips lose acronym
// information: `LLMProfiles` → `llm-profiles` would reverse-derive as
// `LlmProfiles`. Keeping both forms in the def is unambiguous AND lets
// authors override the default kebab when they have a more readable
// canonical form in mind (e.g. `"resources"` instead of `"resource-service"`).
//
// When BusName is left empty in a literal declaration, callers should
// resolve it via `DefaultBusName(Name)` at publish time.
type ServiceDef struct {
	// Name is the PascalCase service identifier (e.g., "LLMProfiles",
	// "ResourceService"). Used as the KTSP slot type identifier in
	// in-process dispatch, manifest entries, and docs.
	Name string

	// BusName is the kebab-case wire form used in NATS slot subjects
	// (e.g., "llm-profiles", "resource-service"). When empty, callers
	// MUST default it via DefaultBusName(Name) at construction time.
	// Explicit BusName overrides the default — use this when the Go
	// type name contains acronyms or when a shorter canonical bus form
	// is preferred (e.g., Name="ResourceService" + BusName="resources").
	BusName string

	// Short is a one-line summary (for CLI help, agent skill lists).
	Short string

	// Long is a detailed description (for CLI --help, skill instructions, docs).
	Long string

	// Version is the service API version (e.g., "1.0.0").
	Version string

	// Methods maps method names (PascalCase) to their definition.
	Methods map[string]MethodDef

	// CLIGroups declares the noun-verb command groups used by methods whose
	// MethodDef.CLIName is a multi-segment path (e.g. "page pull"). Each entry
	// annotates one group by its FULL space-separated path so nested groups can
	// each carry their own Short help + Aliases (e.g. {Name:"page", Aliases:
	// ["pg"]}). Optional: groups referenced by a CLIName path but absent here
	// are created as bare parent commands (name only). CLI-only — KTSP/HTTP/MCP
	// addressing always uses the flat kebab method name.
	CLIGroups []CLIGroup
}

// CLIGroup annotates a CLI command group (a parent command that holds
// subcommands). Name is the group's full space-separated path from the root
// (e.g. "page" or "grp1 grp2"), matching the leading segments of the member
// methods' CLIName paths.
type CLIGroup struct {
	Name    string   // full space-separated group path (e.g. "page", "grp1 grp2")
	Aliases []string // optional shortcuts for this group (e.g. ["pg"])
	Short   string   // optional one-line help for the group command
}

// ResolvedBusName returns the explicit BusName when set, falling back to
// DefaultBusName(Name) otherwise. Callers that need the canonical wire
// token should call this instead of reading BusName directly — it
// guarantees a populated value for every ServiceDef.
func (d ServiceDef) ResolvedBusName() string {
	if d.BusName != "" {
		return d.BusName
	}
	return DefaultBusName(d.Name)
}

// DefaultBusName converts a PascalCase identifier to the kebab-case wire
// form used in NATS slot subjects.
//
// Rules:
//   - Insert "-" before each uppercase letter that follows a lowercase
//     letter or digit ("StartTask" → "start-task").
//   - Insert "-" between the last uppercase letter of an acronym run and
//     the next word ("LLMProfiles" → "llm-profiles", "RESTAPIClient" →
//     "rest-api-client").
//   - Lowercase the result.
//   - Already-kebab input is idempotent ("resource-service" stays).
//
// For ambiguous acronym cases the explicit BusName field on ServiceDef
// is the disambiguation point — this default is a convenience for the
// common case.
func DefaultBusName(name string) string {
	if name == "" {
		return ""
	}
	// Already kebab? (no uppercase, no spaces, contains only [a-z0-9-]).
	if isLowerKebab(name) {
		return name
	}
	var b strings.Builder
	b.Grow(len(name) + 4)
	runes := []rune(name)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			// Boundary rules:
			//   1) lower→Upper (or digit→Upper): insert separator.
			//      Example: "tas[t]A" → "task-a" — the 'A' starts a new
			//      word after a lowercase/digit run.
			//   2) Upper→Upper-followed-by-2+-lower: insert separator
			//      BEFORE this Upper.
			//      Example: "LLM[P]rofiles" — P starts a new CamelCase
			//      word "Profiles" (long lowercase tail signals a word).
			//   3) Upper→Upper-followed-by-only-one-trailing-lower
			//      ("APIs", "URLs", "IDs"): NO split — the single
			//      trailing lowercase is a plural/suffix glued to the
			//      acronym, not a new word.
			if !unicode.IsUpper(prev) && prev != '-' {
				b.WriteRune('-')
			} else if i+1 < len(runes) && unicode.IsLower(runes[i+1]) &&
				i+2 < len(runes) && unicode.IsLower(runes[i+2]) {
				b.WriteRune('-')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// isLowerKebab reports whether s is already in [a-z0-9-] form (no
// uppercase, no spaces, no other punctuation). Used by DefaultBusName
// for idempotency.
func isLowerKebab(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}

// Description returns Short (preferred) or Name as fallback.
func (d ServiceDef) Description() string {
	if d.Short != "" {
		return d.Short
	}
	return d.Name
}

// MethodDef describes a single service method.
type MethodDef struct {
	// Short is a one-line summary (for CLI help, tool descriptions).
	Short string

	// Long is a detailed description (for CLI --help, skill instructions).
	Long string

	// Hidden hides this method from CLI generation (still available via KTSP/HTTP).
	Hidden bool

	// Deprecated marks this method as deprecated with a message.
	Deprecated string

	// RenderTemplate is a Go text/template string for rich CLI output.
	// When set, the auto-generated CLI command uses console.RenderConsole
	// with this template instead of raw JSON output.
	// The template receives the method's output struct as its data.
	// Leave empty for default JSON output.
	RenderTemplate string

	// Live marks a long-running method that emits data snapshots (via
	// progress.Emit) as it runs. When set AND the method renders via a
	// `ui-cli` extension, the auto-generated CLI installs a live terminal
	// renderer (the terminal's own live view): on a TTY in rich
	// mode the rich screen re-renders in place on each snapshot; piped /
	// `--out json` output is unchanged (the final result prints once). No
	// snapshots emitted ⇒ identical to a non-live method (safe default).
	Live bool

	// CLIName overrides the command name when the method is exposed as a CLI
	// subcommand. Empty means use the kebab-cased Go method name (the default).
	// Useful when the Go name reads awkwardly under a CLI group — e.g.
	// `TaskLibrary.ListTasks` becomes `task list` instead of `task list-tasks`.
	CLIName string

	// InputType is a data-type reference — the name a caller uses to
	// look up the input schema via `<provider>:data-type/<InputType>`.
	// Populated by AutoKTSPContainer from the Go input type's PascalCase
	// name (e.g. `"LoadInput"`). The actual AST lives in the
	// `contribution.DataTypePoint` registry; method metadata only carries
	// the reference. Empty when the method takes no input.
	//
	// Phase C of the contribution unification: one schema per Go type
	// across the whole topology, addressable via the unified bus surface.
	InputType string

	// OutputType is the analogous reference for the method's return
	// type. Empty when the method returns no value.
	OutputType string

	// TimeoutS is the per-method request timeout in seconds, applied by
	// callers that dispatch this method over the bus (the toolproxy /
	// ActionService path). Zero means "use the transport default" (the
	// NATS adapter's 5s fallback). Declare a larger value for methods
	// that legitimately run long — e.g. LeanIX GraphQL traversals — so
	// the caller waits for the reply instead of giving up at 5s.
	TimeoutS int

	// ReadOnly declares that the method changes nothing: it only reads.
	// It travels as the method tag `read-only` into the manifest, the action
	// service reports it (Describe) and writes no invocation record for it,
	// and only such a method may serve as a guide recipe's check.
	//
	// Omitted from JSON when false: a service's definition is published as
	// a contribution, and a new false field on every method would change
	// every service's hash while changing nothing it declares.
	ReadOnly bool `json:",omitempty"`

	// LongRunning declares a method that runs longer than a person waits for
	// : run from the action surface it starts a
	// method run that owns a display panel and answers its run and its panel
	// at once; the method reports into the panel and the person may cancel
	// it. It travels as the method tag `long` into the manifest. Omitted from
	// JSON when false, as ReadOnly.
	LongRunning bool `json:",omitempty"`

	// NoLLM keeps the method off every LLM-facing tool surface — the MCP
	// tools, the sidecar agent plugin, `llm tools list|call` and `sc tools
	// list` — for an act a person makes (a confirmation). It travels as the
	// method tag `mcp=false`; the method stays callable over KTSP, HTTP and
	// its CLI command. Omitted from JSON when false, as ReadOnly.
	NoLLM bool `json:",omitempty"`
}

// Description returns Short (preferred) or empty string.
func (d MethodDef) Description() string {
	return d.Short
}

// Method creates a MethodDef with a short description (most common case).
func Method(short string) MethodDef {
	return MethodDef{Short: short}
}

// MethodLong creates a MethodDef with short and long descriptions.
func MethodLong(short, long string) MethodDef {
	return MethodDef{Short: short, Long: long}
}

// HiddenMethod creates a hidden MethodDef (not shown in CLI).
func HiddenMethod(short string) MethodDef {
	return MethodDef{Short: short, Hidden: true}
}
