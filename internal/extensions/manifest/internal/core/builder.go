// SPDX-License-Identifier: Apache-2.0

// Package core holds private state + the Builder that aggregates resources.
package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Builder produces a Manifest by walking every contributed ContributionProvider
// in ContributionProvidersPoint. It does NOT touch runtime services — providers
// must be pure-data per the Provider contract.
//
// Slot types and slots are populated from the local KTSP Endpoint when one
// is registered (named service "default") — see `slot_mapping.go`. Binaries
// without a KTSP container (e.g. ktw-hub itself) emit empty SlotTypes /
// Slots and have an all-resources descriptor.
type Builder struct {
	provider string
	version  string
	// kindDecls are kind declarations the build reads its entries against
	// (residency, derived meta) without publishing them: a folder member's
	// manifest, built by the hub, carries no system set of its own.
	kindDecls []contribapi.Contribution
}

// WithKindDeclarations makes the build read its entries against these kind
// declarations too, as a binary's build reads them against the system set it
// embeds. They are not published.
func (b *Builder) WithKindDeclarations(decls []contribapi.Contribution) *Builder {
	b.kindDecls = decls
	return b
}

// NewBuilder creates a Builder for the given binary identity.
//   - provider: the binary name (e.g. "ktw-docs")
//   - version: the binary version (e.g. "1.4.2", "dev")
//
// SlotIntrospector reports this binary's KTSP slot surface, or nothing when it
// has none. Installed by the manifest's local/ package; nil here.
//
// # Why a seam rather than the call
//
// Build used to resolve `*ktsp.Endpoint` itself and map it — transport wiring
// reaching into the domain layer, which is what `no-ktsp-in-internal` forbids
// and why this package sat on that rule's backlog. What a manifest NEEDS is the
// slot surface; WHERE that comes from is a wiring question, and wiring is
// local/'s job.
//
// A package-level var rather than a Builder field because the resource service
// in this package constructs its own Builder, and a binary has exactly one KTSP
// endpoint — the same shape the extension points already use.
var SlotIntrospector func(ctx context.Context) ([]mbapi.SlotType, []mbapi.Slot)

func NewBuilder(provider, version string) *Builder {
	return &Builder{provider: provider, version: version}
}

// Build aggregates every contributed ContributionProvider (read from the EM via
// `application.UseExtensions[ContributionProvider]`) into a Manifest, and
// populates SlotTypes + Slots from the local KTSP Endpoint if one is
// registered as named service "default".
// Resources are sorted by (provider, type, id) for hash determinism.
// The Manifest's Hash field is sha256 over the canonical JSON of the
// (provider, version, slotTypes, slots, resources) tuple.
func (b *Builder) Build(ctx context.Context) (*mbapi.Manifest, error) {
	providers := application.UseExtensions[contribapi.ContributionProvider](ctx)
	m, err := b.BuildFromProviders(ctx, providers)
	if err != nil {
		return nil, err
	}
	m.ReadRoots = sortedReadRoots(application.UseExtensions[mbapi.ReadRoot](ctx))
	// Optional KTSP introspection, through the seam below. Silent when no
	// introspector is installed — not every binary has a KTSP container
	// (ktw-hub itself doesn't).
	if SlotIntrospector != nil {
		if slotTypes, slots := SlotIntrospector(ctx); len(slotTypes) > 0 || len(slots) > 0 {
			m.SlotTypes, m.Slots = slotTypes, slots
			// Re-hash now that SlotTypes/Slots are populated.
			m.Hash = computeHash(m)
		}
	}

	return m, nil
}

// BuildFromProviders builds a Manifest from an explicit provider list.
// Used by Build (with EM-fetched providers) and by tests (with a hand-rolled list).
//
// Does NOT populate SlotTypes / Slots — callers that need those should
// use Build (which resolves the Endpoint from the application) or set
// them on the returned Manifest directly and call ReHash.
func (b *Builder) BuildFromProviders(ctx context.Context, providers []contribapi.ContributionProvider) (*mbapi.Manifest, error) {
	var contributions []contribapi.Contribution
	// sources[i] names the provider that enumerated contributions[i]
	// (<provider>/<type>): a merge's copies from one source are a provider
	// registered twice, not two providers disagreeing.
	var sources []string
	var drops []contribapi.Drop
	var overrides []contribapi.Override
	var overrideEntries []contribapi.OverrideEntry
	var infos []mbapi.ProviderInfo
	for _, p := range providers {
		entries, err := p.Enumerate(ctx)
		if err != nil {
			return nil, fmt.Errorf("enumerate %s/%s: %w", p.Provider(), p.Type(), err)
		}
		// Identity AFTER Enumerate: it names the version just enumerated (an
		// immutable provider's digest; a mutable one's none), and a provider
		// whose enumeration needs this context stamps its digest there.
		id := p.Identity()
		infos = append(infos, mbapi.ProviderInfo{Name: p.Provider(), Type: p.Type(), Digest: id.Digest, Mutable: id.Mutable})
		// Derive URI for any Contribution the producer left unset.
		for i := range entries {
			if entries[i].URI == "" {
				entries[i].URI = entries[i].ResolvedURI()
			}
		}
		contributions = append(contributions, entries...)
		for range entries {
			sources = append(sources, p.Provider()+"/"+p.Type())
		}
		// What this provider MET and could not publish. Carried in the
		// manifest so it reaches packaging, which is not watching any log and
		// otherwise sees a pack that is simply short.
		if r, ok := p.(contribapi.DropReporter); ok {
			drops = append(drops, r.Drops()...)
		}
		if r, ok := p.(contribapi.OverrideReporter); ok {
			overrides = append(overrides, r.Overrides()...)
		}
		if r, ok := p.(contribapi.OverrideEntryReporter); ok {
			overrideEntries = append(overrideEntries, r.OverrideEntries()...)
		}
	}

	// Dedupe by URI: when multiple providers emit the same logical
	// entry (e.g., two providers each surfacing the same
	// `<binary>:/config-file/<name>` entry), keep ONE entry. The richer
	// payload wins: prefer entries with a non-empty Body, then those
	// with HasDefault=true semantics; first-seen wins as the tiebreaker.
	// Every merge is a finding in the builder's log, never a silent one.
	var merged []DedupeDrop
	contributions, merged = dedupeByURI(contributions, sources)
	b.logMerges(merged)

	sort.Slice(contributions, func(i, j int) bool {
		if contributions[i].Provider != contributions[j].Provider {
			return contributions[i].Provider < contributions[j].Provider
		}
		if contributions[i].Type != contributions[j].Type {
			return contributions[i].Type < contributions[j].Type
		}
		return contributions[i].ID < contributions[j].ID
	})

	// The kind registry, built from the declarations this binary carries — the
	// system set it embeds and its own. It is the ONE reader of a declaration
	// here: schema, residency, severity all come from it.
	//
	// A kind declared by neither is FOREIGN, not undeclared. A kind is declared
	// once in a universe, and the member declaring it may be another one (a
	// files-only pack declares `recipe`; a Go producer publishes recipes). This
	// builder sees one member, so it cannot tell "declared elsewhere" from
	// "declared nowhere" — the aggregator can, and it refuses the ENTRY whose
	// kind no member declares (catalog.Reindex), never a member's whole
	// manifest. So the entries are kept and the kind is recorded on the
	// manifest (ForeignKinds); the census, which can name the declared
	// universe, refuses a foreign kind it cannot resolve. Refusing here made
	// the rule depend on the producer's language (blind audit H3).
	kinds := contribapi.KindsFrom(append(append([]contribapi.Contribution(nil), b.kindDecls...), contributions...))
	used := make([]string, 0, len(contributions))
	for i := range contributions {
		used = append(used, contributions[i].Type)
	}
	var foreign []string
	for _, f := range kinds.Check(used...) {
		if f.URI == "" {
			// The meta-model bootstraps before any declaration can be read;
			// a view without the system provider still resolves it (Kinds.SchemaFor).
			if !contribapi.IsMetaKind(f.Kind) {
				foreign = append(foreign, f.Kind)
			}
			continue
		}
		// An INCOMPLETE or DUPLICATE declaration is reported, not refused, at
		// runtime: one missing budget must not cost a universe the kind's
		// entries. The census refuses it at authoring, where the author is
		// present.
		msg := "manifest: incomplete kind declaration"
		if f.Rule != contribapi.KindRuleIncomplete {
			msg = "manifest: duplicate kind declaration"
		}
		slog.Warn(msg, "rule", f.Rule, "severity", f.Severity, "finding", f.String())
	}
	if len(foreign) > 0 {
		sort.Strings(foreign)
		slog.Info("manifest: entries of kinds declared by neither this binary nor system; the aggregator resolves them against the other members' declarations",
			"provider", b.provider, "kinds", foreign)
	}

	// Read before residency drops any layer: a data-type's version is in its
	// source layer.
	formats := contribapi.DeriveFormats(contributions)

	// Residency, decided HERE and nowhere else. A provider carries the bytes it
	// has (up to the bulk band); the kind says which layers a LISTING needs; the
	// manifest keeps those and drops the rest. Before this the decision was a Go
	// field on one provider, set at one call site — a declaration in the wrong
	// place, invisible to a files-only author.
	for i := range contributions {
		// Meta derived HERE for a kind that declares `meta.from`: the producer
		// wrote the picker fields once, in the data layer; the index carries
		// them in meta because a listing reads meta. One writer (e8).
		if err := contribapi.ApplyDerivedMeta(&contributions[i], kinds); err != nil {
			return nil, fmt.Errorf("derive meta: %w", err)
		}
		applyResidency(&contributions[i], kinds)
	}

	// Override entries, applied after the dedupe and the residency: their
	// layers stay in the record (they are not the producer's to open).
	applied, carried, refused := applyOverrideEntries(contributions, overrideEntries, overrides, kinds)
	overrides = append(overrides, applied...)
	drops = append(drops, refused...)

	// The formats this binary writes, derived HERE from what it contributes
	// and the data-types its kinds name, into its sidecar-info (`formats`):
	// a host compares them with its own before reading the entries
	// (contribapi.JudgeFormat). Derived, never authored.
	if len(formats) > 0 {
		for i := range contributions {
			if contributions[i].Type == contribapi.SidecarInfoKind {
				if err := contribapi.SetFormats(&contributions[i], formats); err != nil {
					return nil, fmt.Errorf("sidecar-info formats: %w", err)
				}
			}
		}
	}

	// Every contribution leaves here with a name.
	//
	// `ResolvedMetadata` derives one at READ time, which serves a Go consumer
	// and nobody else: the wire form carried `"name": ""` for 139 of 155
	// entries, so a polyglot consumer — the GUI's TypeScript, any non-Go
	// reader — saw an empty label and had to invent its own fallback. Deriving
	// it HERE means the answer is computed once, by the side that knows the
	// identity, and travels.
	for i := range contributions {
		c := &contributions[i]
		if c.Metadata == nil {
			meta := c.ResolvedMetadata()
			c.Metadata = &meta
			continue
		}
		if c.Metadata.Name == "" {
			c.Metadata.Name = c.ResolvedMetadata().Name
		}
	}

	m := b.makeManifest(contributions)
	// Sorted, so a manifest is byte-stable: two runs over the same folder must
	// produce the same descriptor, or the content digest that drives staleness
	// churns for no reason.
	sort.Slice(drops, func(i, j int) bool { return drops[i].Path < drops[j].Path })
	m.Drops = drops
	sort.Slice(overrides, func(i, j int) bool {
		if overrides[i].URI != overrides[j].URI {
			return overrides[i].URI < overrides[j].URI
		}
		return overrides[i].Layer < overrides[j].Layer
	})
	m.Overrides = overrides
	if len(carried) > 0 {
		m.OverrideEntries = carried
		m.Hash = computeHash(m)
	}
	m.ForeignKinds = foreign
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Name != infos[j].Name {
			return infos[i].Name < infos[j].Name
		}
		return infos[i].Type < infos[j].Type
	})
	m.Providers = infos
	return m, nil
}

// HashBody returns the content address of a contribution body, or "" for an
// empty body (no address). The address itself is contribapi.HashContent — the
// one hash every layer, transport and verifier uses; this is only the
// empty-body guard, kept for the manifest/local re-export that still names
// it. New code calls contribapi.HashContent.
func HashBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	return contribapi.HashContent(body)
}

// DedupeDrop is one URI the builder saw more than once and kept once: the
// URI and how many copies it had. Recorded so the builder's log says what it
// merged — a silent merge reads as a pack that is simply short of an entry
// (blind audit L2).
type DedupeDrop struct {
	URI    string
	Copies int
	// Sources are the distinct providers (<provider>/<type>) the copies came
	// from; one source means that provider was registered twice.
	Sources []string
}

// warnedMerges remembers the merge findings already warned in this process,
// by builder provider and URIs: a catalog rebuilds its manifest many times a
// start, and the same finding warned at every build filled the log.
var warnedMerges sync.Map

// logMerges reports what the build merged in one line: a warning naming the
// URIs several providers published, once per process for the same set; the
// copies of one provider registered twice only at debug level.
func (b *Builder) logMerges(merged []DedupeDrop) {
	var several, twice []string
	for _, d := range merged {
		if len(d.Sources) == 1 {
			twice = append(twice, d.URI)
			continue
		}
		several = append(several, d.URI)
	}
	if len(twice) > 0 {
		slog.Debug("manifest: one provider registered twice; its copies merged",
			"provider", b.provider, "uris", len(twice), "first", twice[0])
	}
	if len(several) == 0 {
		return
	}
	key := b.provider + "|" + strings.Join(several, "|")
	if _, seen := warnedMerges.LoadOrStore(key, true); seen {
		return
	}
	slog.Warn("manifest: URIs published by several providers; one copy of each kept",
		"provider", b.provider, "count", len(several), "uris", strings.Join(several, " "))
}

// dedupeByURI collapses duplicate Contributions (same `<provider>:/<type>/<id>`
// URI) emitted by different providers. The winner is selected by
// `preferContribution`: for config-file entries, the one with
// HasDefault=true wins (an extension-supplied template beats a
// binary-walked declaration that knows of no default); for every other
// type, the entry with the non-empty Body wins, then first-seen. Every URI
// that had more than one copy is returned as a DedupeDrop, in input order.
func dedupeByURI(in []contribapi.Contribution, sources []string) ([]contribapi.Contribution, []DedupeDrop) {
	bestIdx := make(map[string]int, len(in))
	copies := make(map[string]int, len(in))
	from := make(map[string][]string, len(in))
	var order []string
	for i, c := range in {
		uri := c.URI
		if uri == "" {
			uri = c.ResolvedURI()
		}
		prev, ok := bestIdx[uri]
		if !ok {
			order = append(order, uri)
		}
		copies[uri]++
		src := ""
		if i < len(sources) {
			src = sources[i]
		}
		if !slices.Contains(from[uri], src) {
			from[uri] = append(from[uri], src)
		}
		if !ok || preferContribution(in[i], in[prev]) {
			bestIdx[uri] = i
		}
	}
	out := make([]contribapi.Contribution, 0, len(bestIdx))
	for i, c := range in {
		uri := c.URI
		if uri == "" {
			uri = c.ResolvedURI()
		}
		if bestIdx[uri] == i {
			out = append(out, c)
		}
	}
	var drops []DedupeDrop
	for _, uri := range order {
		if copies[uri] > 1 {
			drops = append(drops, DedupeDrop{URI: uri, Copies: copies[uri], Sources: from[uri]})
		}
	}
	return out, drops
}

// preferContribution returns true when `a` should replace `b` as the
// dedupe winner for the same URI.
//
// Per-type rules:
//   - config-file: a wins if a.HasDefault is true and b.HasDefault is
//     not (extension-supplied templates beat empty WithConfigFiles
//     declarations).
//   - default: a wins if a has a non-empty Body and b doesn't (any
//     payload beats an empty marker entry).
func preferContribution(a, b contribapi.Contribution) bool {
	if a.Type == mbapi.ResourceTypeConfigFile {
		aDefault := configFileHasDefault(a)
		bDefault := configFileHasDefault(b)
		if aDefault && !bDefault {
			return true
		}
		if !aDefault && bDefault {
			return false
		}
	}
	if len(a.Data()) > 0 && len(b.Data()) == 0 {
		return true
	}
	return false
}

// configFileHasDefault decodes the ConfigFileBody and returns its
// HasDefault flag. Returns false on any unmarshal error — we treat a
// malformed body as "no default" rather than failing the build.
func configFileHasDefault(c contribapi.Contribution) bool {
	if len(c.Data()) == 0 {
		return false
	}
	var body mbapi.ConfigFileBody
	if err := json.Unmarshal(c.Data(), &body); err != nil {
		return false
	}
	return body.HasDefault
}

// ReHash recomputes the Hash field on the manifest. Use after manually
// mutating SlotTypes / Slots / Resources outside of Build.
func ReHash(m *mbapi.Manifest) {
	m.Hash = computeHash(m)
}

// FindProvider returns the contributed ContributionProvider matching (provider, type),
// or nil if none. Used by ResourceService.Get to locate the right provider for a URI.
func FindProvider(ctx context.Context, provider, resourceType string) contribapi.ContributionProvider {
	for _, p := range application.UseExtensions[contribapi.ContributionProvider](ctx) {
		if p.Provider() == provider && p.Type() == resourceType {
			return p
		}
	}
	return nil
}

// FindProviders returns EVERY contributed ContributionProvider matching
// (provider, type), in contribution order. A single (provider, type) pair can
// be served by more than one provider — e.g. ktw-agents serves "data-type"
// from BOTH the file-backed embed provider AND the virtual method-I/O provider
// (service-method Input/Output schemas). Read must try each until one serves
// the URI, so virtual contributions aren't shadowed by a file-backed provider
// that happens to be registered first.
func FindProviders(ctx context.Context, provider, resourceType string) []contribapi.ContributionProvider {
	var out []contribapi.ContributionProvider
	for _, p := range application.UseExtensions[contribapi.ContributionProvider](ctx) {
		if p.Provider() == provider && p.Type() == resourceType {
			out = append(out, p)
		}
	}
	return out
}

func (b *Builder) makeManifest(contributions []contribapi.Contribution) *mbapi.Manifest {
	m := &mbapi.Manifest{
		Provider:      b.provider,
		Version:       b.version,
		Contributions: contributions,
	}
	m.Hash = computeHash(m)
	return m
}

// computeHash returns the sha256 of the canonical JSON of the manifest's
// (provider, version, slotTypes, slots, resources, contributions) tuple.
// Same input produces the same hash across encoders thanks to the
// sort-on-build invariants:
//   - Resources sorted by (provider, type, id) in BuildFromProviders.
//   - Contributions mirror Resources in the same sorted order during
//     dual-emission (ResourceToContribution preserves Provider/Type/ID).
//   - SlotTypes / Slots arrive in the order Endpoint.Info() returns them,
//     which is itself stable (Endpoint.Info sorts slots by name).
//   - Method tags within a SlotType are sorted in tagsToSlice (slot_mapping.go).
//
// Contributions is included in the hash input so producer migrations
// (richer Metadata / Schema emitted directly) change the
// content address — consumers re-decode the new shape automatically.
func computeHash(m *mbapi.Manifest) string {
	hashInput := struct {
		Provider      string                    `json:"provider"`
		Version       string                    `json:"version"`
		SlotTypes     []mbapi.SlotType          `json:"slotTypes,omitempty"`
		Slots         []mbapi.Slot              `json:"slots,omitempty"`
		Contributions []contribapi.Contribution `json:"contributions"`
		// Absent when empty: a manifest without override entries hashes as
		// it always did.
		OverrideEntries []contribapi.OverrideEntry `json:"overrideEntries,omitempty"`
	}{m.Provider, m.Version, m.SlotTypes, m.Slots, m.Contributions, m.OverrideEntries}

	b, err := json.Marshal(hashInput)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// applyResidency keeps the layers the kind declares resident and drops the
// bytes of every other layer except `data`.
//
// The data layer is exempt: it IS the index's structured body, and the wire
// applies the inline band to it on its own (DropInline). Copy-on-write for the
// same reason DropInline is — the map may be shared with a provider's cache.
func applyResidency(c *contribapi.Contribution, kinds contribapi.Kinds) {
	var out map[string]contribapi.LayerRef
	for name, l := range c.Layers {
		if name == contribapi.LayerData || l.Inline == nil || kinds.Resident(c.Type, name) {
			continue
		}
		if out == nil {
			out = make(map[string]contribapi.LayerRef, len(c.Layers))
			for k, v := range c.Layers {
				out[k] = v
			}
		}
		l.Inline = nil
		out[name] = l
	}
	if out != nil {
		c.Layers = out
	}
}

// sortedReadRoots is the binary's read-root declarations, deduplicated and
// sorted by (anchor, path): the manifest is byte-stable whatever order the
// declarations were contributed in.
func sortedReadRoots(declared []mbapi.ReadRoot) []mbapi.ReadRoot {
	if len(declared) == 0 {
		return nil
	}
	seen := make(map[mbapi.ReadRoot]bool, len(declared))
	out := make([]mbapi.ReadRoot, 0, len(declared))
	for _, r := range declared {
		if r.Anchor == "" || r.Path == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Anchor != out[j].Anchor {
			return out[i].Anchor < out[j].Anchor
		}
		return out[i].Path < out[j].Path
	})
	return out
}
