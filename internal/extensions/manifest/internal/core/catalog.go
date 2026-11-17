// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"sync"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// CatalogImpl is the in-memory Catalog backed by a (name → Manifest) map
// populated from the per-sidecar Cache on construction (Reload).
//
// Members are the sidecars listed in Univers.yaml — the universe
// membership, each a name and the binary declared for it. Each member's
// cached manifest lives at `<Cache.Root>/<name>@<key>/manifest.json`
// alongside a `metadata.yaml` carrying the exe stat used for staleness
// checks, `<key>` being the declared binary's. Sidecars listed in
// Univers.yaml without any record appear in Names() but not in
// List() — Get() returns ErrSidecarNotInCatalog for them.
//
// Reload is concurrency-safe (RWMutex). The intended call pattern is
// "load once at universe activation; refresh after `catalog refresh
// <name>` writes a fresh cache folder."
//
// Two-phase bootstrap state (per hub-boot-order ADR):
//
//   - schema: attached by SetSchema after Phase 1 loads data-type
//     Contributions; nil during Phase 1.
//   - ready: flips to true at MarkReady (end of Phase 2). Consumers
//     may query Ready() to know if catalog is fully indexed.
type CatalogImpl struct {
	// kinds is the registry over every member's declarations, built once per
	// reload (kindsCached) and answered from memory; kindsBuilds counts the
	// builds for the test that holds it to one per reload.
	kinds       contribapi.Kinds
	kindsCached bool
	kindsBuilds int
	cache       *Cache
	mu          sync.RWMutex
	members     []Member                   // membership in declared order: name + declared binary
	by          map[string]*mbapi.Manifest // loaded manifests, keyed by MEMBER name (a member may publish under another provider name)

	// hosts maps a contribution URI to the MEMBER whose manifest carries it —
	// the name the universe declares and the hub acquires a connection by,
	// which is not always the URI's provider: a member may publish under
	// another provider name (a ktw-host adopting its folder's), and a layer
	// provider's
	// contributions live inside another binary's manifest. Resolving that by
	// walking every manifest on every read is the single hottest scan in the
	// system, since it runs per Read rather than per load.
	hosts map[string]string

	// entries places every contribution by its resolved URI: the manifest that
	// carries it and its index there. Built with hosts, on every load, so a
	// by-URI read is one lookup whatever the catalog's size (G17) — GetBody
	// and ContributionAt used to walk a manifest's entries and, on a miss,
	// every manifest.
	entries map[string]entryLoc

	// providers are the provider names the loaded manifests publish under —
	// what tells a URI of a known provider (an unknown entry) from one of a
	// provider no member publishes.
	providers map[string]bool

	// collisions records every URI more than one member carries: the host
	// that resolved it (the first in member order) and the members shadowed.
	// A finding, not a refusal — the entries stay, one of them answers.
	collisions []URICollision

	// overrideFindings are the override entries a member carried that the
	// last load could not apply: no member serves the entry they override,
	// or they are refused. Findings, not errors: the other entries stay.
	overrideFindings []string

	// edges holds each contribution's outgoing references, derived at ingest
	// and keyed by URI.
	//
	// An index rather than an envelope field. As a field it had two writers —
	// the catalog and any producer — with producer values silently
	// overwritten, which is a contradiction rather than a contract; and it
	// travelled to every consumer, none of which read it. Derived state
	// belongs where it is derived.
	//
	// Its purpose is the whole-graph check at ingest (concepts.md §11): the
	// aggregator is the only place that holds every contribution at once, so
	// it is the only place that can see a reference resolving nowhere.
	edges map[string][]contribapi.ContributionEdge

	// Bootstrap protocol state (hub-boot-order ADR).
	schema mbapi.CatalogSchemaService // attached at SetSchema; nil during Phase 1
	ready  bool                       // flips at MarkReady

	// valCfg is the central validation toggle consulted during Reindex.
	// Zero value (empty Validate + nil PerType) resolves to the
	// documented defaults via SeverityFor — so an unset config still
	// validates with warn-global / error-strict posture. Wired by the
	// hub bootstrap via SetValidationConfig.
	valCfg contribapi.ContributionsConfig

	// regOutcomes is what registering each data-type produced, keyed by URI;
	// set by the aggregator's Phase 2 before Reindex, read by applyValidation
	// for source-only data-type entries. See mbapi.RegistrationOutcomeSetter.
	regOutcomes map[string]mbapi.RegistrationOutcome

	// Event publisher (4K contributions-changed-event). Nil-safe: when
	// unset, emission is silently skipped (binaries that don't need
	// cross-process notification — tests, in-process consumers — pay
	// no overhead).
	eventPub contribapi.ContributionsEventPublisher
}

// NewCatalog creates a catalog backed by the given Cache and the initial
// membership: each member's name and the binary the universe declares for
// it (typically the `sidecars:` of Univers.yaml), which together address
// the member's record. Reload is called once during construction so the
// catalog is immediately usable.
func NewCatalog(cache *Cache, members []Member) (*CatalogImpl, error) {
	c := &CatalogImpl{
		cache:   cache,
		members: append([]Member(nil), members...),
		by:      make(map[string]*mbapi.Manifest, len(members)),
		edges:   make(map[string][]contribapi.ContributionEdge),
	}
	if err := c.Reload(); err != nil {
		return nil, err
	}
	return c, nil
}

// Reload re-reads every member's cached manifest from disk — the record of
// the binary the membership declares for it, never another universe's
// record of the same name. Members without a record are simply absent from
// the map (Get returns ErrSidecarNotInCatalog).
//
// Emits a coarse `contributions-changed` event with KindsChanged=["*"] on
// success (when an event publisher is attached) — refresh subscribers
// can re-fetch and recompute their own diff. A finer per-provider
// emission path lives in SetMembers's per-name follow-up (next iteration).
//
// Cache staleness is NOT checked here — Reload is the cheap boot path
// and trusts whatever the cache holds. The user-facing `catalog refresh`
// command is the authoritative path for re-fetching from a live sidecar
// when its binary changes (it stats the binary and re-spawns via
// connmgr).
func (c *CatalogImpl) Reload() error {
	c.mu.Lock()
	loaded := make(map[string]*mbapi.Manifest, len(c.members))
	for _, member := range c.members {
		m, _, err := c.cache.Load(member.Name, member.Binary)
		if err != nil {
			c.mu.Unlock()
			return fmt.Errorf("catalog: load %s: %w", member.Name, err)
		}
		if m == nil {
			continue
		}
		loaded[member.Name] = universeVisible(m)
	}
	c.by = loaded
	c.kindsCached = false
	c.reindexHosts()
	pub := c.eventPub
	c.mu.Unlock()

	c.emitChanged(pub, "", []string{"*"})
	return nil
}

// universeVisible narrows a loaded record to what an aggregate may hold: the
// universe reach's visibility (mbapi.Filter at ReachUniverse — system yes,
// local no). A `local:` entry is binary-private and never crosses a process
// boundary; the probe no longer asks a member for them, but a record written
// by an older aggregator into the shared cache still carries them, and the
// catalog is the one place every read (List, Get, by-type, by-URI) derives
// from. Dropping them here means no list path needs a rule of its own (blind
// audit M3). The manifest is re-hashed when narrowed so its digest names the
// entries it actually holds.
func universeVisible(m *mbapi.Manifest) *mbapi.Manifest {
	kept := mbapi.Filter{}.Apply(m.Contributions, mbapi.ReachUniverse)
	if len(kept) == len(m.Contributions) {
		return m
	}
	m.Contributions = kept
	ReHash(m)
	return m
}

// SetMembers replaces the membership and reloads from disk. Useful when
// Univers.yaml changes between catalog ticks.
func (c *CatalogImpl) SetMembers(members []Member) error {
	c.mu.Lock()
	c.members = append([]Member(nil), members...)
	c.mu.Unlock()
	return c.Reload()
}

// Get implements Catalog.
func (c *CatalogImpl) Get(ctx context.Context, name string) (*mbapi.Manifest, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.by[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", mbapi.ErrSidecarNotInCatalog, name)
	}
	return m, nil
}

// List implements Catalog.
func (c *CatalogImpl) List(ctx context.Context) ([]*mbapi.Manifest, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*mbapi.Manifest, 0, len(c.by))
	for _, member := range c.members {
		name := member.Name
		if m, ok := c.by[name]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// Names implements Catalog.
func (c *CatalogImpl) Names(ctx context.Context) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.members))
	for _, member := range c.members {
		out = append(out, member.Name)
	}
	return out, nil
}

// GetContributionsByType implements Catalog. Walks every loaded manifest and
// collects entries with a matching type, in member-declared order then
// by Contribution.ID.
func (c *CatalogImpl) GetContributionsByType(ctx context.Context, t string) ([]contribapi.Contribution, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var out []contribapi.Contribution
	for _, member := range c.members {
		name := member.Name
		m, ok := c.by[name]
		if !ok {
			continue
		}
		for i, e := range m.Contributions {
			if e.Type != t {
				continue
			}
			// One URI, one entry: a copy another member shadows (URICollision)
			// is the one the by-URI index does not point at, and a listing
			// must answer with the same entry a by-URI read does.
			if loc := c.entries[uriOf(m, &m.Contributions[i])]; loc.m != m || loc.i != i {
				continue
			}
			out = append(out, e)
		}
	}
	// Stable secondary sort by ID — primary key (provider) preserves
	// member order via the outer loop.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return false // keep member order
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// GetBody implements Catalog: the DATA layer of one entry, from the loaded
// manifests — no content store, no bus. Returns an error when the URI is
// unknown; (nil, nil) when the entry has no data layer. The body lane answers
// one question; a kind whose content lives in `source` is read through
// Open(uri, layer), never through here — it used to fall back to the source
// layer, which is how YAML reached a JSON decoder.
func (c *CatalogImpl) GetBody(ctx context.Context, uri string) ([]byte, error) {
	parsed, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return nil, fmt.Errorf("catalog GetBody: parse %q: %w", uri, err)
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	loc, ok := c.entries[parsed.String()]
	if !ok {
		if !c.providers[parsed.Provider] {
			return nil, notInCatalog(uri)
		}
		return nil, contribapi.ErrContributionNotFound{URI: uri}
	}
	return loc.m.Contributions[loc.i].Data(), nil
}

// entryLoc places one contribution: the loaded manifest that carries it and
// its index in that manifest's slice. Valid until the next load replaces
// c.by, which is also when reindexHosts rebuilds it.
type entryLoc struct {
	m *mbapi.Manifest
	i int
}

// URICollision is a URI more than one member carries. Host is the member
// whose copy answers every by-URI read (the first in member order); Shadowed
// are the others, in member order.
type URICollision struct {
	URI      string
	Host     string
	Shadowed []string
}

// uriOf resolves an entry's URI inside the manifest that carries it: an entry
// may carry its own provider (a layer provider hosted inside another binary's
// manifest), else the carrying manifest's.
func uriOf(m *mbapi.Manifest, e *contribapi.Contribution) string {
	prov := e.Provider
	if prov == "" {
		prov = m.Provider
	}
	return contribapi.Contribution{Provider: prov, Type: e.Type, ID: e.ID}.ResolvedURI()
}

// reindexHosts rebuilds the two by-URI indexes — URI→carrying provider and
// URI→entry location — in MEMBER ORDER, so a URI two members carry resolves
// to the same host on every load: the first member declaring it wins, the
// others are recorded as a URICollision (blind audit M4: ranging the map made
// the host a coin toss per reload). Caller must hold c.mu for writing.
func (c *CatalogImpl) reindexHosts() {
	h := make(map[string]string)
	loc := make(map[string]entryLoc)
	provs := make(map[string]bool)
	var collisions []URICollision
	at := map[string]int{} // uri → index in collisions
	for _, member := range c.members {
		m := c.by[member.Name]
		if m == nil {
			continue
		}
		provs[m.Provider] = true
		for i := range m.Contributions {
			uri := uriOf(m, &m.Contributions[i])
			if p, err := contribapi.ParseResourceURI(uri); err == nil {
				provs[p.Provider] = true
			}
			if host, taken := h[uri]; taken {
				if host == member.Name {
					continue // a duplicate inside one manifest: the builder's dedupe owns it
				}
				k, seen := at[uri]
				if !seen {
					k = len(collisions)
					at[uri] = k
					collisions = append(collisions, URICollision{URI: uri, Host: host})
				}
				collisions[k].Shadowed = append(collisions[k].Shadowed, member.Name)
				continue
			}
			h[uri] = member.Name
			loc[uri] = entryLoc{m: m, i: i}
		}
	}
	for _, col := range collisions {
		slog.Warn("catalog: one URI carried by several members; the first in member order answers",
			"uri", col.URI, "host", col.Host, "shadowed", col.Shadowed)
	}
	c.hosts = h
	c.entries = loc
	c.providers = provs
	c.collisions = collisions
	c.applyCarriedOverrides()
}

// applyCarriedOverrides applies the override entries the members carry over
// the entries the by-URI index places (applyCarriedOverrideEntries, the one
// function MemoryCatalog shares). Caller must hold c.mu for writing, after the
// indexes are built.
func (c *CatalogImpl) applyCarriedOverrides() {
	ordered := make([]*mbapi.Manifest, 0, len(c.members))
	for _, member := range c.members {
		ordered = append(ordered, c.by[member.Name])
	}
	c.overrideFindings = applyCarriedOverrideEntries(ordered, c.entries, c.kindsLocked)
}

// OverrideFindings are the override entries the last load could not apply,
// each saying why (no member serves the entry it overrides; refused).
func (c *CatalogImpl) OverrideFindings() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]string(nil), c.overrideFindings...)
}

// URICollisions reports every URI more than one member carries, as of the
// last load, in member order of the host. Empty when every URI has one carrier.
func (c *CatalogImpl) URICollisions() []URICollision {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]URICollision(nil), c.collisions...)
}

// Members returns the membership in declared order: each member's name and
// the binary the universe declares for it.
func (c *CatalogImpl) Members() []Member {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Member(nil), c.members...)
}

// UnavailableMembers names the members whose declared binary is not on
// disk: their entries are served from the record, their producer cannot be
// reached. The stat lives here, with the membership that names the binary,
// never on the wire's read path (G20: nothing on the control plane composes
// a path). A member declared without a binary is never unavailable.
func (c *CatalogImpl) UnavailableMembers() []string {
	var out []string
	for _, m := range c.Members() {
		if m.Binary == "" {
			continue
		}
		if _, err := os.Stat(m.Binary); err != nil {
			out = append(out, m.Name)
		}
	}
	return out
}

// HostOf returns the binary whose manifest carries the contribution at uri — the
// one that must be reached to read its content. Falls back to the URI's own
// provider when the catalog does not place it, so an ordinary per-binary read is
// unchanged.
func (c *CatalogImpl) HostOf(uri contribapi.ResourceURI) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if h, ok := c.hosts[uri.String()]; ok {
		return h
	}
	return uri.Provider
}

// SetSchema implements Catalog. Attaches a SchemaService for Phase 2
// reindex. Idempotent — replacing an existing schema is allowed
// (refresh path); the next Reindex picks up the new state.
func (c *CatalogImpl) SetSchema(schema mbapi.CatalogSchemaService) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.schema = schema
	return nil
}

// SetValidationConfig installs the central ContributionsConfig consulted
// during Reindex. Idempotent — safe to call before or after SetSchema;
// the next Reindex picks up the new posture. Wired by the hub bootstrap
// from `application.UseConfig[contribapi.ContributionsConfig]`.
func (c *CatalogImpl) SetValidationConfig(cfg contribapi.ContributionsConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.valCfg = cfg
}

// SetRegistrationOutcomes records what registering each data-type produced.
// Called by the aggregator's Phase 2 BEFORE Reindex; the next Reindex stamps
// the outcomes on the entries as verdicts.
func (c *CatalogImpl) SetRegistrationOutcomes(outcomes map[string]mbapi.RegistrationOutcome) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.regOutcomes = outcomes
}

var _ mbapi.RegistrationOutcomeSetter = (*CatalogImpl)(nil)

// Reindex implements Catalog. Re-runs schema-aware processing over the
// affected Contributions: validates the data layer against its kind's
// data-type and derives edges into the aggregator's index.
//
// changedURIs == nil (or empty) means "reindex everything"; otherwise
// only the listed URIs are reprocessed.
//
// Reindex is a no-op until SetSchema has been called (Phase 1 state per
// the hub-boot-order ADR). Validation + edge-extraction errors are
// logged as warnings but never block the reindex — partial results are
// preferred to a hard fail.
//
// Three phases, and the write lock is held for the first and the last only:
//
//  1. snapshot, under the lock: the kinds, the posture, the schema service and
//     a COPY of every entry to process;
//  2. verdicts, with no lock: every schema-service call — validation, data-type
//     resolution, edge extraction — runs here, so a slow validation never
//     blocks a read (blind audit L6: the lock used to span every call);
//  3. apply, under the lock: each verdict is stamped on the entry it was
//     computed for, provided the manifest that carried it is still the loaded
//     one — a Reload in between swapped the map, and its own reindex follows.
func (c *CatalogImpl) Reindex(ctx context.Context, changedURIs []string) error {
	// --- 1. snapshot ---
	c.mu.Lock()
	if c.schema == nil {
		c.mu.Unlock()
		slog.Debug("catalog.Reindex called before SetSchema; skipping (Phase 1 state)")
		return nil
	}
	schema := c.schema
	valCfg := c.valCfg
	regOutcomes := c.regOutcomes
	// Every kind in the universe, read once. This is the aggregator's
	// advantage and the reason schema resolution belongs here: a binary sees
	// only the kinds it declares, while the aggregator holds the whole graph
	// and can resolve a kind one member declares for content another member
	// published.
	kinds := c.kindsLocked()
	schemaDisagreements := c.schemaDisagreementsLocked(kinds)

	changedSet := map[string]bool{}
	for _, u := range changedURIs {
		changedSet[u] = true
	}
	reindexAll := len(changedSet) == 0

	type work struct {
		member string
		m      *mbapi.Manifest
		i      int
		entry  contribapi.Contribution // a copy: phase 2 reads it with no lock
	}
	var todo []work
	// The formats each member writes (its sidecar-info), read once: a kind
	// whose format is newer than this hub's is read leniently (a newer minor)
	// or refused (a newer major) — contribapi.JudgeFormat.
	formats := map[string]map[string]string{}
	for _, member := range c.members {
		m, ok := c.by[member.Name]
		if !ok {
			continue
		}
		formats[member.Name] = contribapi.MemberFormats(m.Contributions)
		for i := range m.Contributions {
			if !reindexAll && !changedSet[m.Contributions[i].ResolvedURI()] {
				continue
			}
			todo = append(todo, work{member: member.Name, m: m, i: i, entry: m.Contributions[i]})
		}
	}
	c.mu.Unlock()

	// What is wrong with the declarations themselves, once per reindex: an
	// incomplete one, and a kind two members declare (the system one wins,
	// KindsFrom; a member's redeclaration of a system kind is an error).
	// Reported, never refused: a live universe keeps its entries.
	for _, f := range kinds.Check() {
		if f.IsWarning() {
			slog.Warn("catalog.Reindex: kind declaration finding", "rule", f.Rule, "finding", f.String())
		} else {
			slog.Error("catalog.Reindex: kind declaration finding", "rule", f.Rule, "finding", f.String())
		}
	}
	for _, f := range schemaDisagreements {
		slog.Error("catalog.Reindex: kind declaration finding", "rule", contribapi.KindRuleDuplicate, "finding", f)
	}

	// --- 2. verdicts, unlocked ---
	// A data-type is resolved once per reindex, not once per entry that
	// declares it: resolving decodes the whole schema document, and a few
	// data-types serve hundreds of entries.
	dataTypes := newDataTypeMemo(schema)
	host := builtin.Formats()
	verdicts := make([]entryVerdict, len(todo))
	processed, errors := 0, 0
	for k, w := range todo {
		verdicts[k] = verdictFor(ctx, w.member, w.entry, kinds, valCfg, regOutcomes, schema, dataTypes)
		verdicts[k] = judgeFormat(verdicts[k], w.member, w.entry, host, formats[w.member])
		if verdicts[k].errored {
			errors++
		} else {
			processed++
		}
	}

	// --- 3. apply ---
	c.mu.Lock()
	for k, w := range todo {
		if c.by[w.member] != w.m {
			continue // reloaded meanwhile: that load's reindex re-verdicts it
		}
		cn := &w.m.Contributions[w.i]
		v := verdicts[k]
		cn.Valid = v.valid
		cn.ValidationErrors = v.errs
		uri := cn.ResolvedURI()
		if len(v.edges) > 0 {
			c.edges[uri] = v.edges
		} else {
			delete(c.edges, uri)
		}
	}
	c.mu.Unlock()
	slog.Debug("catalog.Reindex complete", "processed", processed, "errors", errors)
	return nil
}

// entryVerdict is what phase 2 of Reindex computes for one entry, applied
// under the lock in phase 3.
type entryVerdict struct {
	valid   *bool
	errs    []string
	edges   []contribapi.ContributionEdge
	errored bool // counted as an error in the reindex summary
}

// judgeFormat applies the format rule to one entry's verdict: an entry of a
// kind its member writes in a newer MAJOR than this hub reads is refused,
// saying to update the hub; in a newer MINOR, a schema finding is the keys
// this hub does not know — the entry stays valid and the finding says so.
// The same or an older format keeps the verdict as it is (strict).
func judgeFormat(v entryVerdict, member string, cn contribapi.Contribution, host, formats map[string]string) entryVerdict {
	h, mine := host[cn.Type], formats[cn.Type]
	switch contribapi.JudgeFormat(h, mine) {
	case contribapi.FormatNewerMajor:
		invalid := false
		return entryVerdict{valid: &invalid, errored: true,
			errs: []string{contribapi.NewerMajorMessage(member, cn.Type, mine, h)}}
	case contribapi.FormatNewerMinor:
		if len(v.errs) == 0 {
			return v
		}
		valid := true
		errs := make([]string, 0, len(v.errs))
		for _, e := range v.errs {
			errs = append(errs, fmt.Sprintf("tolerated (%s writes %s in format %s, newer than this hub's %s): %s",
				member, cn.Type, mine, h, e))
		}
		v.valid, v.errs = &valid, errs
	}
	return v
}

// verdictFor runs the single framework validation step and edge extraction
// for one contribution, with no catalog lock held: everything it needs is
// passed in. The severity is the KIND's, with valCfg as override.
func verdictFor(ctx context.Context, member string, cn contribapi.Contribution, kinds contribapi.Kinds,
	valCfg contribapi.ContributionsConfig, regOutcomes map[string]mbapi.RegistrationOutcome,
	schemaSvc mbapi.CatalogSchemaService, dataTypes *dataTypeMemo) entryVerdict {
	// --- Undeclared kind: the aggregator's refusal ---
	// This is the ONE place that can tell "declared elsewhere" from
	// "declared nowhere": a member's builder keeps entries of a kind it does
	// not declare (Manifest.ForeignKinds), because another member may. Here
	// every member's declarations are in view, so an entry whose kind none
	// declares is marked invalid — the entry, with its reason, never the
	// member's manifest.
	if _, declared := kinds.Get(cn.Type); !declared && !contribapi.IsMetaKind(cn.Type) {
		valid, errs := applyUndeclaredKind(cn, member, valCfg)
		return entryVerdict{valid: valid, errs: errs, errored: true}
	}

	// --- Schema validation (the single ingest checkpoint) ---
	// Runs for EVERY type, gated by the central config severity. A kind that
	// declares no data-type is flagged here too.
	schema := kinds.SchemaFor(cn.Type)
	v := entryVerdict{}
	v.valid, v.errs = validationVerdict(ctx, cn, schema, kinds.SeverityFor(cn.Type, valCfg), regOutcomes, schemaSvc)

	if schema == "" {
		// No data-type → no edge extraction.
		return v
	}
	dt, err := dataTypes.resolve(ctx, schema)
	if err != nil || dt == nil {
		// Data-type unresolvable — emit warning, keep Contribution
		// without edges. NOT an ingest failure.
		slog.Warn("catalog.Reindex: data-type not found",
			"uri", cn.ResolvedURI(), "schema", schema)
		v.errored = true
		return v
	}
	edges, err := ExtractEdges(cn.Data(), dt)
	if err != nil {
		slog.Warn("catalog.Reindex: edge extraction failed",
			"uri", cn.ResolvedURI(), "error", err)
		v.errored = true
		return v
	}
	// Derived, so it lives in the aggregator's index rather than on the
	// contribution: one writer, one home, and nothing shipped over the wire
	// that no consumer reads.
	v.edges = edges
	return v
}

// validationVerdict is the single framework validation step for one
// contribution: Valid + ValidationErrors, as the entry will carry them. The
// severity is the KIND's, with the central config as override — resolved by
// the caller from the registry, so this function has one job.
//
// The BodyValidator is schemaSvc.ValidateBody when a schema service is
// attached; nil otherwise (Phase 1 / no schema) — ValidateContribution
// records the "no schema service" diagnostic under the configured severity
// rather than hard-failing. A skipped check (severity off, no data layer)
// records nothing: Valid nil, no errors — on the wire the same as an entry
// nothing checked (validation.md §2).
func validationVerdict(ctx context.Context, cn contribapi.Contribution, schema, severity string,
	regOutcomes map[string]mbapi.RegistrationOutcome, schemaSvc mbapi.CatalogSchemaService) (*bool, []string) {
	var validate BodyValidator
	if schemaSvc != nil {
		validate = schemaSvc.ValidateBody
	}
	var outcome ValidationOutcome
	if o, ok := regOutcomes[cn.ResolvedURI()]; ok && cn.Type == contribapi.ResourceTypeDataType && len(cn.Data()) == 0 {
		// A data-type IS its DSL: its verdict is whether that parsed at
		// registration, which the body check cannot see (no body beside it).
		outcome = VerdictFromRegistration(severity, o)
	} else {
		outcome = ValidateContribution(ctx, severity, cn, schema, validate)
	}
	if outcome.Skipped {
		return nil, nil
	}
	if len(outcome.Errors) > 0 {
		if outcome.Valid != nil && !*outcome.Valid {
			slog.Warn("catalog.Reindex: contribution failed schema validation (marked invalid)",
				"uri", cn.ResolvedURI(), "schema", schema, "errors", outcome.Errors)
		} else {
			slog.Warn("catalog.Reindex: contribution schema validation warning",
				"uri", cn.ResolvedURI(), "schema", schema, "errors", outcome.Errors)
		}
	}
	return outcome.Valid, outcome.Errors
}

// applyUndeclaredKind is the verdict of an entry whose kind no member of
// the universe declares: invalid, with the reason, kept. The central `off`
// posture still skips it — off means nothing is checked — but no `warn`
// softens it: nothing may publish a contribution of an undeclared kind (goals
// G2), and the member's builder already deferred exactly this question here.
func applyUndeclaredKind(cn contribapi.Contribution, member string, valCfg contribapi.ContributionsConfig) (*bool, []string) {
	if contribapi.NormalizeSeverity(valCfg.Validate) == contribapi.ValidateOff {
		return nil, nil
	}
	invalid := false
	slog.Warn("catalog.Reindex: contribution of an undeclared kind (marked invalid)",
		"uri", cn.ResolvedURI(), "kind", cn.Type, "member", member)
	return &invalid, []string{fmt.Sprintf(
		"kind %q is declared by no member of this universe (published by %s): add a `%s` declaration to the member that owns the kind",
		cn.Type, member, contribapi.ResourceTypeContributionType)}
}

// dataTypeMemo resolves each data-type once for the length of one Reindex.
// Not safe for concurrent use: Reindex computes its verdicts in one loop.
type dataTypeMemo struct {
	svc  mbapi.CatalogSchemaService
	seen map[string]dataTypeResult
}

type dataTypeResult struct {
	t   *schemaapi.Type
	err error
}

func newDataTypeMemo(svc mbapi.CatalogSchemaService) *dataTypeMemo {
	return &dataTypeMemo{svc: svc, seen: map[string]dataTypeResult{}}
}

// resolve answers resolveDataType's result for schemaURI, computed on its first
// request. ExtractEdges only reads the type, so one is shared by every entry.
func (m *dataTypeMemo) resolve(ctx context.Context, schemaURI string) (*schemaapi.Type, error) {
	if r, ok := m.seen[schemaURI]; ok {
		return r.t, r.err
	}
	t, err := resolveDataType(ctx, m.svc, schemaURI)
	m.seen[schemaURI] = dataTypeResult{t: t, err: err}
	return t, err
}

// resolveDataType fetches a parsed data-type Schema AST from the schema
// service via its GetDataTypeByURI wrapper. The wrapper returns the raw JSON
// bytes of a schema.Schema; we unmarshal to the schema.Type root for
// ExtractEdges. No catalog lock is needed: the service is passed in.
func resolveDataType(ctx context.Context, schemaSvc mbapi.CatalogSchemaService, schemaURI string) (*schemaapi.Type, error) {
	if schemaSvc == nil {
		return nil, fmt.Errorf("no schema service attached")
	}
	raw, err := schemaSvc.GetDataTypeByURI(ctx, schemaURI)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var schemaDoc schemaapi.Schema
	if err := json.Unmarshal(raw, &schemaDoc); err != nil {
		return nil, fmt.Errorf("decode data-type %q: %w", schemaURI, err)
	}
	if schemaDoc.Root != "" {
		if t, ok := schemaDoc.Types[schemaDoc.Root]; ok {
			return t, nil
		}
	}
	// Fall back: pick any single top-level type. (Many data-types have a
	// single root and no explicit Root field.)
	for _, t := range schemaDoc.Types {
		return t, nil
	}
	return nil, nil
}

// MarkReady implements Catalog. Flips the ready flag and emits a
// coarse `contributions-changed` event (KindsChanged=["*"], Provider="")
// so subscribers (agent prompt builder, GUI catalog, MCP bridge) refresh
// once the catalog is first fully indexed.
func (c *CatalogImpl) MarkReady(ctx context.Context) error {
	c.mu.Lock()
	c.ready = true
	pub := c.eventPub
	members := len(c.members)
	loaded := len(c.by)
	c.mu.Unlock()
	slog.Info("catalog ready", "members", members, "loaded", loaded)
	c.emitChanged(pub, "", []string{"*"})
	return nil
}

// SetEventPublisher attaches a ContributionsEventPublisher. Idempotent —
// safe to call before or after MarkReady; subsequent Reload/MarkReady
// calls will emit through the new publisher. Pass nil to detach.
//
// Wired by ktw-hub's bootstrap to a NATS-backed publisher; other
// binaries (ktw-agents, ktw-gui) attach subscribers via NATS Subscribe,
// not via this method.
func (c *CatalogImpl) SetEventPublisher(pub contribapi.ContributionsEventPublisher) {
	c.mu.Lock()
	c.eventPub = pub
	c.mu.Unlock()
}

// emitChanged publishes a `contributions-changed` event via the attached
// publisher. Best-effort: a publisher failure is logged but never bubbles
// up — a broken bus must NOT block catalog operations. Pub is passed as
// a parameter (not re-read from c.eventPub) so callers can snapshot it
// while holding the mutex and emit safely after release.
func (c *CatalogImpl) emitChanged(pub contribapi.ContributionsEventPublisher, provider string, kinds []string) {
	if pub == nil {
		return
	}
	ev := contribapi.ContributionsChangedEvent{
		Provider:     provider,
		KindsChanged: kinds,
		ChangedAt:    time.Now().UTC(),
	}
	if err := pub.PublishContributionsChanged(ev); err != nil {
		slog.Warn("catalog: publish contributions-changed failed", "error", err, "provider", provider, "kinds", kinds)
	}
}

// Ready implements Catalog. Returns true after MarkReady.
func (c *CatalogImpl) Ready() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// Compile-time interface assertion.
var _ mbapi.Catalog = (*CatalogImpl)(nil)

// ContributionAt resolves one contribution by URI without materialising its
// kind: the host index names the one manifest that carries it, and only that
// manifest is scanned. Satisfies mbapi.ContributionResolver.
func (c *CatalogImpl) ContributionAt(uri string) (contribapi.Contribution, bool) {
	parsed, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return contribapi.Contribution{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	loc, ok := c.entries[parsed.String()]
	if !ok {
		return contribapi.Contribution{}, false
	}
	return loc.m.Contributions[loc.i], true
}

// EdgesOf returns the outgoing references derived for one contribution.
//
// Derived state, so it is read from the aggregator rather than carried on the
// contribution: the catalog is its only writer, and a consumer that wants it
// asks for it instead of every consumer receiving it whether or not they do.
func (c *CatalogImpl) EdgesOf(uri string) []contribapi.ContributionEdge {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.edges[uri]
}

// EdgesTo answers the reverse question — what references `uri` — by scanning
// the index. `relation` narrows to one edge label; empty means every label.
//
// Scans rather than maintaining a reverse map: the forward index is small
// (edges per contribution, not per byte of content) and a second map would be
// a second thing to keep correct for a question nothing asks on a hot path.
func (c *CatalogImpl) EdgesTo(uri, relation string) []contribapi.ContributionEdge {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []contribapi.ContributionEdge
	for source, edges := range c.edges {
		for _, e := range edges {
			if e.Target != uri || (relation != "" && e.Relation != relation) {
				continue
			}
			tagged := e
			tagged.Source = source + "#" + e.Source
			out = append(out, tagged)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out
}

// OpenLayer serves a layer the aggregator HOLDS — resident in tier 1 — and
// refuses the rest, through contribapi.OpenLayerFrom with no lanes: a layer
// not resident here is contribapi.ErrLayerNotCarried, a bulk one
// contribapi.ErrBulkLayer. Reaching a producer is the CatalogService's job (it
// has the connection manager); this catalog contacts nobody, which is what
// makes its answers safe to give during discovery.
func (c *CatalogImpl) OpenLayer(_ context.Context, uri, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	entry, found := c.ContributionAt(uri)
	if !found {
		return nil, contribapi.LayerRef{}, notInCatalog(uri)
	}
	if layer == "" {
		c.mu.RLock()
		kinds := c.kindsLocked()
		c.mu.RUnlock()
		layer = kinds.DefaultLayer(entry.Type)
	}
	return contribapi.OpenLayerFrom(entry, layer, contribapi.LayerLanes{})
}

// notInCatalog is the universe views' not-found: the catalog's own sentinel
// (callers test for it) wrapping the one not-found every reach answers,
// contribapi.ErrContributionNotFound.
func notInCatalog(uri string) error {
	return fmt.Errorf("%w: %w", mbapi.ErrSidecarNotInCatalog, contribapi.ErrContributionNotFound{URI: uri})
}

var _ mbapi.LayerOpener = (*CatalogImpl)(nil)

// kindsLocked builds the kind registry from every declaration the catalog
// holds — the aggregator sees the whole universe, so a kind one member declares
// resolves for content another member published.
//
// Built per reindex rather than cached: a reindex is exactly the moment the set
// of kinds may have changed, and a cache that had to be invalidated by whoever
// added a kind would be one more thing to get wrong.
//
// Caller must hold c.mu.
func (c *CatalogImpl) kindsLocked() contribapi.Kinds {
	if c.kindsCached {
		return c.kinds
	}
	var all []contribapi.Contribution
	for _, member := range c.members {
		name := member.Name
		if m, ok := c.by[name]; ok && m != nil {
			all = append(all, m.Contributions...)
		}
	}
	c.kinds = contribapi.KindsFrom(all)
	c.kindsCached = true
	c.kindsBuilds++
	return c.kinds
}

// schemaDisagreementsLocked compares the schemas of a kind several members
// declare alike: each binary carrying the pack that owns the kind declares it
// under its own provider and ships the schema it names, so one kind holds
// only while those data-types are the same document. A data-type whose
// source digest differs from the resolved declarer's (binaries of different
// versions) is one finding naming both. A schema this view does not carry is
// not compared: the census refuses a declared schema that does not ship.
func (c *CatalogImpl) schemaDisagreementsLocked(kinds contribapi.Kinds) []string {
	var digests map[string]string
	var out []string
	for _, name := range kinds.Names() {
		declared := kinds.AgreeingSchemas(name)
		if declared == nil {
			continue
		}
		if digests == nil {
			digests = map[string]string{}
			for _, member := range c.members {
				m, ok := c.by[member.Name]
				if !ok || m == nil {
					continue
				}
				for i := range m.Contributions {
					if e := &m.Contributions[i]; e.Type == contribapi.ResourceTypeDataType {
						digests[e.ResolvedURI()] = e.Layers[contribapi.LayerSource].Hash
					}
				}
			}
		}
		want := digests[declared[0].Schema]
		for _, d := range declared[1:] {
			if got := digests[d.Schema]; want != "" && got != "" && got != want {
				out = append(out, fmt.Sprintf("kind %q: %s (declared by %s) and %s (declared by %s) differ; one kind, one schema - the binaries carry different versions of its pack",
					name, declared[0].Schema, declared[0].Declarer, d.Schema, d.Declarer))
			}
		}
	}
	return out
}

// Kinds implements mbapi.KindsHolder: the registry the aggregate carries,
// built once per reload and answered from memory after that (re-audit N14:
// every OpenLayer with an empty layer used to rebuild it).
func (c *CatalogImpl) Kinds() contribapi.Kinds {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.kindsLocked()
}
