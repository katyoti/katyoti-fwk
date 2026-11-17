// SPDX-License-Identifier: Apache-2.0

package api

// DECLARATIONS. What the catalog declares that CWM itself reads — edge types,
// object types, capabilities, aspects, workflows, policies, hook types,
// hooks and event types — decoded once, in one deterministic order. CWM's store and every
// service reading the same declarations (the object pane, object-context) go
// through DeclarationsFromCatalog, so they never disagree on what is
// declared. A kind an extension declares and reads (an object panel, a gate,
// an agent context) is not here: its extension reads it (mbapi.QueryTyped).

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Declarations is every declaration the catalog carries, plus the DSL of every
// data-type by URI (a capability's schema is compiled from it).
type Declarations struct {
	Edges        []EdgeTypeRegistration
	Objects      []ObjectType
	Capabilities []Capability
	Aspects      []Aspect
	Workflows    []Workflow
	Policies     []Policy
	HookTypes    []HookType
	Hooks        []Hook
	EventTypes   []EventType
	DataTypes    map[string][]byte
	// Findings are what reading them skipped or read leniently: a declaration
	// that does not decode, one in a newer minor of its kind's format (keys
	// ignored), one in a newer major (refused). Shown by DeclarationFindings —
	// a log line alone is seen by nobody.
	Findings []DeclarationFinding
	// Sources is where each declaration read comes from - its contribution's
	// URI and the binary that contributed it - by DeclarationKey. A refresh
	// replaces a declaration by it, and keeps the last good one while its
	// member is absent or its new version is refused.
	Sources map[string]DeclarationSource
	// Members are the providers whose manifest this reading had in hand.
	Members map[string]bool
	// Named are the URIs of every declaration the manifests name, read or
	// refused: a declaration goes only when its member no longer names it.
	Named map[string]bool
	// Overridden are the declarations, by DeclarationKey, whose body carries
	// a project's override entry (its member's Overrides name it from
	// `project:<token>`): such a declaration wins over the host's floor,
	// where a plain catalog declaration never does.
	Overridden map[string]bool
}

// DeclarationSource is where one declaration comes from.
type DeclarationSource struct {
	URI      string
	Provider string
}

// DeclarationKey is a declaration's key in a declaration set: its kind and
// its name (a hook's Source, which carries its provider).
func DeclarationKey(kind, name string) string { return kind + "/" + name }

// declaredKinds is every kind CWM reads from the catalog.
//
// `aspect` was MISSING from the list this replaced, from 7g until 2026-09-25:
// the decoder handled it, the filter dropped it first, so set.Aspects was
// always empty in production. One set, read by the filter and the decoder
// alike (add), is the guard.
var declaredKinds = map[string]bool{
	"edge-type": true, "object-type": true, "capability": true, "aspect": true,
	"workflow": true, "policy": true, "hook-type": true, "hook": true,
	"event-type": true,
}

// DeclarationsFromCatalog decodes every declaration of the universe-wide
// catalog. Bodies are JSON; a body-free index entry is materialised via
// Catalog.GetBody. A declaration that cannot be read is skipped and recorded
// as a finding — one malformed remote declaration never blocks the rest.
func DeclarationsFromCatalog(ctx stdctx.Context, cat mbapi.Catalog) Declarations {
	manifests, err := cat.List(ctx)
	if err != nil {
		slog.Warn("declarations: catalog List failed: nothing is declared", "error", err)
		return Declarations{DataTypes: map[string][]byte{}}
	}
	return DeclarationsFromManifests(manifests, func(uri string) ([]byte, error) { return cat.GetBody(ctx, uri) })
}

// DeclarationsFromManifests is DeclarationsFromCatalog over manifests already
// in hand (a universe lint reads them from executables). getBody materialises
// a body-free index entry; nil skips such an entry.
//
// FORMATS. Each member states the format it writes of each kind (its
// sidecar-info `formats`); this host reads its own from the built-in kind
// reader (builtin.Formats: the system set overlaid by the pack-owned kinds).
// An entry of a kind its member writes in a newer MAJOR is refused; in a newer
// MINOR, the keys this host does not know are dropped and named; otherwise the
// strict codec refuses a key no field takes (contribapi.JudgeFormat).
func DeclarationsFromManifests(manifests []*mbapi.Manifest, getBody func(uri string) ([]byte, error)) Declarations {
	var set Declarations
	// The DSL of every data-type, by URI: a capability's schema is compiled
	// from it after the walk (capabilitySchemas).
	set.DataTypes = map[string][]byte{}
	set.Sources, set.Members, set.Named, set.Overridden = map[string]DeclarationSource{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	host := builtin.Formats()
	// ONE NAME, ONE DECLARATION. The manifests are walked in provider order,
	// so the first of two providers declaring one name differently is always
	// the same; the second is refused with a finding naming both (contract 2).
	// The same body from several binaries (a pack linked into each) is one.
	ordered := append([]*mbapi.Manifest(nil), manifests...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i] != nil && (ordered[j] == nil || ordered[i].Provider < ordered[j].Provider)
	})
	bodies := map[string][]byte{}
	for _, m := range ordered {
		if m == nil {
			continue
		}
		if m.Provider != "" {
			set.Members[m.Provider] = true
		}
		formats := contribapi.MemberFormats(m.Contributions)
		// The entries a project's override entry changed in this member.
		projectOverridden := map[string]bool{}
		for _, o := range m.Overrides {
			if strings.HasPrefix(o.From, contribapi.ProjectOrigin("")) {
				projectOverridden[o.URI] = true
			}
		}
		for _, c := range m.Contributions {
			if c.Type == "data-type" {
				// The DSL is the data-type's source layer, resident in the index.
				if src := c.Source(); len(src) > 0 {
					set.DataTypes[c.ResolvedURI()] = src
				}
				continue
			}
			if !declaredKinds[c.Type] {
				continue
			}
			set.Members[c.Provider] = true
			set.Named[c.ResolvedURI()] = true
			finding := func(rule, severity, msg string) {
				set.Findings = append(set.Findings, DeclarationFinding{
					Rule: rule, Severity: severity, Kind: c.Type, Name: c.ID, URI: c.ResolvedURI(),
					Provider: c.Provider, Message: msg})
			}
			hostFormat, memberFormat := host[c.Type], formats[c.Type]
			verdict := contribapi.JudgeFormat(hostFormat, memberFormat)
			if verdict == contribapi.FormatNewerMajor {
				msg := contribapi.NewerMajorMessage(c.Provider, c.Type, memberFormat, hostFormat)
				slog.Warn("cwm: refusing a declaration in a newer format", "uri", c.ResolvedURI(), "reason", msg)
				finding(FindingFormatNewerMajor, FindingError, msg)
				continue
			}
			// QUARANTINED, NOT SERVED. The catalog judged the entry against
			// its kind's schema and marked it invalid (every kind validates
			// as an error): CWM does not read it. It stays named, so a
			// refresh keeps the last good declaration in force, and the
			// finding shows why - kind by no name, the catalog's verdict.
			if c.Valid != nil && !*c.Valid {
				msg := "quarantined: it does not fit its kind's schema"
				if len(c.ValidationErrors) > 0 {
					msg += " (" + strings.Join(c.ValidationErrors, "; ") + ")"
				}
				slog.Warn("cwm: a declaration the catalog marked invalid is not served", "uri", c.ResolvedURI(), "errors", c.ValidationErrors)
				finding(FindingInvalid, FindingError, msg)
				continue
			}
			body := c.Data()
			if len(body) == 0 {
				if getBody == nil {
					continue
				}
				b, gerr := getBody(c.ResolvedURI())
				if gerr != nil {
					slog.Warn("cwm: catalog GetBody failed for type; skipping", "uri", c.ResolvedURI(), "error", gerr)
					finding(FindingDecode, FindingError, "its body could not be read: "+gerr.Error())
					continue
				}
				body = b
			}
			if verdict == contribapi.FormatNewerMinor {
				if stripped, ignored, lerr := LenientDeclarationJSON(c.Type, body); lerr == nil && len(ignored) > 0 {
					body = stripped
					finding(FindingFormatNewerMinor, FindingWarning,
						contribapi.NewerMinorMessage(c.Provider, c.Type, memberFormat, hostFormat, ignored))
				}
			}
			// Every kind decodes through its strict codec (declaration_keys.go):
			// a key no field takes is REFUSED, naming it, and the declaration is
			// skipped with the reason recorded — as the pack loader does.
			claim := func(key string) bool {
				k := DeclarationKey(c.Type, key)
				held, ok := set.Sources[k]
				if !ok {
					set.Sources[k] = DeclarationSource{URI: c.ResolvedURI(), Provider: c.Provider}
					bodies[k] = compactJSON(body)
					if projectOverridden[c.ResolvedURI()] {
						set.Overridden[k] = true
					}
					return true
				}
				if !bytes.Equal(bodies[k], compactJSON(body)) {
					finding(FindingNameClash, FindingError, fmt.Sprintf(
						"refused: %s %q is already declared by %s (%s); two providers never declare one name differently",
						c.Type, key, held.Provider, held.URI))
				}
				return false
			}
			if derr := set.add(c.Type, c.Provider, body, claim); derr != nil {
				slog.Warn("cwm: skipping malformed "+c.Type, "uri", c.ResolvedURI(), "error", derr)
				finding(FindingDecode, FindingError, "skipped: "+derr.Error())
			}
		}
	}
	// DETERMINISTIC ORDER. Catalog iteration is map-backed, so without this the
	// winner of a cross-provider name collision would change between runs —
	// and today exactly one name collides (`module`, declared by ktw-code as a
	// node type and by ktw-methods as an object type). A type whose meaning
	// depends on iteration order is worse than either meaning.
	sort.Slice(set.Objects, func(i, j int) bool { return set.Objects[i].Name < set.Objects[j].Name })
	sort.Slice(set.Capabilities, func(i, j int) bool { return set.Capabilities[i].Name < set.Capabilities[j].Name })
	sort.Slice(set.Aspects, func(i, j int) bool { return set.Aspects[i].Name < set.Aspects[j].Name })
	sort.Slice(set.Workflows, func(i, j int) bool { return set.Workflows[i].Name < set.Workflows[j].Name })
	sort.Slice(set.HookTypes, func(i, j int) bool { return set.HookTypes[i].Name < set.HookTypes[j].Name })
	sort.Slice(set.Hooks, func(i, j int) bool { return set.Hooks[i].Source() < set.Hooks[j].Source() })
	sort.Slice(set.EventTypes, func(i, j int) bool { return set.EventTypes[i].Name < set.EventTypes[j].Name })
	SortFindings(set.Findings)
	return set
}

// compactJSON is body without insignificant spaces, to compare two bodies.
func compactJSON(body []byte) []byte {
	var b bytes.Buffer
	if json.Compact(&b, body) != nil {
		return body
	}
	return b.Bytes()
}

// add decodes one declaration of kind into the set, stamping its provider.
// claim is asked with the declaration's name before it joins the set: false
// leaves it out (its name is already declared).
func (set *Declarations) add(kind, provider string, body []byte, claim func(name string) bool) error {
	var derr error
	switch kind {
	case "edge-type":
		var et EdgeTypeRegistration
		if derr = json.Unmarshal(body, &et); derr != nil {
			break
		}
		// A cardinality outside 1:1 | 1:N | N:1 | M:N is REFUSED: a
		// rule CWM cannot read must not register as no rule at all.
		if derr = et.Validate(); derr != nil {
			break
		}
		if claim(string(et.Name)) {
			set.Edges = append(set.Edges, et)
		}
	case "capability":
		var cp Capability
		// A nameless capability is DROPPED: the name is the storage key,
		// so one without it can never be matched by a type's list.
		if derr = json.Unmarshal(body, &cp); derr == nil && cp.Name != "" {
			cp.Provider = provider
			if claim(cp.Name) {
				set.Capabilities = append(set.Capabilities, cp)
			}
		}
	case "aspect":
		var tr Aspect
		if derr = json.Unmarshal(body, &tr); derr == nil && tr.Name != "" {
			tr.Provider = provider
			if claim(tr.Name) {
				set.Aspects = append(set.Aspects, tr)
			}
		}
	case "hook-type":
		var ht HookType
		if derr = json.Unmarshal(body, &ht); derr == nil && ht.Name != "" {
			if claim(string(ht.Name)) {
				set.HookTypes = append(set.HookTypes, ht)
			}
		}
	case "event-type":
		var et EventType
		if derr = json.Unmarshal(body, &et); derr == nil && et.Name != "" {
			et.Provider = provider
			if claim(et.Name) {
				set.EventTypes = append(set.EventTypes, et)
			}
		}
	case "hook":
		var h Hook
		if derr = json.Unmarshal(body, &h); derr == nil && h.Name != "" {
			h.Provider = provider
			if claim(h.Source()) {
				set.Hooks = append(set.Hooks, h)
			}
		}
	case "policy":
		var pol Policy
		if derr = json.Unmarshal(body, &pol); derr == nil && pol.Name != "" {
			pol.Provider = provider
			if claim(pol.Name) {
				set.Policies = append(set.Policies, pol)
			}
		}
	case "workflow":
		var w Workflow
		if derr = json.Unmarshal(body, &w); derr == nil && w.Name != "" {
			if claim(w.Name) {
				set.Workflows = append(set.Workflows, w)
			}
		}
	case "object-type":
		var ot ObjectType
		// A declaration with no name is DROPPED, not registered: an
		// unnamed type can never match an object's `type:` field, so
		// keeping it would inflate the declared set — and the declared
		// set being non-empty is exactly what switches validation ON.
		if derr = json.Unmarshal(body, &ot); derr == nil && ot.Name != "" {
			ot.Provider = provider
			if claim(ot.Name) {
				set.Objects = append(set.Objects, ot)
			}
		}
	}
	return derr
}
