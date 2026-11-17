// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// SCOPE — what a sidecar's contributions may act on.
//
// Only GATE entries are scoped: the entries at a point that may refuse a
// write (a hook-type whose timing is `before` and whose answer is `accept` —
// before_create, before_update, before_delete, on_transition and any such
// point an extension opens). Everything else — the after and on-demand
// points, framing, display — applies universe-wide, always attributed.
//
// A provider OWNS the object types and the capabilities it declares. A gate
// entry from provider P applies to an object whose type P owns, or that
// carries a capability P owns by the object's own choice — its type, an
// aspect, the instance — never by P's own `attach` (a capability putting
// itself on foreign objects does not make them P's). A universe widens it per
// sidecar in Univers.yaml: `sidecars[].grants.gate: {types, capabilities}`.
//
// An entry out of scope is NARROWED, never dropped silently: the object's hook
// table lists it with Applies=false and a reason naming the grant to add, and
// the declaration is a finding (ScopeOf) shown in the universe's health and
// by `ktw-compose lint-universe`. There is no platform exemption.

// Grants is the gate grant of each sidecar, by provider name.
type Grants map[string]cenv.GateGrant

// IsGate reports a point that may refuse a write.
func (t HookType) IsGate() bool {
	return t.Timing == HookTimingBefore && t.Answer == HookAnswerAccept
}

// GatePoints is every gate point: CWM's own and those the declared hook-types
// open.
func GatePoints(declared []HookType) map[HookPoint]bool {
	out := map[HookPoint]bool{}
	for _, t := range BuiltinHookTypes {
		if t.IsGate() {
			out[t.Name] = true
		}
	}
	for _, t := range declared {
		if t.IsGate() {
			out[t.Name] = true
		}
	}
	return out
}

// Scope answers whether a provider's gate entry applies to an object.
type Scope struct {
	typeOwners map[string]map[string]bool
	capOwners  map[string]map[string]bool
	grants     Grants
}

// NewScope builds the scope of the declarations, with the universe's grants.
// A type or capability two providers declare is owned by both.
func NewScope(d Declarations, grants Grants) Scope {
	sc := Scope{typeOwners: map[string]map[string]bool{}, capOwners: map[string]map[string]bool{}, grants: grants}
	for _, t := range d.Objects {
		addOwner(sc.typeOwners, t.Name, t.Provider)
	}
	for _, c := range d.Capabilities {
		addOwner(sc.capOwners, c.Name, c.Provider)
	}
	return sc
}

func addOwner(m map[string]map[string]bool, name, provider string) {
	if name == "" || provider == "" {
		return
	}
	if m[name] == nil {
		m[name] = map[string]bool{}
	}
	m[name][provider] = true
}

// OwnsType reports a type the provider declares or is granted.
func (sc Scope) OwnsType(provider, typ string) bool {
	if sc.typeOwners[typ][provider] {
		return true
	}
	g := sc.grants[provider]
	return contains(g.Types, typ) || contains(g.Types, "*")
}

// OwnsCapability reports a capability the provider declares or is granted.
func (sc Scope) OwnsCapability(provider, capability string) bool {
	return sc.capOwners[capability][provider] || contains(sc.grants[provider].Capabilities, capability)
}

// Applies reports whether a gate entry from provider applies to an object of
// type typ carrying the effective capabilities, and when not, why — naming
// the grant that would widen it. An entry no provider contributed (the
// object's own) always applies.
func (sc Scope) Applies(provider, typ string, effective []CapabilityRef) (bool, string) {
	if provider == "" || sc.OwnsType(provider, typ) {
		return true, ""
	}
	for _, ref := range effective {
		// The provider's own attach does not make the object its own; a
		// granted capability counts however the object carries it.
		if contains(sc.grants[provider].Capabilities, ref.ID) {
			return true, ""
		}
		if ref.Source != "attached" && sc.capOwners[ref.ID][provider] {
			return true, ""
		}
	}
	return false, fmt.Sprintf("out of scope: %s owns neither the type %q nor a capability this object carries by its own choice; "+
		"to let it refuse writes here, grant it in Univers.yaml: sidecars: [{name: %s, grants: {gate: {types: [%s]}}}]",
		provider, typ, provider, typ)
}

// Finding rules a declaration can break.
const (
	// FindingScope: a gate entry reaches objects its provider does not own; it
	// is narrowed to the ones it owns.
	FindingScope = "scope"
	// FindingDecode: a declaration the host could not read; it is skipped.
	FindingDecode = "decode"
	// FindingFormatNewerMinor: a declaration written in a newer minor of its
	// kind's format; the keys this host does not know are ignored.
	FindingFormatNewerMinor = "format-newer-minor"
	// FindingFormatNewerMajor: a declaration written in a newer major of its
	// kind's format; it is refused (update the hub).
	FindingFormatNewerMajor = "format-newer-major"
	// FindingRejected: a declaration read but not registered (an object type
	// whose storage folder would not be scanned, or whose sections do not
	// check against their capabilities).
	FindingRejected = "rejected"
	// FindingAsyncPoint: an async hook entry at a point no event type names;
	// it never applies (it would match no event).
	FindingAsyncPoint = "async-point"
	// FindingEventPoint: an event type whose point a hook-type declares with
	// a timing other than after.
	FindingEventPoint = "event-point"
	// FindingNameClash: a second provider declaring a name another already
	// declares with another body - refused (contract 2).
	FindingNameClash = "name-clash"
	// FindingInvalid: a declaration the catalog marked invalid against its
	// kind's schema - quarantined: not served, the last good one kept.
	FindingInvalid = "invalid"
)

// Finding severities.
const (
	FindingError   = "error"
	FindingWarning = "warning"
)

// DeclarationFinding is one thing wrong with what the catalog declares to
// CWM: a declaration skipped, read leniently or narrowed — and why.
type DeclarationFinding struct {
	Rule     string `json:"rule"               jsonschema:"scope | decode | format-newer-minor | format-newer-major | rejected | async-point | event-point | name-clash | invalid"`
	Severity string `json:"severity"           jsonschema:"error | warning"`
	Kind     string `json:"kind"               jsonschema:"The declaration's kind (hook, capability, policy, object-type, ...)"`
	Name     string `json:"name,omitempty"     jsonschema:"The declaration's name"`
	URI      string `json:"uri,omitempty"      jsonschema:"The contribution's URI"`
	Provider string `json:"provider,omitempty" jsonschema:"The binary that contributed it"`
	Message  string `json:"message"            jsonschema:"What is wrong and what to do"`
}

// String is one line: severity, rule, where, what.
func (f DeclarationFinding) String() string {
	where := f.URI
	if where == "" {
		where = f.Kind + " " + f.Name
	}
	return fmt.Sprintf("%s [%s] %s: %s", f.Severity, f.Rule, where, f.Message)
}

// DeclarationFindingsInput asks what is wrong with the universe's declarations.
type DeclarationFindingsInput struct{}

// DeclarationFindingsOutput is every finding, sorted.
type DeclarationFindingsOutput struct {
	Findings []DeclarationFinding `json:"findings"`
}

// DeclarationFindings is every finding about the declarations: what reading
// them skipped or read leniently (d.Findings) and every gate entry its
// provider's scope narrows (ScopeOf). The one answer CWM's method and the
// universe lint give.
func DeclarationFindings(d Declarations, grants Grants) []DeclarationFinding {
	out := append([]DeclarationFinding(nil), d.Findings...)
	out = append(out, ScopeOf(d, grants)...)
	out = append(out, AsyncPointsOf(d)...)
	out = append(out, EventPointTimingsOf(d)...)
	SortFindings(out)
	return out
}

// AsyncPointsOf is every async hook entry declared at a point no event type
// names — a `hook` contribution's, a capability's, a policy's, an object
// type's. Such an entry is listed by ObjectHooks as not applying.
func AsyncPointsOf(d Declarations) []DeclarationFinding {
	points := EventPoints(d.EventTypes)
	var out []DeclarationFinding
	bad := func(kind, name, provider string, point HookPoint, uri string) {
		out = append(out, DeclarationFinding{Rule: FindingAsyncPoint, Severity: FindingError, Kind: kind, Name: name,
			Provider: provider, Message: fmt.Sprintf("async entry %s at %s: no event type names this point", uri, point)})
	}
	check := func(kind, name, provider string, hooks Hooks) {
		for p, es := range hooks {
			for _, e := range es {
				if e.Async != nil && !points[p] {
					bad(kind, name, provider, p, e.URI)
				}
			}
		}
	}
	for _, h := range d.Hooks {
		if h.Async != nil && !points[h.Type] {
			bad("hook", h.Name, h.Provider, h.Type, h.URI)
		}
	}
	for _, c := range d.Capabilities {
		check("capability", c.Name, c.Provider, c.Hooks)
	}
	for _, p := range d.Policies {
		check("policy", p.Name, p.Provider, p.Hooks)
	}
	for _, t := range d.Objects {
		check("object-type", t.Name, t.Provider, t.Hooks)
	}
	return out
}

// EventPointTimingsOf is every event type whose point a hook-type (CWM's or a
// pack's) declares with a timing other than after: the event fires the point
// after it happened, so a point that also gates a write or runs on demand
// would mean two things.
func EventPointTimingsOf(d Declarations) []DeclarationFinding {
	timing := map[HookPoint]string{}
	for _, t := range BuiltinHookTypes {
		timing[t.Name] = t.Timing
	}
	for _, t := range d.HookTypes {
		timing[t.Name] = t.Timing
	}
	var out []DeclarationFinding
	for _, et := range d.EventTypes {
		if et.Point == "" {
			continue
		}
		if tm, ok := timing[et.Point]; ok && tm != HookTimingAfter {
			out = append(out, DeclarationFinding{Rule: FindingEventPoint, Severity: FindingError, Kind: "event-type", Name: et.Name,
				Provider: et.Provider, Message: fmt.Sprintf("its point %s is a hook-type with timing %s: an event's point fires after the event — name another point", et.Point, tm)})
		}
	}
	return out
}

// SortFindings orders findings by kind, name, rule, message.
func SortFindings(fs []DeclarationFinding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Provider+a.Name != b.Provider+b.Name {
			return a.Provider+a.Name < b.Provider+b.Name
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
}

// ScopeOf is every gate entry whose declaration reaches objects its provider
// does not own — narrowed at run time to the ones it owns. Read from the
// declarations alone, by shape:
//
//   - a `hook` at a gate point: its filter's types must all be owned (or, with
//     no types, its capabilities must all be); a filter naming neither reaches
//     every object;
//   - a capability with gate entries that ATTACHES itself: what its attach
//     selects must be owned — its own attach does not make an object its own;
//   - a policy with gate entries: what it applies to must be owned.
//
// A type's own entries are in scope by definition, and an object's own have
// no provider. Empty for every pack the platform ships (a whole-repo test
// holds it).
func ScopeOf(d Declarations, grants Grants) []DeclarationFinding {
	sc := NewScope(d, grants)
	gates := GatePoints(d.HookTypes)
	var out []DeclarationFinding
	for _, h := range d.Hooks {
		if !gates[h.Type] || h.Provider == "" {
			continue
		}
		if foreign := sc.foreignReach(h.Provider, h.Filter.Types, h.Filter.Capabilities); foreign != "" {
			out = append(out, scopeFinding("hook", h.Name, h.Provider,
				fmt.Sprintf("%s entry %s reaches %s", h.Type, h.URI, foreign)))
		}
	}
	for _, c := range d.Capabilities {
		if c.Attach == nil || c.Provider == "" || !hasGateEntry(c.Hooks, gates) {
			continue
		}
		if foreign := sc.foreignReach(c.Provider, c.Attach.Types, c.Attach.Capabilities); foreign != "" {
			out = append(out, scopeFinding("capability", c.Name, c.Provider,
				fmt.Sprintf("its gate entries (%s) ride its attach to %s, which its attach does not make its own", gatePointsOf(c.Hooks, gates), foreign)))
		}
	}
	for _, p := range d.Policies {
		if p.Provider == "" || !hasGateEntry(p.Hooks, gates) || p.Applies.IsZero() {
			continue
		}
		if foreign := sc.foreignReach(p.Provider, p.Applies.Types, p.Applies.Capabilities); foreign != "" {
			out = append(out, scopeFinding("policy", p.Name, p.Provider,
				fmt.Sprintf("its gate entries (%s) apply to %s", gatePointsOf(p.Hooks, gates), foreign)))
		}
	}
	SortFindings(out)
	return out
}

// foreignReach names what a selector by types and capabilities reaches that
// provider does not own, or "" when it reaches only its own objects. Types
// decide when named; else capabilities; a selector naming neither reaches
// every object.
func (sc Scope) foreignReach(provider string, types, caps []string) string {
	if len(types) > 0 {
		var foreign []string
		for _, t := range types {
			if !sc.OwnsType(provider, t) {
				foreign = append(foreign, t)
			}
		}
		if len(foreign) == 0 {
			return ""
		}
		if len(caps) > 0 && sc.allCapsOwned(provider, caps) {
			return "" // only objects carrying one of its capabilities
		}
		return "the types " + strings.Join(foreign, ", ")
	}
	if len(caps) > 0 {
		var foreign []string
		for _, c := range caps {
			if !sc.OwnsCapability(provider, c) {
				foreign = append(foreign, c)
			}
		}
		if len(foreign) == 0 {
			return ""
		}
		return "objects carrying " + strings.Join(foreign, ", ")
	}
	return "every object"
}

func (sc Scope) allCapsOwned(provider string, caps []string) bool {
	for _, c := range caps {
		if !sc.OwnsCapability(provider, c) {
			return false
		}
	}
	return true
}

func scopeFinding(kind, name, provider, what string) DeclarationFinding {
	return DeclarationFinding{
		Rule: FindingScope, Severity: FindingWarning, Kind: kind, Name: name, Provider: provider,
		Message: fmt.Sprintf("%s, which %s does not own: it is narrowed to the objects %s owns. "+
			"To let it refuse writes there, grant it in Univers.yaml (sidecars: [{name: %s, grants: {gate: {types: [...], capabilities: [...]}}}])",
			what, provider, provider, provider),
	}
}

func hasGateEntry(h Hooks, gates map[HookPoint]bool) bool {
	for p, es := range h {
		if gates[p] && len(es) > 0 {
			return true
		}
	}
	return false
}

func gatePointsOf(h Hooks, gates map[HookPoint]bool) string {
	var ps []string
	for p, es := range h {
		if gates[p] && len(es) > 0 {
			ps = append(ps, string(p))
		}
	}
	sort.Strings(ps)
	return strings.Join(ps, ", ")
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
