// SPDX-License-Identifier: Apache-2.0

package census

import (
	"context"
	"encoding/json"
	"fmt"
	contentlint "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/lint"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	schemalocal "github.com/katyoti/katyoti-fwk/internal/extensions/schema/local"
)

// AssertValid validates every contribution a binary publishes against the schema
// its kind declares, using the binary's OWN shipped data-types.
//
// # Why this lives in a test rather than in the binary
//
// Validating at the source is the only way an authoring error surfaces where it
// was authored — but doing it INSIDE a shipped sidecar would mean linking the
// DSL parser (~56 packages) into every binary, which is exactly the dependency
// the schema boundary exists to keep out, and would undo the extraction that
// took sidecars from 40 MB to 15 MB.
//
// A test has no such constraint: it may link anything. So the author still gets
// the error at build time, and the shipped binary stays light. The aggregator
// validates again at ingest, where it has the whole graph and can check
// cross-contribution invariants a single binary cannot see.
func AssertValid(t *testing.T, name string, m *mbapi.Manifest) {
	t.Helper()
	AssertValidWith(t, name, m, SchemaService(t))
}

// SchemaService builds the throwaway validator AssertValid uses.
//
// Exported because a caller checking SEVERAL manifests must build it ONCE: the
// schema extension registers into package-level extension points, so a second
// application in the same process fails on duplicate keys. One service, many
// manifests.
func SchemaService(t *testing.T) schemaapi.SchemaService { return schemaService(t) }

// AssertValidWith is AssertValid against an already-built validator.
//
// The same checks, so a manifest read back from a COMPILED BINARY is judged by
// exactly the rules its in-process build was judged by. Two implementations of
// "is this manifest sound" would be two things to keep in step, and the whole
// point of checking the artifact is to compare it against the same standard.
func AssertValidWith(t *testing.T, name string, m *mbapi.Manifest, svc schemaapi.SchemaService) {
	t.Helper()
	AssertValidWithin(t, name, m, svc, nil)
}

// AssertValidWithin is AssertValidWith over a DECLARED universe.
//
// `declared` is what the census knows the OTHER members declare — the kind
// declarations and schemas of fixture packs it can name (FixtureDeclarations).
// A binary may publish entries of a kind it declares nowhere (a foreign kind,
// Manifest.ForeignKinds): a kind is declared once in a universe, and the
// member declaring it may be another one. At runtime the aggregator resolves
// that; here the census stays strict and resolves it against `declared` —
// every foreign kind must be declared there, its entries are then judged by
// that declaration (budget, required layers, schema), and a foreign kind the
// census cannot resolve fails with the kind and the entry URIs. With no
// `declared`, a binary may have no foreign kind at all.
func AssertValidWithin(t *testing.T, name string, m *mbapi.Manifest, svc schemaapi.SchemaService, declared []contribapi.Contribution) {
	t.Helper()

	assertNothingWasDropped(t, name, m)

	// The binary's own entries and the declarations it may resolve against,
	// in that order: the binary's own declaration of a kind wins over a
	// fixture's (KindsFrom keeps the first non-system one), and both
	// declaring the same kind is a duplicate finding like any other.
	view := append(append([]contribapi.Contribution(nil), m.Contributions...), declared...)
	registered := registerDataTypes(t, svc, view)
	// A schema is the KIND's statement, so the check starts by reading what
	// this binary's kinds declare. A binary sees only its own kinds, which is
	// exactly why the aggregator checks again with the whole graph — and why
	// the census, which can name that graph, reads the declared universe.
	assertForeignKindsResolve(t, name, m, declared)
	kinds := contribapi.KindsFrom(view)

	assertImmutableProvidersCarryADigest(t, name, m)
	assertSystemProviderCarried(t, name, m)
	assertEveryKindIsComplete(t, name, kinds, m.Contributions)
	assertMetaWithinBudget(t, name, kinds, m.Contributions)
	assertIndexWithinBands(t, name, kinds, m)
	assertMetaDerivedFromData(t, name, kinds, m.Contributions)
	assertEveryKindIsDescribed(t, name, kinds, view)
	assertRequiredLayersCarried(t, name, kinds, m.Contributions)
	assertLayerMediaTypesMatchDeclaration(t, name, kinds, m.Contributions)
	assertDeclaredSchemasShip(t, name, kinds, registered)
	assertEveryDataTypeResolves(t, name, svc, m.Contributions)
	assertSchemaReferencesShip(t, name, m.Contributions, registered)
	assertEveryBodyIsValidated(t, name, kinds, m.Contributions, registered)

	var failures []string
	for _, c := range m.Contributions {
		schema := kinds.SchemaFor(c.Type)
		if schema == "" || len(c.Data()) == 0 {
			continue
		}
		if !registered[schema] {
			continue // reported by assertEveryBodyIsValidated
		}
		out, err := svc.ValidateAgainst(context.Background(), schemaapi.ValidateAgainstInput{
			URI:  schema,
			Body: c.Data(),
		})
		if err != nil {
			failures = append(failures, fmt.Sprintf("  %s: %v", c.ResolvedURI(), err))
			continue
		}
		if out.Valid {
			continue
		}
		var diags []string
		for _, d := range out.Diagnostics {
			diags = append(diags, d.Message)
		}
		failures = append(failures, fmt.Sprintf("  %s (against %s): %s",
			c.ResolvedURI(), schema, strings.Join(diags, "; ")))
	}

	if len(failures) > 0 {
		sort.Strings(failures)
		t.Errorf("validate[%s]: %d contribution(s) do not match the schema their kind declares:\n%s",
			name, len(failures), strings.Join(failures, "\n"))
	}
}

// assertEveryBodyIsValidated refuses an entry whose body the census could not
// judge: its kind declares a schema that neither this binary nor the declared
// universe it was given registers.
//
// Every kind validates as an error, so the census is an error on every kind:
// no entry with a body passes unchecked. A schema another member ships is
// named through the declared universe (DeclarationsOf that member's folder),
// as a foreign kind is - the census never assumes the aggregator will check
// what it could not. An entry with no data layer has nothing to validate, at
// ingest as here.
func assertEveryBodyIsValidated(t *testing.T, name string, kinds contribapi.Kinds, cs []contribapi.Contribution, registered map[string]bool) {
	t.Helper()
	bad := unvalidatedBodies(kinds, cs, registered)
	if len(bad) == 0 {
		return
	}
	t.Errorf("census[%s]: %d contribution(s) whose kind's schema the census cannot resolve, so their body goes unchecked:\n%s\n\n"+
		"Every kind validates as an error, at build as at ingest. Name the member that ships the schema in the census\n"+
		"(census.DeclarationsOf its folder), or ship the data-type with the kind.",
		name, len(bad), strings.Join(bad, "\n"))
}

// unvalidatedBodies is assertEveryBodyIsValidated's finding list, sorted: one
// line per entry carrying a body whose kind's declared schema is not
// registered. Pure, so the guard can be falsified without a binary.
func unvalidatedBodies(kinds contribapi.Kinds, cs []contribapi.Contribution, registered map[string]bool) []string {
	var bad []string
	for _, c := range cs {
		schema := kinds.SchemaFor(c.Type)
		if schema == "" || len(c.Data()) == 0 || registered[schema] {
			continue // no schema: reported by assertEveryKindIsComplete
		}
		bad = append(bad, fmt.Sprintf("  %s (%s): schema %s is not registered", c.ResolvedURI(), c.Type, schema))
	}
	sort.Strings(bad)
	return bad
}

// schemaService builds a throwaway application wired with only the schema
// extension, so the test gets a real validator through public API — the parser
// it needs is a test-time dependency, never one the sidecar ships.
func schemaService(t *testing.T) schemaapi.SchemaService {
	t.Helper()
	app := application.New(application.Config(), schemalocal.Extension())
	// The schema extension hangs a CLI verb off the root command during
	// Bootstrap, so a root must exist even though this application never runs a
	// CLI. Registering a throwaway one is cheaper than special-casing the
	// extension for tests.
	application.PutService[appcore.CobraCommand](app, "root",
		appcore.CobraCommand{Cmd: &cobra.Command{Use: "validate"}})
	if err := app.Run(); err != nil {
		t.Fatalf("validate: build schema service: %v", err)
	}
	svc, err := application.UseService[schemaapi.SchemaService](
		application.WithApplication(context.Background(), app))
	if err != nil {
		t.Fatalf("validate: resolve schema service: %v", err)
	}
	return svc
}

// registerDataTypes teaches the schema service every data-type a binary ships,
// and returns which URIs registered.
//
// Through the SHARED function, not a local copy. It used to be a third
// implementation of the same loop — and when the DSL moved to the source layer
// (c24) that copy kept reading the body, which meant it would have registered
// NOTHING and every validation below would have quietly skipped. A guard that
// silently checks nothing is worse than no guard.
func registerDataTypes(t *testing.T, svc schemaapi.SchemaService, cs []contribapi.Contribution) map[string]bool {
	t.Helper()
	before := map[string]bool{}
	for _, c := range cs {
		if c.Type == contribapi.ResourceTypeDataType {
			before[c.ResolvedURI()] = true
		}
	}
	_, failed := schemalocal.RegisterDataTypes(context.Background(), svc, cs, nil)
	for _, f := range failed {
		delete(before, f.URI)
	}
	// A data-type with no content registers nothing and is not a failure.
	for _, c := range cs {
		if c.Type == contribapi.ResourceTypeDataType && len(c.Source()) == 0 {
			delete(before, c.ResolvedURI())
		}
	}
	return before
}

// assertEveryDataTypeResolves checks that every data-type URI a binary
// publishes can actually be fetched back.
//
// The assertion that was missing. A published data-type whose content the
// registry cannot parse registers nothing, and every consumer that asks for it
// gets a 404 it cannot explain — while every check around it stays green,
// because a contribution with no registered schema is SKIPPED, which reads
// exactly like "nothing to check".
//
// It hid for two milestones that way: 16 config shapes carried JSON Schema
// while the registry parses the DSL, so the parse could never succeed. Nothing
// noticed, because nobody asked the one question this asks — does the URI you
// published answer?
func assertEveryDataTypeResolves(t *testing.T, name string, svc schemaapi.SchemaService, cs []contribapi.Contribution) {
	t.Helper()
	var unresolved []string
	for _, c := range cs {
		if c.Type != contribapi.ResourceTypeDataType {
			continue
		}
		// A data-type carrying no content declares no schema — there is
		// nothing to resolve, and that is not a failure.
		if len(c.Source()) == 0 {
			continue
		}
		uri := c.ResolvedURI()
		if _, err := svc.GetDataType(context.Background(), schemaapi.GetDataTypeInput{URI: uri}); err != nil {
			unresolved = append(unresolved, fmt.Sprintf("  %s: %v", uri, err))
		}
	}
	if len(unresolved) == 0 {
		return
	}
	sort.Strings(unresolved)
	t.Errorf("census[%s]: %d published data-type URI(s) do not resolve through GetDataType.\n%s\n\n"+
		"A published schema that does not register is a URI that 404s in a consumer which cannot say why.\n"+
		"Either its content is not the DSL the registry parses, or it should not be published as a data-type.",
		name, len(unresolved), strings.Join(unresolved, "\n"))
}

// assertSchemaReferencesShip refuses an entry whose body names, under a
// `schema` key anywhere in it, a data-type of THIS binary's provider (or of
// `system`) that is not registered — whatever the entry's kind.
//
// The hole this closes: a panel's page names the table it draws by data-type
// URI, resolved only when the tab opens (object-pane ResolvePanel). When the
// `deliverable` data-type was retired, the Composition tab kept naming
// ktw-methods:/data-type/deliverable and every module showed "schema:
// data-type not found" — the census green, because nothing read the
// references a body makes, only the schema its kind declares.
func assertSchemaReferencesShip(t *testing.T, name string, cs []contribapi.Contribution, registered map[string]bool) {
	t.Helper()
	bad := unshippedSchemaReferences(name, cs, registered)
	if len(bad) == 0 {
		return
	}
	t.Errorf("census[%s]: %d reference(s) to a data-type of this binary that is not published:\n%s\n\n"+
		"A body naming a schema under its own provider needs that data-type shipped: it fails where it is drawn, with no URI to explain it.",
		name, len(bad), strings.Join(bad, "\n"))
}

// unshippedSchemaReferences is assertSchemaReferencesShip's finding list,
// sorted: one line per entry and unregistered same-provider schema it names.
func unshippedSchemaReferences(name string, cs []contribapi.Contribution, registered map[string]bool) []string {
	var bad []string
	for _, c := range cs {
		if len(c.Data()) == 0 {
			continue
		}
		var body any
		if err := json.Unmarshal(c.Data(), &body); err != nil {
			continue // not JSON: reported by the kind's own validation
		}
		seen := map[string]bool{}
		var walk func(any)
		walk = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				for k, child := range x {
					if s, ok := child.(string); ok && k == "schema" && !seen[s] {
						seen[s] = true
						u, err := contribapi.ParseResourceURI(s)
						if err == nil && u.Type == contribapi.ResourceTypeDataType &&
							(u.Provider == name || u.Provider == contribapi.SystemProvider) && !registered[s] {
							bad = append(bad, fmt.Sprintf("  %s: names schema %s, which this binary does not ship", c.ResolvedURI(), s))
						}
					}
					walk(child)
				}
			case []any:
				for _, child := range x {
					walk(child)
				}
			}
		}
		walk(body)
	}
	sort.Strings(bad)
	return bad
}

// assertNothingWasDropped fails the build when a provider met a file it could
// not publish.
//
// # The failure this closes
//
// Validation can only judge contributions that EXIST. A file the provider
// rejected produces none, so every check downstream sees a binary that is
// simply short — and passes. An author who misspells a frontmatter key does not
// get "unknown field": the task disappears, and the only trace is a line in a
// census golden saying something was removed, which requires the golden to have
// been blessed while the file still worked. A brand-new task with a typo shows
// up as nothing at all.
//
// The providers already record this (c23) and the manifest already carries it;
// packaging already refuses on it. Only the per-binary gate ignored it, which
// is the one moment the author is standing there.
//
// Skipping is still right at RUNTIME — one malformed skill must not cost a
// universe the other two hundred. This is the build, where nothing has been
// distributed yet and the answer is different.
func assertNothingWasDropped(t *testing.T, name string, m *mbapi.Manifest) {
	t.Helper()
	if len(m.Drops) == 0 {
		return
	}
	lines := make([]string, 0, len(m.Drops))
	for _, d := range m.Drops {
		kind := d.Kind
		if kind == "" {
			kind = "?"
		}
		lines = append(lines, fmt.Sprintf("  %s [%s]: %s", d.Path, kind, d.Reason))
	}
	sort.Strings(lines)
	t.Errorf("census[%s]: %d file(s) published NOTHING:\n%s\n\n"+
		"A dropped file is not an invalid contribution — it is an absent one, so every "+
		"other check passes on a binary that is simply short. Fix the file, or delete it "+
		"if it was never meant to publish.",
		name, len(m.Drops), strings.Join(lines, "\n"))
}

// assertEveryKindIsComplete refuses a kind declaration that is missing any
// part of its contract — schema, layers with exactly one default, severity,
// metadata budget — an entry whose kind nothing declares, and a kind
// declared more than once: by two URIs, or by a member AND the system set
// (a member may not redeclare a system kind).
//
// This is the authoring-time half of goals G2: the aggregator only WARNS about
// an incomplete or duplicate declaration, because a live universe must not
// lose a kind's entries over a missing budget; here the author is present and
// the fix costs a minute, so it refuses — every finding, the warn-level
// identical duplicate included: two copies of one declaration is one copy
// too many wherever it is authored. Same findings, one function
// (Kinds.Check), two postures.
func assertEveryKindIsComplete(t *testing.T, name string, kinds contribapi.Kinds, cs []contribapi.Contribution) {
	t.Helper()
	findings := kindFindings(kinds, cs)
	if len(findings) == 0 {
		return
	}
	t.Errorf("census[%s]: %d kind declaration finding(s):\n%s\n\n"+
		"A kind is declared once, completely: schema, layers (exactly one default), validate, meta.budget.\n"+
		"The framework reads every one of them; an omission here is a behaviour nobody chose, and a second\n"+
		"declaration is one the universe resolves by precedence rather than by intent.",
		name, len(findings), strings.Join(findings, "\n"))
}

// kindFindings is the decision assertEveryKindIsComplete reports: one line
// per finding of Kinds.Check over the kinds the entries use, sorted. Pure,
// so the guard can be falsified without a binary.
func kindFindings(kinds contribapi.Kinds, cs []contribapi.Contribution) []string {
	used := make([]string, 0, len(cs))
	for _, c := range cs {
		used = append(used, c.Type)
	}
	var findings []string
	for _, f := range kinds.Check(used...) {
		if f.URI == "" && contribapi.IsMetaKind(f.Kind) {
			continue // the meta-model bootstraps before any declaration can be read
		}
		findings = append(findings, "  ["+f.Rule+"] "+f.String())
	}
	sort.Strings(findings)
	return findings
}

// assertForeignKindsResolve: every kind the binary publishes entries of and
// declares nowhere itself (neither under its provider nor in the system set)
// is declared in the universe the census was given, and the builder recorded
// exactly those kinds on the manifest.
//
// The strict half of blind audit H3. The builder no longer refuses a foreign
// kind — it cannot tell "declared elsewhere" from "declared nowhere" — so
// the refusal moved to the two places that can: the aggregator, per entry,
// at runtime; and here, at build, against the declarations the test names.
// A foreign kind nobody names is reported with every entry that carries it.
func assertForeignKindsResolve(t *testing.T, name string, m *mbapi.Manifest, declared []contribapi.Contribution) {
	t.Helper()
	if bad := UnresolvedForeignKinds(m, declared); len(bad) > 0 {
		t.Errorf("census[%s]: %d foreign kind finding(s):\n%s\n\n"+
			"A kind is declared once in a universe, possibly by another member; the census resolves a binary's\n"+
			"foreign kinds against the fixture packs the test names (census.FixtureDeclarations). Name the pack\n"+
			"that declares the kind, or declare it in this binary.",
			name, len(bad), strings.Join(bad, "\n"))
	}
}

// UnresolvedForeignKinds is the decision assertForeignKindsResolve reports,
// one line per finding, sorted. Pure, so the guard can be falsified without a
// binary.
func UnresolvedForeignKinds(m *mbapi.Manifest, declared []contribapi.Contribution) []string {
	own := contribapi.KindsFrom(m.Contributions)
	elsewhere := contribapi.KindsFrom(declared)
	entries := map[string][]string{} // foreign kind → entry URIs
	for _, c := range m.Contributions {
		if _, ok := own.Get(c.Type); ok || contribapi.IsMetaKind(c.Type) {
			continue
		}
		entries[c.Type] = append(entries[c.Type], c.ResolvedURI())
	}
	foreign := make([]string, 0, len(entries))
	for kind := range entries {
		foreign = append(foreign, kind)
	}
	sort.Strings(foreign)

	var bad []string
	recorded := append([]string(nil), m.ForeignKinds...)
	sort.Strings(recorded)
	if strings.Join(recorded, ",") != strings.Join(foreign, ",") {
		bad = append(bad, fmt.Sprintf("  the manifest records foreign kinds %v but its entries use undeclared kinds %v: the builder is the one writer of ForeignKinds",
			recorded, foreign))
	}
	for _, kind := range foreign {
		if _, ok := elsewhere.Get(kind); ok {
			continue
		}
		for _, uri := range entries[kind] {
			bad = append(bad, fmt.Sprintf("  %s: kind %q is declared by neither %s nor system, and no named fixture declares it",
				uri, kind, m.Provider))
		}
	}
	sort.Strings(bad)
	return bad
}

// assertDeclaredSchemasShip refuses a kind whose schema is declared under
// THIS binary's provider — or under `system`, which every binary embeds — and
// is not registered here.
//
// The hole this closes: a declared-but-unshipped schema was skipped as a
// cross-binary reference even when the schema's provider was the binary under
// test. `capability-panel` sat there for months, its kind pointing at a
// data-type nobody had written, every entry reading "data-type not found" in
// the GUI, and every census green.
func assertDeclaredSchemasShip(t *testing.T, name string, kinds contribapi.Kinds, registered map[string]bool) {
	t.Helper()
	var unshipped []string
	for _, kind := range kinds.Names() {
		schema := kinds.SchemaFor(kind)
		if schema == "" {
			continue // reported by assertEveryKindIsComplete
		}
		u, err := contribapi.ParseResourceURI(schema)
		if err != nil {
			unshipped = append(unshipped, fmt.Sprintf("  kind %q: schema %q is not a resource URI: %v", kind, schema, err))
			continue
		}
		if u.Provider != name && u.Provider != contribapi.SystemProvider {
			continue // a genuinely cross-binary reference; the aggregator resolves it
		}
		if !registered[schema] {
			unshipped = append(unshipped, fmt.Sprintf("  kind %q (%s): declares schema %s, which this binary does not ship",
				kind, kinds.DeclaredBy(kind), schema))
		}
	}
	if len(unshipped) == 0 {
		return
	}
	sort.Strings(unshipped)
	t.Errorf("census[%s]: %d kind(s) declare a schema under their own provider that is not published:\n%s\n\n"+
		"A schema a kind declares under its own provider ships with it (goals G2). Add the data-type next to the kind.",
		name, len(unshipped), strings.Join(unshipped, "\n"))
}

// assertMetaDerivedFromData: for a kind that declares `meta.from`, an entry's
// meta is what its data layer derives — field for field. A disagreement is a
// second writer of the same name, which is how a skill appeared twice in a
// listing (c22) and what deriving exists to make impossible.
func assertMetaDerivedFromData(t *testing.T, name string, kinds contribapi.Kinds, cs []contribapi.Contribution) {
	t.Helper()
	var bad []string
	for _, c := range cs {
		for _, d := range contribapi.MetaDisagreements(c, kinds) {
			bad = append(bad, "  "+d)
		}
	}
	if len(bad) == 0 {
		return
	}
	sort.Strings(bad)
	t.Errorf("census[%s]: %d meta field(s) disagree with the data layer they are derived from:\n%s\n\n"+
		"The producer must write these fields in the data layer only; the builder fills meta.",
		name, len(bad), strings.Join(bad, "\n"))
}

// assertRequiredLayersCarried: every layer a kind declares `required: true`
// is present on every entry of that kind. The declaration had no reader
// until 2026-09-22 — a required layer nobody checked was a promise nothing
// kept.
func assertRequiredLayersCarried(t *testing.T, name string, kinds contribapi.Kinds, cs []contribapi.Contribution) {
	t.Helper()
	missing := missingRequiredLayers(kinds, cs)
	if len(missing) == 0 {
		return
	}
	t.Errorf("census[%s]: %d entry(ies) lack a layer their kind requires:\n%s", name, len(missing), strings.Join(missing, "\n"))
}

// missingRequiredLayers is the decision assertRequiredLayersCarried reports:
// one line per (entry, required layer it lacks), sorted. Pure, so the guard
// can be falsified without a binary.
func missingRequiredLayers(kinds contribapi.Kinds, cs []contribapi.Contribution) []string {
	var missing []string
	for _, c := range cs {
		d, ok := kinds.Get(c.Type)
		if !ok {
			continue // reported by assertEveryKindIsComplete
		}
		for layer, decl := range d.Layers {
			if !decl.Required {
				continue
			}
			if _, carried := c.Layers[layer]; !carried {
				missing = append(missing, fmt.Sprintf("  %s: kind %q requires layer %q, entry carries none", c.ResolvedURI(), c.Type, layer))
			}
		}
	}
	sort.Strings(missing)
	return missing
}

// assertImmutableProvidersCarryADigest holds the identity rule on every
// provider row the builder recorded: an immutable source names the version it
// serves (its enumeration digest, contribapi.DigestOf), a mutable one carries
// none. An immutable row without a digest is a cache key that cannot tell two
// versions apart; a mutable row with one claims a single version a changing
// source does not have.
func assertImmutableProvidersCarryADigest(t *testing.T, name string, m *mbapi.Manifest) {
	t.Helper()
	if bad := identityRuleViolations(m); len(bad) > 0 {
		t.Errorf("census[%s]: %d provider(s) break the identity rule (immutable => digest, mutable => none):\n%s",
			name, len(bad), strings.Join(bad, "\n"))
	}
}

// identityRuleViolations is the decision assertImmutableProvidersCarryADigest
// reports, one line per offending row, sorted. Pure, so the guard can be
// falsified without a binary.
func identityRuleViolations(m *mbapi.Manifest) []string {
	var bad []string
	for _, p := range m.Providers {
		if why := contribapi.IdentityRule(contribapi.Identity{Digest: p.Digest, Mutable: p.Mutable}); why != "" {
			bad = append(bad, fmt.Sprintf("  %s/%s: %s", p.Name, p.Type, why))
		}
	}
	sort.Strings(bad)
	return bad
}

// assertMetaWithinBudget refuses an entry whose serialised metadata exceeds
// the budget its kind declares.
//
// Metadata is the one part of a contribution multiplied by the size of the
// catalog (goals G6): it is always resident, in every index, on every wire.
// The kind bounds it, and the bound is measured — the first run of this check
// printed every entry over the provisional budgets with its size, and the
// budgets were set from those numbers plus a margin. A budget nobody measured
// is a guess.
func assertMetaWithinBudget(t *testing.T, name string, kinds contribapi.Kinds, cs []contribapi.Contribution) {
	t.Helper()
	var over []string
	for _, c := range cs {
		budget := kinds.Budget(c.Type)
		if budget <= 0 || c.Metadata == nil {
			continue // an undeclared budget is reported by assertEveryKindIsComplete
		}
		raw, err := json.Marshal(c.Metadata)
		if err != nil {
			continue
		}
		if len(raw) > budget {
			over = append(over, fmt.Sprintf("  %s: meta is %d B, kind %q allows %d", c.ResolvedURI(), len(raw), c.Type, budget))
		}
	}
	if len(over) == 0 {
		return
	}
	sort.Strings(over)
	t.Errorf("census[%s]: %d entry(ies) exceed their kind's metadata budget:\n%s\n\n"+
		"Move what is not picker-level into a layer, or raise the kind's meta.budget deliberately.",
		name, len(over), strings.Join(over, "\n"))
}

// assertContentLintClean: every skill and task this binary publishes obeys
// its kind's format contract, and every relative link, own-provider resource
// URI and own-provider tool URI in its body names something the binary
// publishes. References to OTHER providers are the universe lint's
// (`ktw-compose lint-universe`): one binary cannot know what another
// publishes. Warnings are printed, errors fail — a sidecar ships no skill
// nobody can read (ms-contribution-ext e10).
func assertContentLintClean(t *testing.T, name string, m *mbapi.Manifest, sources map[string][]byte) {
	t.Helper()
	open := func(uri, _ string) ([]byte, error) {
		if b, ok := sources[uri]; ok {
			return b, nil
		}
		return nil, fmt.Errorf("no source enumerated for %s", uri)
	}
	findings := contentlint.LintManifest(m, open, contentlint.NewUniverse([]*mbapi.Manifest{m}), contentlint.ScopeSelf)
	var errs, warns []string
	for _, f := range findings {
		if f.Severity == verify.SeverityError {
			errs = append(errs, "  "+f.String())
		} else {
			warns = append(warns, "  "+f.String())
		}
	}
	if len(warns) > 0 {
		t.Logf("census[%s]: %d content lint warning(s):\n%s", name, len(warns), strings.Join(warns, "\n"))
	}
	if len(errs) == 0 {
		return
	}
	t.Errorf("census[%s]: %d skill/task content error(s):\n%s\n\n"+
		"Fix the file, or run `ktw-compose lint-skill` / `lint-task` on it for the same findings.",
		name, len(errs), strings.Join(errs, "\n"))
}
