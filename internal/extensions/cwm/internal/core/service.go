// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/santhosh-tekuri/jsonschema/v5"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// Service is the L4 CWM service. Composes every cwm/internal/*
// sub-service into one struct that satisfies api.CWMService.
//
// The struct fields are exported so callers (typically the
// cwm/local Extension) can override individual sub-services
// for testing. In production every field is owned by Service
// and shouldn't be swapped after New.
type Service struct {
	Config *CWMServiceConfig

	// steps are the migration steps this binary ships (migrate.go).
	steps []MigrationStep
	// contributionKinds are the kinds a project-level step reads the
	// project's files against; nil: the built-in kinds.
	contributionKinds func() (contribapi.Kinds, error)

	// dataVersion caches the served project's data version for the legacy
	// reads of an unconverted repository (migrate_legacy.go); 0 until read,
	// reset by a migration.
	dataVersion atomic.Int64

	// fns are the expression functions the capabilities declare (functions.go):
	// swapped whole when a refresh brings new capabilities.
	fns atomic.Pointer[functionRegistry]

	// decls is the declaration snapshot a refresh swapped in (decl()); nil until
	// the first refresh, when the construction-time set is the one in force.
	decls     atomic.Pointer[CatalogTypeSet]
	refreshMu sync.Mutex

	// observers are the attached in-process event observers (AttachObservers),
	// drained by Drain and stopped by Close.
	observers []*events.Observer
	// member marks a store served beside others in a store set (ServeAsMember):
	// its observers' context names its project key (eventProject).
	member    atomic.Bool
	memberKey atomic.Value // string: the project key, once read

	// beforeClose run first in Close (BeforeClose): the work the service's
	// builder started in the background is waited out before the workspace
	// is released.
	beforeCloseMu sync.Mutex
	beforeClose   []func()

	Storage *storage.Storage
	// locks serialise the writes of one object (write_locks.go).
	locks  writeLocks
	Events *events.Service

	// hookJournal is the on_* calls owed for writes (hooks_journal.go), loaded
	// on first use; hookRetry overrides the retry policy (tests).
	hookJournal     *hookJournal
	hookJournalOnce sync.Once
	hookRetry       hookRetryPolicy

	// matchStore is the async subscriptions' matches not yet taken
	// (matches.go); takers the MatchTakers attached (AttachMatchTakers).
	// outboxStore is what reactions' writes staged (outbox.go).
	matchStore     *matchStore
	matchStoreOnce sync.Once
	takersMu       sync.RWMutex
	takers         []api.MatchTaker
	outboxStore    *outboxStore
	outboxOnce     sync.Once
	// changeSetStore is the atomic reactions' open change sets (changeset.go);
	// changeSetStopAfterLog stops a commit right after its redo log (tests: a
	// crash in the middle of a commit).
	changeSetStore        *changeSetStore
	changeSetOnce         sync.Once
	changeSetStopAfterLog func() bool
	// moduleSubs is the cross-module subscriptions gathered from the module
	// objects, cached per index generation (module_subscriptions.go).
	moduleSubsMu sync.Mutex
	moduleSubs   *moduleSubscriptions

	// objIndex is the rebuildable read model over the object files. Derived
	// state — built lazily on first read, kept current by the write verbs.
	objIndex     *objectIndex
	objIndexOnce sync.Once
	// endedIDs is the archived ids, read lazily per module from the history
	// folders (ended.go).
	endedIDs  *endedSet
	endedOnce sync.Once

	// objTypes is the declared object-type set (D1), keyed by name. It lives
	// here rather than being read off Config on every write because
	// RefreshCatalogTypes may add to it on a RUNNING service — an unguarded
	// slice on Config would be a data race between a hot-deploy and a create.
	objTypes   map[string]api.ObjectType
	objTypesMu sync.RWMutex
	// typeRejections are the object types read and not registered, by name —
	// declaration findings (DeclarationFindings). Guarded by objTypesMu.
	typeRejections map[string]api.DeclarationFinding

	// capSchemas are the compiled capability schemas by data-type URI
	// (capability_validate.go); add-only, like objTypes.
	capSchemas   map[string]*jsonschema.Schema
	capSchemaRaw map[string]json.RawMessage // the compiled JSON Schema, for its defaults
	capSchemasMu sync.RWMutex
	// unresolvedSchemas remembers the schema URIs already reported missing, so
	// the warning is logged once, not on every write.
	unresolvedSchemas sync.Map

	// problems are the objects' errors as read (problems.go).
	problems problemStore

	// numberBlocks is the minter and its repository (number_blocks.go).
	numberBlocks numberBlocks

	// refused is the refusal of the project this service was opened on
	// (wraps api.ErrProjectRefused); nil when it opens. The index
	// answers it to every read (ensureLoaded), the object paths to every
	// write, and the bootstrap point does not run.
	refused error
}

// New constructs a Service: storage (the lock and the type registry), the
// source service and the events, in dependency order. What opening a project
// creates is the `bootstrap` hook point's, run once the service is up.
func New(ctx stdctx.Context, cfg *CWMServiceConfig) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	// A project this binary cannot serve is refused at opening, not here: the
	// service starts (the sidecar answers, the GUI reads the reason) and every
	// read and write of the project answers the refusal (refused). Data newer
	// than this binary knows comes first - reading it would misread it,
	// writing it would corrupt it, and this binary's type checks
	// may not even read it; then a type needing a solution this universe
	// lacks.
	refused := refuseNewerData(cfg.ObjectsRoot(), migrationSteps)
	if refused == nil {
		refused = refuseMissingSolution(cfg.ObjectsRoot(), cfg.CatalogTypes.Objects, cfg.UniverseSolution, cfg.UniverseID)
	}
	// An aspect naming an undeclared capability is refused AT LOAD (7g).
	cfg.CatalogTypes.Aspects = dropAspectsNamingUndeclaredCapabilities(cfg.CatalogTypes.Aspects, cfg.CatalogTypes.Capabilities)

	// 1. Storage acquires the multi-process lockfile + loads the
	//    in-binary FLOOR: TypeSources (this binary's compiled-in type
	//    YAMLs) + the workspace's `types/` overrides. The store ALWAYS
	//    opens on the floor, even with the catalog/hub unavailable.
	s, err := storage.NewWithTypes(ctx, cfg.WorkspaceRoot, cfg.TypeSources...)
	if err != nil {
		return nil, err
	}

	// 1b. Overlay CWM types contributed by OTHER sidecars, resolved from
	//     the aggregate catalog at construction time (cwmlocal fills
	//     cfg.CatalogTypes). FLOOR-WINS: a structural floor type is never
	//     shadowed by a remote contribution. This is what makes "any sidecar
	//     can contribute CWM types" real — the registry consumes them like
	//     skills/tasks. Pinned here, BEFORE any write is accepted. Empty
	//     set = degraded (floor only), already logged by cwmlocal.
	s.Types().Merge(cfg.CatalogTypes.Edges)
	// A project's override entry of a floor type is the exception: the
	// catalog's declaration, carrying it, replaces the floor's.
	s.Types().Override(cfg.CatalogTypes.overridingEdges())

	// Events: identify the workspace in the NATS subject scheme.
	// Defaults match the convention in modules/bus/nats_subjects_list.md:
	// tenant="local", universe="default", workspace = the project's id
	// (api.ProjectWorkspaceToken, "unbootstrapped" before it has one), so two
	// repositories' modules of the same name never share a subject.
	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = "local"
	}
	universeID := cfg.UniverseID
	if universeID == "" {
		universeID = "default"
	}
	workspaceID := cfg.WorkspaceID
	if workspaceID == "" {
		workspaceID = api.ProjectWorkspaceToken(cfg.ObjectsRoot())
	}
	eventsSvc := events.New(tenantID, universeID, workspaceID, cfg.EventBus, cfg.MessageBus)

	svc := &Service{
		Config:  cfg,
		Storage: s,
		Events:  eventsSvc,
		steps:   migrationSteps,
		refused: refused,
	}

	// 2. Pin the declared object-types before any write is accepted. Same
	//    reasoning as the type-registry merge at step 1b: validation that
	//    arrives after the first write is not validation.
	//    The schemas first: a type's sections are checked against them.
	svc.mergeCapabilitySchemas(cfg.CatalogTypes.CapabilitySchemas)
	svc.mergeObjectTypes(cfg.CatalogTypes.Objects, cfg.CatalogTypes.Capabilities)
	fns, _ := buildFunctions(cfg.CatalogTypes.Capabilities)
	svc.fns.Store(fns)

	return svc, nil
}

// CatalogTypeRefresher is implemented by *Service. It puts the declarations
// resolved from the aggregate catalog in force on the live service, without a
// process restart — the CWM consumer's reaction to a contributions-changed
// event. cwmlocal type-asserts the published CWMService to this interface.
type CatalogTypeRefresher interface {
	RefreshCatalogTypes(cts CatalogTypeSet)
}

// RefreshCatalogTypes puts a catalog snapshot in force, replaced whole and
// keyed by each declaration's URI: a new declaration is added, a
// changed one replaces the one in force, one its member's manifest no longer
// names goes. The last good declaration stays while its member is absent from
// the snapshot, or while its new version is refused (it does not decode, its
// name clashes, its type is rejected). A declaration with no source (one not
// read from a catalog) is replaced by name and never dropped. Safe on a
// running service: every set is swapped as one snapshot, so a reader never
// sees a type whose capability is not there yet; a write validates against
// the snapshot in force when it starts.
func (s *Service) RefreshCatalogTypes(cts CatalogTypeSet) {
	if s.Storage == nil {
		return
	}
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	cur := s.decl()
	next := *cur
	next.Sources = make(map[string]api.DeclarationSource, len(cts.Sources))
	for k, v := range cts.Sources {
		next.Sources[k] = v
	}
	stays := func(kind, name string) bool {
		k := api.DeclarationKey(kind, name)
		src, ok := cur.Sources[k]
		if !ok || src.URI == "" {
			return true // not read from a catalog: never dropped
		}
		if cts.Members[src.Provider] && !cts.Named[src.URI] {
			return false // its member no longer names it
		}
		next.Sources[k] = src
		return true // its member is absent, or its new version was refused
	}
	next.Capabilities = replaceByName(cur.Capabilities, cts.Capabilities, func(c api.Capability) string { return c.Name }, "capability", stays)
	next.Edges = replaceByName(cur.Edges, cts.Edges, func(e api.EdgeTypeRegistration) string { return string(e.Name) }, "edge-type", stays)
	s.Storage.Types().Replace(next.Edges)
	s.Storage.Types().Override(cts.overridingEdges())
	next.CapabilitySchemas = make(map[string]json.RawMessage, len(cur.CapabilitySchemas)+len(cts.CapabilitySchemas))
	for k, v := range cur.CapabilitySchemas {
		next.CapabilitySchemas[k] = v
	}
	for k, v := range cts.CapabilitySchemas {
		next.CapabilitySchemas[k] = v // the fresh one wins
	}
	// The schemas, then the capabilities: a type's sections are checked against them.
	s.mergeCapabilitySchemas(next.CapabilitySchemas)
	next.Objects = s.replaceObjectTypes(
		replaceByName(cur.Objects, cts.Objects, func(t api.ObjectType) string { return t.Name }, "object-type", stays),
		next.Capabilities)
	next.Aspects = dropAspectsNamingUndeclaredCapabilities(
		replaceByName(cur.Aspects, cts.Aspects, func(a api.Aspect) string { return a.Name }, "aspect", stays), next.Capabilities)
	next.Workflows = replaceByName(cur.Workflows, cts.Workflows, func(w api.Workflow) string { return w.Name }, "workflow", stays)
	next.Policies = replaceByName(cur.Policies, cts.Policies, func(p api.Policy) string { return p.Name }, "policy", stays)
	next.HookTypes = replaceByName(cur.HookTypes, cts.HookTypes, func(h api.HookType) string { return string(h.Name) }, "hook-type", stays)
	next.Hooks = replaceByName(cur.Hooks, cts.Hooks, func(h api.Hook) string { return h.Source() }, "hook", stays)
	next.EventTypes = replaceByName(cur.EventTypes, cts.EventTypes, func(e api.EventType) string { return e.Name }, "event-type", stays)
	// The universe's grants and what reading the catalog found are a reading
	// of the whole: the latest one stands.
	next.Grants = cts.Grants
	next.Findings = cts.Findings
	next.Members, next.Named = cts.Members, cts.Named
	fns, _ := buildFunctions(next.Capabilities)
	s.fns.Store(fns)
	s.decls.Store(&next)
	// New declarations can clear (or reveal) problems everywhere.
	s.index().markAllStale()
}

// decl is the declaration set in force: the last refresh's snapshot, else the
// one the service was built with.
func (s *Service) decl() *CatalogTypeSet {
	if d := s.decls.Load(); d != nil {
		return d
	}
	if s.Config == nil { // asked before the service is configured (a service built early): nothing declared yet
		return &CatalogTypeSet{}
	}
	return &s.Config.CatalogTypes
}

// replaceByName is the declarations of one kind after a refresh: the fresh
// ones (the first of a name), then those in force the fresh set does not name
// and stays keeps. cur is never modified.
func replaceByName[T any](cur, fresh []T, key func(T) string, kind string, stays func(kind, name string) bool) []T {
	named := make(map[string]bool, len(fresh))
	out := make([]T, 0, len(fresh)+len(cur))
	for _, f := range fresh {
		if k := key(f); k != "" && !named[k] {
			named[k] = true
			out = append(out, f)
		}
	}
	for _, c := range cur {
		if k := key(c); k != "" && !named[k] && stays(kind, k) {
			named[k] = true
			out = append(out, c)
		}
	}
	return out
}

// replaceObjectTypes puts types in force as the registered object types,
// whole: each is checked as mergeObjectTypes does; a refused new version
// leaves the one in force (the last good declaration); a type types does not
// name goes. It answers the types in force, the last good ones in place of
// those refused.
func (s *Service) replaceObjectTypes(types []api.ObjectType, caps []api.Capability) []api.ObjectType {
	s.objTypesMu.Lock()
	defer s.objTypesMu.Unlock()
	next := map[string]api.ObjectType{}
	s.registerObjectTypes(next, types, caps)
	out := make([]api.ObjectType, 0, len(types))
	for _, t := range types {
		if _, ok := next[t.Name]; !ok {
			old, had := s.objTypes[t.Name]
			if !had {
				continue
			}
			next[t.Name] = old
			t = old
		}
		out = append(out, t)
	}
	s.objTypes = next
	return out
}

// mergeObjectTypes registers the object-type declarations the service is
// built with (a refresh replaces them whole: replaceObjectTypes).
//
// A type whose sections do not check against the capabilities declared is
// refused in error mode (type_section_validate.go): it is not registered, so
// the next refresh asks again.
func (s *Service) mergeObjectTypes(types []api.ObjectType, caps []api.Capability) {
	if len(types) == 0 {
		return
	}
	s.objTypesMu.Lock()
	defer s.objTypesMu.Unlock()
	if s.objTypes == nil {
		s.objTypes = map[string]api.ObjectType{}
	}
	s.registerObjectTypes(s.objTypes, types, caps)
}

// registerObjectTypes checks each of types and registers in dst those that
// pass; the caller holds objTypesMu for writing.
func (s *Service) registerObjectTypes(dst map[string]api.ObjectType, types []api.ObjectType, caps []api.Capability) {
	for _, t := range types {
		if err := t.ValidateFolder(); err != nil {
			// REFUSED AT DECLARATION. A type whose storage folder would never be
			// scanned writes objects that are invisible from the moment they are
			// created — the worst failure mode available here, so it is caught
			// when the type is declared rather than when an object goes missing.
			slog.Error("cwm: object-type rejected — unscannable storage folder", "type", t.Name, "error", err)
			s.recordRejection(t, "rejected — its storage folder would never be scanned: "+err.Error())
			continue
		}
		if err := t.ValidateDisplay(); err != nil {
			// A colour or a child order a renderer cannot draw: refused at
			// declaration, like an unscannable folder.
			slog.Error("cwm: object-type rejected — its display facts cannot be drawn", "type", t.Name, "error", err)
			s.recordRejection(t, "rejected — "+err.Error())
			continue
		}
		if t.Name == "" {
			continue
		}
		if problems := s.sectionProblems(t, caps); len(problems) > 0 {
			switch s.declarationMode() {
			case api.TypeValidationError:
				slog.Error("cwm: object-type rejected — its sections do not check against their capabilities",
					"type", t.Name, "problems", strings.Join(problems, " | "))
				s.recordRejection(t, "rejected — its sections do not check against their capabilities: "+strings.Join(problems, " | "))
				continue
			case api.TypeValidationWarn:
				slog.Warn("cwm: object-type sections do not check against their capabilities",
					"type", t.Name, "problems", strings.Join(problems, " | "),
					"hint", "this type is refused once validation runs in error mode")
			}
		}
		if _, exists := dst[t.Name]; exists {
			// A DUPLICATE NAME IS NEVER SILENT. A catalog's clash is refused
			// with a finding when it is read (api.FindingNameClash); one in a
			// set given by hand is logged here.
			slog.Warn("cwm: duplicate object-type declaration ignored",
				"type", t.Name, "hint", "two providers declare this type; the first resolved wins")
			continue
		}
		dst[t.Name] = t
	}
}

// AttachObservers wires every in-process event observer (api.EventObserver):
// each receives, in order and without loss, the events its pattern matches,
// on its own goroutine, with ctx (the application's) for resolving services.
// It hears this store's events only - several stores share the in-process bus
// - and, for a store served beside others (ServeAsMember), ctx
// names the store's project key (api.EventProject). A handler that panics is
// logged and the observer keeps running. Close stops them.
func (s *Service) AttachObservers(ctx stdctx.Context, observers []api.EventObserver) {
	for _, o := range observers {
		o := o
		if o.Handle == nil {
			continue
		}
		obs := s.Events.ObserveOwn(o.TopicPattern, func(ev api.EventEnvelope) {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("cwm: event observer panicked", "observer", o.Name, "topic", ev.Topic, "panic", r)
				}
			}()
			hctx := ctx
			if key := s.eventProject(); key != "" {
				hctx = api.WithEventProject(ctx, key)
			}
			o.Handle(hctx, ev)
		})
		s.observers = append(s.observers, obs)
	}
}

// ServeAsMember marks the store as one of several served by a store set: its
// observers' context names its project key from now on. A store served alone
// leaves it unnamed (api.EventProject is "").
func (s *Service) ServeAsMember() { s.member.Store(true) }

// eventProject is the project key an observer's context names: "" for a store
// served alone; for a member, its project key - read from its project object
// until there is one (a member minting its project object while it opens),
// then kept.
func (s *Service) eventProject() string {
	if !s.member.Load() {
		return ""
	}
	if k, _ := s.memberKey.Load().(string); k != "" {
		return k
	}
	k := s.ProjectKey()
	if k != "" {
		s.memberKey.Store(k)
	}
	return k
}

// Drain lets the event observers finish what was already published, while
// the workspace and the application's services are still open: every
// observer stops taking new events at once, then each handles its queue (a
// one-shot command's own write gets journaled before the process closes).
// Bounded per observer by the observer stop wait. The application container
// calls it (services.Drainer) before it closes anything.
func (s *Service) Drain(ctx stdctx.Context) {
	for _, o := range s.observers {
		o.EndIntake()
	}
	for _, o := range s.observers {
		o.Drain(ctx)
	}
}

// BeforeClose has Close run fn first: what the service's builder started in
// the background (its bootstrap after boot) ends before the observers stop
// and the workspace is released - never a write into a released workspace.
func (s *Service) BeforeClose(fn func()) {
	s.beforeCloseMu.Lock()
	s.beforeClose = append(s.beforeClose, fn)
	s.beforeCloseMu.Unlock()
}

// Close stops the event observers and releases the workspace lockfile.
// Always call this from the application shutdown path; a hot-unloaded
// service leaves the lockfile in place.
func (s *Service) Close() error {
	s.beforeCloseMu.Lock()
	waits := s.beforeClose
	s.beforeClose = nil
	s.beforeCloseMu.Unlock()
	for _, wait := range waits {
		wait()
	}
	if s.objIndex != nil {
		s.objIndex.flushPending() // the verbs' writes reach the catch-up cursor
	}
	for _, o := range s.observers {
		o.Stop()
	}
	s.observers = nil
	if s.Storage != nil {
		return s.Storage.Close()
	}
	return nil
}

// --- small helpers ---------------------------------------------------------

// wrapErr formats `sentinel: detail` while preserving errors.Is
// behavior. Used by every stub + a few real methods that need
// extra detail beyond the typed sentinel.
func wrapErr(sentinel error, detail string) error {
	if detail == "" {
		return sentinel
	}
	return fmt.Errorf("%w: %s", sentinel, detail)
}
