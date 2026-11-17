// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// kinds.go — the kind registry: every kind declaration in a view, resolved once.
//
// Three places used to decode `contribution-type` entries independently — the
// query surface, the aggregator's reindex and the census — each reading only the
// schema, each a copy the next field would have to be added to. One registry,
// built from the same contributions all three already hold, answers every
// question a consumer has about a kind: schema, severity, default layer,
// residency, execution class, metadata budget.
//
// It is a value, not a service: whoever holds the contributions builds it, and
// it is as fresh as they are. The aggregator rebuilds it per reindex; the census
// per manifest; a query per call.

// Kinds is the resolved set of kind declarations in one view.
type Kinds struct {
	byName map[string]KindDeclaration
	// declaredBy is the URI of the contribution-type entry each kind came
	// from — the provider that owns the kind, for findings that must name it.
	declaredBy map[string]string
	// shadowed holds, per kind, every declaration that LOST to the one in
	// byName: a kind is declared once, and a second declaration is a
	// finding (Check), never a silent fallback. Kept in the order met.
	shadowed map[string][]kindSource
	// canon is the canonical body of the winning declaration, for the
	// identical-or-differing question a duplicate raises — provider-neutral
	// (canon) and as written (raw).
	canon map[string]string
	raw   map[string]string
	// schema is the schema the winning declaration names, as written.
	schema map[string]string
}

// kindSource is one declaration of a kind: where it came from and its
// declaration hash input — the canonical body (json.Marshal of the decoded
// declaration — struct field order, sorted map keys — so two producers
// serialising the same declaration differently still compare equal), with a
// schema under the declarer's own provider made provider-neutral
// (declarationBody).
type kindSource struct {
	URI    string
	Body   string
	Raw    string // the canonical body as written, schema included
	Schema string // the schema this declaration names, as written
}

// KindsFrom decodes every `contribution-type` entry with a resident data layer.
//
// Entries whose data layer is not resident, or does not decode, are skipped
// silently here: this is a lookup, and the census (Check, at authoring) and the
// aggregator (Check, at ingest) are where a broken declaration is reported.
//
// # Precedence is by origin, never by order
//
// A kind declared more than once resolves to ONE declaration, chosen by where
// it comes from: a `system:` declaration wins over any member's, whatever the
// slice order; between two members the first in slice order wins — at the hub
// that is member order, which is deterministic and reported. The losers are
// kept and every one of them is a finding of Check. The first rule closes
// blind audit H4: the hub lists the system carrier LAST, so first-wins let any
// member declaring `workflow` shadow `system:/contribution-type/workflow`
// universe-wide, and reordering Univers.yaml changed a kind's schema.
//
// The same URI met twice is the same entry (a view holding one manifest's
// entries beside another's copy of them) and is not a second declaration.
func KindsFrom(cs []Contribution) Kinds {
	k := Kinds{byName: map[string]KindDeclaration{}, declaredBy: map[string]string{},
		shadowed: map[string][]kindSource{}, canon: map[string]string{}, raw: map[string]string{}, schema: map[string]string{}}
	seen := map[string]bool{}
	for _, c := range cs {
		if c.Type != ResourceTypeContributionType {
			continue
		}
		body := c.Data()
		if len(body) == 0 {
			continue
		}
		var decl KindDeclaration
		if err := json.Unmarshal(body, &decl); err != nil {
			continue
		}
		name := decl.Name
		if name == "" {
			name = c.ID
			decl.Name = name
		}
		uri := c.ResolvedURI()
		if seen[uri] {
			continue
		}
		seen[uri] = true
		rawBody, _ := json.Marshal(decl)
		src := kindSource{URI: uri, Body: declarationBody(decl, uri), Raw: string(rawBody), Schema: decl.Schema}
		if cur, dup := k.declaredBy[name]; dup {
			if c.Provider == SystemProvider && !isSystemURI(cur) {
				// The system declaration wins whatever its position: the
				// member's goes to the shadowed list, where Check names it.
				k.shadowed[name] = append(k.shadowed[name], kindSource{URI: cur, Body: k.canon[name], Raw: k.raw[name], Schema: k.schema[name]})
				k.byName[name], k.declaredBy[name], k.canon[name], k.raw[name], k.schema[name] = decl, uri, src.Body, src.Raw, src.Schema
				continue
			}
			k.shadowed[name] = append(k.shadowed[name], src)
			continue
		}
		k.byName[name] = decl
		k.declaredBy[name] = uri
		k.canon[name] = src.Body
		k.raw[name] = src.Raw
		k.schema[name] = src.Schema
	}
	return k
}

// DeclaredSchema is one declarer of a kind and the schema it names.
type DeclaredSchema struct {
	Declarer string
	Schema   string
}

// AgreeingSchemas returns, for a kind declared by several URIs with the same
// declaration body, each declarer and the schema it names — the resolved one
// first. Each binary carrying a pack names the schema it ships itself, so the
// declarations agreeing says nothing yet of the schemas: whoever sees the
// data-types (the hub catalog, the census) compares their digests. Nil when
// one URI declares the kind; a declaration that differs is left out (Check
// reports it).
func (k Kinds) AgreeingSchemas(kind string) []DeclaredSchema {
	uri, ok := k.declaredBy[kind]
	if !ok || len(k.shadowed[kind]) == 0 {
		return nil
	}
	out := []DeclaredSchema{{Declarer: uri, Schema: k.schema[kind]}}
	for _, s := range k.shadowed[kind] {
		if k.agrees(kind, s) && s.Schema != "" {
			out = append(out, DeclaredSchema{Declarer: s.URI, Schema: s.Schema})
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

// ownProviderToken stands for the declarer's own provider in a declaration's
// hash input: `<binary>:/data-type/skill` declared by `<binary>` reads the
// same from every binary that carries the pack.
const ownProviderToken = "<declarer>"

// declarationBody is the canonical body two declarations of one kind are
// compared on. A pack that declares its kind is carried by several binaries;
// each publishes the declaration under its own provider, naming the schema it
// ships under that same provider. Those copies differ only by that provider,
// so a schema under the declarer's own provider is compared provider-neutral:
// same name and same body is one kind, whichever binaries carry it. Whether
// the schemas themselves agree is the catalog's question, where the
// data-types are visible.
func declarationBody(decl KindDeclaration, declarer string) string {
	if du, err := ParseResourceURI(declarer); err == nil && decl.Schema != "" {
		if su, err := ParseResourceURI(decl.Schema); err == nil && su.Provider == du.Provider &&
			strings.HasPrefix(decl.Schema, su.Provider+":") {
			decl.Schema = ownProviderToken + strings.TrimPrefix(decl.Schema, su.Provider)
		}
	}
	canon, _ := json.Marshal(decl)
	return string(canon)
}

// isSystemURI reports whether a resource URI is the system provider's.
func isSystemURI(uri string) bool {
	u, err := ParseResourceURI(uri)
	return err == nil && u.Provider == SystemProvider
}

// Declarers returns every URI that declares `kind`, the resolved one first,
// then the shadowed ones in the order met; nil when the kind is undeclared.
func (k Kinds) Declarers(kind string) []string {
	uri, ok := k.declaredBy[kind]
	if !ok {
		return nil
	}
	out := []string{uri}
	for _, s := range k.shadowed[kind] {
		out = append(out, s.URI)
	}
	return out
}

// Get returns the declaration of a kind, and whether it is declared.
func (k Kinds) Get(kind string) (KindDeclaration, bool) {
	d, ok := k.byName[kind]
	return d, ok
}

// DeclaredBy returns the URI of the contribution-type entry that declares
// `kind`, or "" when it is undeclared.
func (k Kinds) DeclaredBy(kind string) string { return k.declaredBy[kind] }

// Names lists every declared kind, sorted.
func (k Kinds) Names() []string {
	out := make([]string, 0, len(k.byName))
	for n := range k.byName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Len is the number of declared kinds.
func (k Kinds) Len() int { return len(k.byName) }

// SchemaFor resolves the data-type governing `kind`.
//
// The meta-model is the base case: `contribution-type` and `data-type` cannot
// name their own schema without circularity, so when they are undeclared in
// this view they resolve to the system data-type of the same name — which is
// the point where the chain terminates in a self-reference rather than in a
// special case per consumer.
func (k Kinds) SchemaFor(kind string) string {
	if d, ok := k.byName[kind]; ok && d.Schema != "" {
		return d.Schema
	}
	if IsMetaKind(kind) {
		return Contribution{Provider: SystemProvider, Type: ResourceTypeDataType, ID: kind}.ResolvedURI()
	}
	return ""
}

// Schemas returns kind → schema URI for every declared kind that names one.
// The shape consumers that only want the schema column ask for.
func (k Kinds) Schemas() map[string]string {
	out := make(map[string]string, len(k.byName))
	for name, d := range k.byName {
		if d.Schema != "" {
			out[name] = d.Schema
		}
	}
	return out
}

// SeverityFor resolves the severity a failing entry of `kind` is reported at.
//
// The KIND declares it (contribution-types.md §2); central config may OVERRIDE
// it, per kind or globally, and never invents a default of its own. Precedence:
// override.PerType[kind] > override.Validate > the kind's `validate:` > warn.
// The answer is normalised to off / warn / on.
func (k Kinds) SeverityFor(kind string, override ContributionsConfig) string {
	if s, ok := override.PerType[kind]; ok && s != "" {
		return normalizeSeverity(s)
	}
	if override.Validate != "" {
		return normalizeSeverity(override.Validate)
	}
	if d, ok := k.byName[kind]; ok {
		return d.Severity()
	}
	return normalizeSeverity(defaultGlobalSeverity)
}

// DefaultLayer returns the layer `Open(uri, "")` resolves to for `kind`.
//
// An undeclared kind answers by the same convention a half-written
// declaration does — source, else data — so a consumer holding an entry of a
// kind this view does not know can still open its content. Whether the entry
// HAS that layer is the opener's question, answered from the envelope.
func (k Kinds) DefaultLayer(kind string) string {
	if d, ok := k.byName[kind]; ok {
		if l := d.DefaultLayer(); l != "" {
			return l
		}
	}
	return LayerSource
}

// Resident reports whether `kind` wants `layer` carried in the index.
// Undeclared kinds and undeclared layers are not resident: nothing is carried
// by accident, and the band remains a ceiling this cannot raise.
func (k Kinds) Resident(kind, layer string) bool {
	d, ok := k.byName[kind]
	return ok && d.IsResident(layer)
}

// Execution returns how `kind`'s content is consumed. An undeclared kind is
// declarative: the safe assumption about an unknown kind is that nothing runs it.
func (k Kinds) Execution(kind string) Execution {
	if d, ok := k.byName[kind]; ok {
		return d.ExecutionClass()
	}
	return ExecutionDeclarative
}

// Budget returns the metadata budget of `kind` in bytes, or 0 when undeclared.
func (k Kinds) Budget(kind string) int {
	if d, ok := k.byName[kind]; ok {
		return d.Budget()
	}
	return 0
}

// MetaFrom returns the layer a kind's meta is derived from and the fields
// derived — ("", nil) when the producer writes meta itself.
func (k Kinds) MetaFrom(kind string) (layer string, fields []string) {
	if d, ok := k.byName[kind]; ok {
		return d.MetaFrom()
	}
	return "", nil
}

// Translatable returns the texts a kind lets a person read in their
// language (KindDeclaration.Translatable, parsed) — nil for a kind that
// declares none, or is undeclared.
func (k Kinds) Translatable(kind string) []TranslatablePath {
	if d, ok := k.byName[kind]; ok {
		return d.TranslatablePaths()
	}
	return nil
}

// Expressions returns the fields of a kind's data layer that hold an el
// predicate over one object (KindDeclaration.Expressions) - nil for a kind
// that declares none, or is undeclared.
func (k Kinds) Expressions(kind string) []string {
	if d, ok := k.byName[kind]; ok {
		return d.Expressions
	}
	return nil
}

// KindFinding is one thing wrong with a kind declaration.
type KindFinding struct {
	// Kind is the discriminator the finding is about.
	Kind string
	// URI is the contribution-type entry that declares it ("" if the finding
	// is that nothing does). For a duplicate, the declaration that LOST.
	URI string
	// Message says what is wrong, in one line.
	Message string
	// Rule is the stable code of the check that produced the finding — one
	// of the KindRule* constants — so a consumer that treats one class
	// apart (the universe lint reports duplicates; the census refuses all)
	// switches on it rather than on the message.
	Rule string
	// Severity is ValidateWarn or ValidateError. Every finding is an error
	// except one provider declaring a kind twice with identical bodies, a
	// warning: nothing resolves differently, one copy is simply redundant
	// (two providers declaring it alike are one kind, no finding).
	Severity string
}

// The rules a KindFinding can carry.
const (
	// KindRuleIncomplete: the declaration lacks part of its contract.
	KindRuleIncomplete = "kind-incomplete"
	// KindRuleUndeclared: entries of the kind exist, no declaration does.
	KindRuleUndeclared = "kind-undeclared"
	// KindRuleDuplicate: two URIs declare the kind with different bodies,
	// or one provider declares it twice; the message names both.
	KindRuleDuplicate = "kind-duplicate"
	// KindRuleRedeclaresSystem: a member declares a kind the system set
	// declares. Always an error: a member may not redeclare a system kind.
	KindRuleRedeclaresSystem = "kind-redeclares-system"
	// KindRuleWarnUnexplained: a kind at `validate: warn` states no
	// `warn_reason`, or states one while not at warn. Every kind validates
	// its entries as an error; a warn kept is a recorded choice.
	KindRuleWarnUnexplained = "kind-warn-unexplained"
)

// warnReasonFinding is the message of a KindRuleWarnUnexplained finding about
// d, or "" when its severity and its reason agree.
func warnReasonFinding(d KindDeclaration) string {
	switch {
	case d.Validate == ValidateWarn && strings.TrimSpace(d.WarnReason) == "":
		return "validate is warn with no warn_reason: validate as error, or state why the kind cannot"
	case d.Validate != ValidateWarn && d.WarnReason != "":
		return fmt.Sprintf("warn_reason is set but validate is %q: drop the reason, or keep the kind at warn", d.Validate)
	}
	return ""
}

func (f KindFinding) String() string {
	if f.URI != "" {
		return fmt.Sprintf("kind %q (%s): %s", f.Kind, f.URI, f.Message)
	}
	return fmt.Sprintf("kind %q: %s", f.Kind, f.Message)
}

// IsWarning reports whether the finding informs rather than refuses.
func (f KindFinding) IsWarning() bool { return f.Severity == ValidateWarn }

// Check reports every incomplete declaration in this view.
//
// A kind is declared once, completely (goals G2): a schema, layers with exactly
// one default, a severity, a budget. Each omission is a finding about the KIND
// — the entries are blameless — reported here so the census refuses it at
// authoring and the aggregator logs it at ingest, from one function.
//
// `used` may name kinds that entries in the view carry; each one with no
// declaration is reported too, because nothing may publish a contribution of an
// undeclared kind.
//
// A kind declared by several providers with the same declaration body (a
// schema under each declarer's own provider compared provider-neutral) is one
// kind, and no finding: the pack that owns it is carried by several binaries.
// Each other declaration is reported once (KindRuleDuplicate, or
// KindRuleRedeclaresSystem when the kind is the system set's), naming both
// URIs: one provider declaring it twice alike is a warning, differing bodies
// an error, and a member redeclaring a system kind an error whatever the
// bodies say.
func (k Kinds) Check(used ...string) []KindFinding {
	var out []KindFinding
	for _, name := range k.Names() {
		d := k.byName[name]
		uri := k.declaredBy[name]
		add := func(msg string) {
			out = append(out, KindFinding{Kind: name, URI: uri, Message: msg, Rule: KindRuleIncomplete, Severity: ValidateError})
		}
		out = append(out, k.duplicateFindings(name)...)
		if d.Schema == "" && !IsMetaKind(name) {
			add("declares no schema")
		}
		if d.Validate == "" {
			add("declares no validate severity")
		} else if s := normalizeSeverity(d.Validate); s == ValidateWarn && d.Validate != ValidateWarn {
			add(fmt.Sprintf("validate %q is not off, warn or error", d.Validate))
		}
		if msg := warnReasonFinding(d); msg != "" {
			out = append(out, KindFinding{Kind: name, URI: uri, Message: msg, Rule: KindRuleWarnUnexplained, Severity: ValidateError})
		}
		if d.Budget() <= 0 {
			add("declares no meta budget")
		}
		if len(d.Layers) == 0 {
			add("declares no layers")
		} else {
			defaults := 0
			for _, l := range d.Layers {
				if l.Default {
					defaults++
				}
			}
			if defaults != 1 {
				add(fmt.Sprintf("%d default layers, want exactly 1", defaults))
			}
		}
		if len(d.Translatable) > 0 {
			_, errs := ParseTranslatablePaths(d.Translatable)
			for _, err := range errs {
				add(err.Error())
			}
			if _, declared := d.Layers[LayerTranslations]; !declared {
				add("declares translatable texts but no `translations` layer to carry them")
			}
		}
		if from, fields := d.MetaFrom(); from != "" {
			if _, declared := d.Layers[from]; !declared {
				add(fmt.Sprintf("meta.from names layer %q, which the kind does not declare", from))
			}
			if len(fields) == 0 {
				add("meta.from is set but meta.fields names nothing to derive")
			}
			for _, f := range fields {
				if _, known := DerivableMetaFields[f]; !known {
					add(fmt.Sprintf("meta.fields names %q, which is not a derivable meta field", f))
				}
			}
		}
	}
	out = append(out, k.sourceFindings()...)
	seen := map[string]bool{}
	for _, u := range used {
		if seen[u] {
			continue
		}
		seen[u] = true
		if _, ok := k.byName[u]; !ok {
			out = append(out, KindFinding{Kind: u, Message: "entries published, kind undeclared", Rule: KindRuleUndeclared, Severity: ValidateError})
		}
	}
	return out
}

// agrees reports whether a shadowed declaration is the resolved one: the same
// body as written, or the same once each names its own schema
// provider-neutral.
func (k Kinds) agrees(kind string, s kindSource) bool {
	return s.Raw == k.raw[kind] || s.Body == k.canon[kind]
}

// sameProvider reports whether two resource URIs are one provider's.
func sameProvider(a, b string) bool {
	ua, errA := ParseResourceURI(a)
	ub, errB := ParseResourceURI(b)
	return errA == nil && errB == nil && ua.Provider == ub.Provider
}

// duplicateFindings reports every declaration of `name` that lost to the
// resolved one and does not agree with it.
func (k Kinds) duplicateFindings(name string) []KindFinding {
	losers := k.shadowed[name]
	if len(losers) == 0 {
		return nil
	}
	winner := k.declaredBy[name]
	systemWins := isSystemURI(winner)
	out := make([]KindFinding, 0, len(losers))
	for _, l := range losers {
		f := KindFinding{Kind: name, URI: l.URI}
		same := k.agrees(name, l)
		switch {
		case systemWins:
			f.Rule, f.Severity = KindRuleRedeclaresSystem, ValidateError
			f.Message = fmt.Sprintf("a member may not redeclare a system kind: %s is declared by %s; drop this declaration", name, winner)
		case same && !sameProvider(winner, l.URI):
			continue // the same declaration, carried by another binary: one kind
		case same:
			f.Rule, f.Severity = KindRuleDuplicate, ValidateWarn
			f.Message = fmt.Sprintf("declared twice with identical bodies: %s (resolved) and %s (shadowed); declare it once", winner, l.URI)
		default:
			f.Rule, f.Severity = KindRuleDuplicate, ValidateError
			f.Message = fmt.Sprintf("declared twice with different bodies: %s (resolved, first in member order) and %s (shadowed); one kind, one declaration", winner, l.URI)
		}
		out = append(out, f)
	}
	return out
}
