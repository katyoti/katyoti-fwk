// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// Path is the parsed form of an authoring slot URI. Every Sender call goes
// through ParsePath to convert the string the caller wrote into the four
// tokens that build the NATS subject.
//
// Wire form: `slot.<Provider>.<Slottype>.<Instance>.<Method>` (all kebab).
//
// All four fields MUST be non-empty after ParsePath returns; the parser
// fills in defaults (`Instance = "default"`, `Provider = self`) when the
// authoring URI omits them. An empty field after parse is a parser bug.
type Path struct {
	Provider string // kebab provider (e.g. "ktw-agents") — from ctx self-provider when authoring URI starts ":"
	Slottype string // kebab bus name (e.g. "terminal", "resource-service") — derived from Type
	Instance string // named instance (e.g. "default", "leanix-prod")
	Method   string // kebab method path: one segment, or several joined by '/'
}

// Subject reconstructs the canonical authoring form
// `<Provider>:<Type>[(<Instance>)]/<Method>` per the command-surface decision
// §2.2. The `(default)` instance is omitted (it's the implicit value).
// Used for logging + error messages.
func (p Path) Subject() string {
	instSuffix := ""
	if p.Instance != "" && p.Instance != defaultInstance {
		instSuffix = "(" + p.Instance + ")"
	}
	return p.Provider + ":" + p.Slottype + instSuffix + "/" + p.Method
}

// ParsePath parses an authoring slot URI into its four tokens.
//
// Canonical grammar (locked by rule):
//
//	path     := [PROVIDER] ':' TYPE [ '(' INSTANCE ')' ] '/' METHOD
//	PROVIDER := token  (kebab; empty before ':' → resolved from ctx via SelfProviderFromContext)
//	TYPE     := PascalCase or kebab — kebabed for wire
//	INSTANCE := token  (default: literal "default")
//	METHOD   := SEGMENT { '/' SEGMENT }  (one or more segments: `tasks/dossier/refine-scope`)
//	SEGMENT  := PascalCase or kebab — kebabed for wire
//	token    := [a-z0-9-]{1,63}
//
// Examples (assuming SelfProviderFromContext(ctx) returns "ktw-agents"):
//
//	"ktw-agents:Terminal(default)/StartTask" → {ktw-agents, terminal, default, start-task}
//	"ktw-agents:Terminal/StartTask"          → {ktw-agents, terminal, default, start-task}
//	":Terminal/StartTask"                    → {ktw-agents, terminal, default, start-task}
//	"ktw-hub:Catalog/ListResources"          → {ktw-hub, catalog, default, list-resources}
//
// Errors:
//   - missing `:`                  → "missing ':' separator"
//   - empty parens                 → "empty instance parens"
//   - missing method               → "missing '/Method' suffix"
//   - empty provider with no ctx   → "empty provider and no self-provider in ctx"
//   - any resulting token violating [a-z0-9-]{1,63} → "invalid token"
//
// Legacy `:/` prefix (the form used before 2026-05-23) is accepted and
// stripped silently to keep in-flight code working through the cutover.
// The deprecation removal is tracked by the URI-grammar-unification
// follow-up; new callers MUST emit the canonical `:` form.
func ParsePath(ctx context.Context, raw string) (Path, error) {
	// Find the mandatory ':' separator. It splits provider from the rest.
	colonIdx := strings.IndexByte(raw, ':')
	if colonIdx < 0 {
		return Path{}, fmt.Errorf("messagebus: path %q missing ':' separator (want \"<provider>:Type(instance)/Method\")", raw)
	}
	provider := raw[:colonIdx]
	rest := raw[colonIdx+1:]

	// Legacy compatibility: strip a single leading '/' if present — the old
	// resource-shaped tool form. New code MUST emit the no-slash canonical form.
	rest = strings.TrimPrefix(rest, "/")

	// Method is everything after the FIRST '/': a type or an instance
	// never holds one, so a method of several segments (`a/b/c`) keeps them.
	slashIdx := strings.IndexByte(rest, '/')
	if slashIdx < 0 {
		return Path{}, fmt.Errorf("messagebus: path %q missing '/Method' suffix", raw)
	}
	typeAndInstance := rest[:slashIdx]
	method := rest[slashIdx+1:]
	if method == "" {
		return Path{}, fmt.Errorf("messagebus: path %q has empty method", raw)
	}

	// typeAndInstance is either "Type" or "Type(Instance)".
	var typeStr, instance string
	if openIdx := strings.IndexByte(typeAndInstance, '('); openIdx >= 0 {
		if !strings.HasSuffix(typeAndInstance, ")") {
			return Path{}, fmt.Errorf("messagebus: path %q has '(' but no matching ')'", raw)
		}
		typeStr = typeAndInstance[:openIdx]
		instance = typeAndInstance[openIdx+1 : len(typeAndInstance)-1]
		if instance == "" {
			return Path{}, fmt.Errorf("messagebus: path %q has empty instance parens; omit them to default to %q", raw, defaultInstance)
		}
	} else {
		typeStr = typeAndInstance
		instance = defaultInstance
	}

	if typeStr == "" {
		return Path{}, fmt.Errorf("messagebus: path %q has empty Type", raw)
	}

	// Default provider from ctx if empty before ':'.
	if provider == "" {
		var ok bool
		provider, ok = application.SelfProviderFromContext(ctx)
		if !ok {
			return Path{}, fmt.Errorf("messagebus: path %q has empty provider but no self-provider in ctx (bind Application or call WithSelfProvider)", raw)
		}
	}

	p := Path{
		Provider: provider,
		Slottype: toKebab(typeStr),
		Instance: instance,
		Method:   kebabMethod(method),
	}

	if err := validateToken("provider", p.Provider); err != nil {
		return Path{}, fmt.Errorf("messagebus: path %q: %w", raw, err)
	}
	if err := validateToken("slottype", p.Slottype); err != nil {
		return Path{}, fmt.Errorf("messagebus: path %q: %w", raw, err)
	}
	if err := validateToken("instance", p.Instance); err != nil {
		return Path{}, fmt.Errorf("messagebus: path %q: %w", raw, err)
	}
	for _, seg := range strings.Split(p.Method, "/") {
		if err := validateToken("method", seg); err != nil {
			return Path{}, fmt.Errorf("messagebus: path %q: %w", raw, err)
		}
	}
	return p, nil
}

// kebabMethod kebabs each `/`-separated segment of a method path, so
// `Dossier/RefineScope` becomes `dossier/refine-scope`.
func kebabMethod(method string) string {
	segs := strings.Split(method, "/")
	for i, seg := range segs {
		segs[i] = toKebab(seg)
	}
	return strings.Join(segs, "/")
}

// defaultInstance is the literal token assigned when the authoring URI
// omits `(instance)`. Matches the in-process service manager's default
// name (every `PublishSimpleService` factory produces this instance).
const defaultInstance = "default"

// toKebab is the PascalCase → kebab conversion used at parse time.
// Delegates to meta.DefaultBusName so the algorithm (and its acronym
// disambiguation rules) is shared with ServiceDef.BusName defaulting.
// Idempotent on already-kebab inputs.
func toKebab(name string) string {
	return meta.DefaultBusName(name)
}

// validateToken enforces the [a-z0-9-]{1,63} charset on a fully-resolved
// token (i.e. AFTER kebab conversion). Mirrors mustToken in the subject
// builders but returns an error instead of panicking — the parser
// converts these into typed errors the caller can decide to log vs
// retry vs surface to the user.
func validateToken(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is empty", name)
	}
	if len(value) > 63 {
		return fmt.Errorf("%s %q exceeds 63 chars", name, value)
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return fmt.Errorf("%s %q contains invalid char %q (must match [a-z0-9-]{1,63})", name, value, string(r))
		}
	}
	return nil
}
