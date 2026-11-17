// SPDX-License-Identifier: Apache-2.0

// Package api defines the public interface and DTOs for the Contexia
// Workspace Manager (CWM) — the generic substrate that stores objects: one
// file per object under its module folder, an index rebuilt from those files,
// typed edges between them.
//
// This package imports only internal/core packages and the manifest api,
// never CWM's implementation. Consumers (sidecars, CLI, tests, extensions
// that contribute capabilities) import it freely.
//
// Layered architecture:
//
//	api/           ← THIS PACKAGE: types, interface, ServiceDef
//	local/         ← Extension() wiring (publish, KTSP, agent tools)
//	internal/...   ← service implementation (core, storage, events, concurrency)
//
// Extensions consume CWM through this interface and publish their own KTSP
// services under their own slots (comments, gates, object-pane, …).
//
// # Interface organization
//
// CWMService is FLAT: methods named <Slot><Verb> (e.g. ObjectCreate,
// EdgeList). The meta.ServiceDef.Methods keys are those Go names; the CLI
// names each command after its kebab form (object-create, edge-list).
//
// # The events.Subscribe exception
//
// All methods follow the typed-Input → typed-Output convention,
// EXCEPT EventsSubscribe, which returns (channel, cancel, error) for
// streaming. It is hidden from auto-CLI via MethodDef.Hidden=true and is
// in-process only: the remote client refuses it (NotOverTheBus).
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// CWMService is the language-neutral, method-neutral CWM contract.
//
// All methods follow the context-first signature with typed
// Input/Output, except EventsSubscribe (channel return) — see
// package doc.
type CWMService interface {

	// object slot — the authored side of the substrate.
	//
	// One object = one file under its module folder, in its type's `folder:`
	// (default objects/), committed with the code. ObjectGet returns the
	// object's revision; ObjectUpdate given that revision refuses a newer
	// stored one with ErrConflict, and given none writes unconditionally.
	ObjectCreate(ctx context.Context, in ObjectCreateInput) (ObjectCreateOutput, error)
	ObjectGet(ctx context.Context, in ObjectGetInput) (ObjectGetOutput, error)
	ObjectUpdate(ctx context.Context, in ObjectUpdateInput) (ObjectUpdateOutput, error)
	ObjectDelete(ctx context.Context, in ObjectDeleteInput) (ObjectDeleteOutput, error)
	// ObjectArchive ends an object: its file is renamed, bytes unchanged, into
	// its module's `.ktw/_history/<yyyy-mm>/` and leaves the index; every link
	// to it stays valid as an ended link (never dangling, accepted on write),
	// and its id is never minted again. One `archived` event; idempotent.
	ObjectArchive(ctx context.Context, in ObjectArchiveInput) (ObjectArchiveOutput, error)

	// Object reads — answered from the rebuildable index, never by rescanning
	// the object files per call (goals.md G5).
	ObjectList(ctx context.Context, in ObjectListInput) (ObjectListOutput, error)
	ObjectTree(ctx context.Context, in ObjectTreeInput) (ObjectTreeOutput, error)
	// ObjectSearch answers from resident metadata — id, name, title, description,
	// tags, type, module, effective capabilities — with zero file reads (7b).
	ObjectSearch(ctx context.Context, in ObjectSearchInput) (ObjectSearchOutput, error)
	// ObjectScan rebuilds the index and reports added/modified/removed — the
	// ObserveActual L0 change-detection basis (G4).
	ObjectScan(ctx context.Context, in ObjectScanInput) (ObjectScanOutput, error)
	// CatchUpRelease publishes the per-object events a catch-up of more than
	// CatchUpPerObjectMax objects held (catch_up.go): the answer to its card.
	CatchUpRelease(ctx context.Context, in CatchUpReleaseInput) (CatchUpReleaseOutput, error)
	// ObjectErrorList lists every object carrying errors (object_errors.go),
	// and the files CWM could not read — the global Errors panel.
	ObjectErrorList(ctx context.Context, in ObjectErrorListInput) (ObjectErrorListOutput, error)
	// ProjectInit mints the project id and writes the root project object
	// (identity-and-index.md §7). Refuses to re-mint: the id is minted once.
	ProjectInit(ctx context.Context, in ProjectInitInput) (ProjectInitOutput, error)
	// ProjectsKnown lists the projects this universe knows - the one it
	// serves and those in the installation's projects folder - each with its
	// type; a capability keeps those whose type carries it.
	ProjectsKnown(ctx context.Context, in ProjectsKnownInput) (ProjectsKnownOutput, error)
	// Migrate brings the project's data to this binary's data version through
	// the ordered migration steps; DryRun lists what it would change.
	Migrate(ctx context.Context, in MigrateInput) (MigrateOutput, error)
	// MigrateCommit converts the project's data as Migrate does and commits
	// the files it wrote as one commit, each step reported into its panel;
	// refused, writing nothing, when nothing waits or a file it writes is not
	// as the last commit has it.
	MigrateCommit(ctx context.Context, in MigrateCommitInput) (MigrateCommitOutput, error)
	// ObjectResolve turns a reference — a full name from a log, or a bare id —
	// into the object's file in THIS project (identity-and-index.md §1).
	ObjectResolve(ctx context.Context, in ObjectResolveInput) (ObjectResolveOutput, error)

	// ObjectEffectiveCapabilities is the set of capabilities in force on an
	// object — its type's, its aspects', its own groups, and those that attach
	// themselves to its type — each with its source. A policy is never one.
	ObjectEffectiveCapabilities(ctx context.Context, in ObjectEffectiveCapabilitiesInput) (ObjectEffectiveCapabilitiesOutput, error)
	// ObjectEffectiveSurface is what the type and the capabilities in force
	// contribute to an object — actions, badges, hooks, agent hints — composed,
	// each with where it came from. Read-only.
	ObjectEffectiveSurface(ctx context.Context, in ObjectEffectiveSurfaceInput) (ObjectEffectiveSurfaceOutput, error)

	// The DETAIL SURFACE (the GUI port): one type namespace, so a type lookup
	// is one lookup and "which types may live here" is one create picker.
	ObjectTypeGet(ctx context.Context, in ObjectTypeGetInput) (ObjectTypeGetOutput, error)
	ObjectTypeList(ctx context.Context, in ObjectTypeListInput) (ObjectTypeListOutput, error)
	// ObjectAspectList is the "+ Aspect" picker: every declared aspect.
	ObjectAspectList(ctx context.Context, in ObjectAspectListInput) (ObjectAspectListOutput, error)
	// ObjectAllowedChildren is the create picker under one parent: the UNION of
	// the parent's allowed_children and every type whose accept_parents admits
	// it, abstract types excluded.
	ObjectAllowedChildren(ctx context.Context, in ObjectAllowedChildrenInput) (ObjectAllowedChildrenOutput, error)

	// An object's attached FILES live in its MODULE: the
	// module is the self-contained unit, so it travels with its own bytes and
	// anyone who clones the repo has them. Every path is module-relative, and
	// an object file (.cwm.yaml / .cwm.md) is NOT reachable here — those have
	// a verb each that checks what a byte write cannot. What does not belong
	// in git is not a file at all: it is a `links` edge (target: uri).
	ObjectFileRead(ctx context.Context, in ObjectFileReadInput) (ObjectFileReadOutput, error)
	ObjectFileWrite(ctx context.Context, in ObjectFileWriteInput) (ObjectFileWriteOutput, error)
	ObjectFileList(ctx context.Context, in ObjectFileListInput) (ObjectFileListOutput, error)
	ObjectFileDelete(ctx context.Context, in ObjectFileDeleteInput) (ObjectFileDeleteOutput, error)
	ObjectFileStat(ctx context.Context, in ObjectFileStatInput) (ObjectFileStatOutput, error)
	// ObjectFileAttach keeps an object's `path` edges (attaches) in step with
	// the files of its module that globs match: it adds the missing edges,
	// removes those whose file is gone, and reports both. Idempotent.
	ObjectFileAttach(ctx context.Context, in ObjectFileAttachInput) (ObjectFileAttachOutput, error)

	// NAMED FILES: a capability declares the files it manages (its `files:`
	// map, module-relative paths with `${...}` over the object); these resolve
	// the declaration and go through the ObjectFile* rules above.
	// ObjectHooks resolves the hook entries that apply to one object, at one
	// point or every point: capabilities, then its type, then the object itself;
	// a closer entry replaces a farther one of the same uri.
	ObjectHooks(ctx context.Context, in ObjectHooksInput) (ObjectHooksOutput, error)
	// DeclarationFindings answers what is wrong with the universe's
	// declarations as CWM reads them: one skipped (it does not decode, or its
	// kind's format is a newer major), one read leniently (a newer minor), an
	// object type not registered, a gate entry its provider's scope narrows.
	DeclarationFindings(ctx context.Context, in DeclarationFindingsInput) (DeclarationFindingsOutput, error)
	// ObjectRunHooks runs one on-demand point (CWM's or an extension's) over an
	// object, its subtree or a module and returns the raw answers — the one
	// runner; reading them is the point owner's (lint and verify: the
	// verification service).
	ObjectRunHooks(ctx context.Context, in ObjectRunHooksInput) (ObjectRunHooksOutput, error)
	// ObjectTransition moves an object to another state of its workflow:
	// the transition must exist from its state and its gate must hold; the
	// state and the history are written and on_transition hooks run. The
	// one way a lifecycle state changes.
	ObjectTransition(ctx context.Context, in ObjectTransitionInput) (ObjectTransitionOutput, error)
	// ObjectEvaluate evaluates expressions over an object as a guard would —
	// the object's vocabulary plus by / from / to, the capabilities'
	// functions — and answers each value with what the functions answered.
	// Read-only.
	ObjectEvaluate(ctx context.Context, in ObjectEvaluateInput) (ObjectEvaluateOutput, error)
	// ObjectAddCapability gives an object a capability — its fields, with the
	// capability's defaults, required fields checked, the capabilities it uses
	// required, and its workflow (if any) started at the initial state.
	ObjectAddCapability(ctx context.Context, in ObjectAddCapabilityInput) (ObjectAddCapabilityOutput, error)
	// ObjectRemoveCapability takes an instance capability off an object.
	ObjectRemoveCapability(ctx context.Context, in ObjectRemoveCapabilityInput) (ObjectRemoveCapabilityOutput, error)
	ObjectNamedFileList(ctx context.Context, in ObjectNamedFileListInput) (ObjectNamedFileListOutput, error)
	ObjectNamedFileRead(ctx context.Context, in ObjectNamedFileReadInput) (ObjectNamedFileReadOutput, error)
	ObjectNamedFileWrite(ctx context.Context, in ObjectNamedFileWriteInput) (ObjectNamedFileWriteOutput, error)
	ObjectNamedFileDeclare(ctx context.Context, in ObjectNamedFileDeclareInput) (ObjectNamedFileDeclareOutput, error)

	// Structure edits that change IDENTITY. A move within a module is
	// ObjectUpdate on parent and needs no verb; these are for what is
	// not that — refiling across modules MINTS A NEW ID, and a split ends one
	// identity and starts several. Each publishes ONE event (moved / split)
	// naming the ids and the reason, and the journal keeps it: the one fact a
	// reader cannot recover afterwards is that these ids were the same thing.
	ObjectMove(ctx context.Context, in ObjectMoveInput) (ObjectMoveOutput, error)
	// ObjectRetype changes an object's type keeping its id, file and edges
	//
	ObjectRetype(ctx context.Context, in ObjectRetypeInput) (ObjectRetypeOutput, error)
	ObjectSplit(ctx context.Context, in ObjectSplitInput) (ObjectSplitOutput, error)

	// outbox slot — what a reaction's writes staged (subscriptions.go): the
	// async matches and on_* hook calls of every write whose cause is a
	// reaction, held until its collector releases them (the reaction
	// succeeded) or drops them (it was given up). Its object events went out
	// at once.
	OutboxRelease(ctx context.Context, in OutboxReleaseInput) (OutboxReleaseOutput, error)
	OutboxDrop(ctx context.Context, in OutboxDropInput) (OutboxDropOutput, error)
	OutboxList(ctx context.Context, in OutboxListInput) (OutboxListOutput, error)

	// change-set slot — an atomic reaction's object writes, staged under its
	// id while its set is open, written together at the commit or not at all
	// (changeset.go). Opened and committed by the reaction's runner.
	ChangeSetBegin(ctx context.Context, in ChangeSetBeginInput) (ChangeSetBeginOutput, error)
	ChangeSetCommit(ctx context.Context, in ChangeSetCommitInput) (ChangeSetCommitOutput, error)
	ChangeSetDiscard(ctx context.Context, in ChangeSetDiscardInput) (ChangeSetDiscardOutput, error)
	ChangeSetList(ctx context.Context, in ChangeSetListInput) (ChangeSetListOutput, error)

	// module slot — the cross-module subscriptions naming a module
	// (module_subscriptions.go): each module object's `on:` entry, with its
	// declared writes and whether it applies.
	ModuleSubscribers(ctx context.Context, in ModuleSubscribersInput) (ModuleSubscribersOutput, error)

	// event slot — an event that is not an object write (event_raise.go):
	// stamped, published, journaled and matched at its type's point on the
	// object, like a write's event. The object is not changed.
	EventRaise(ctx context.Context, in EventRaiseInput) (EventRaiseOutput, error)
	// EventSubscriptions lists the subscriptions to an event type across every
	// layer that declares them (event_subscriptions.go), with no object.
	EventSubscriptions(ctx context.Context, in EventSubscriptionsInput) (EventSubscriptionsOutput, error)

	// edge slot — typed-edge CRUD. The containment parent is the object's
	// `parent` (ObjectCreate / ObjectUpdate / ObjectMove), never an edge.
	EdgeCreate(ctx context.Context, in EdgeCreateInput) (EdgeCreateOutput, error)
	EdgeList(ctx context.Context, in EdgeListInput) (EdgeListOutput, error)
	EdgeDelete(ctx context.Context, in EdgeDeleteInput) (EdgeDeleteOutput, error)

	// events slot — 1 method (channel-returning; non-standard shape)
	//
	// EventsSubscribe returns a receive-only channel of EventEnvelopes
	// and a cancel function. This is the only method that does NOT
	// follow the typed-Input → typed-Output convention. It is hidden
	// from auto-CLI via MethodDef.Hidden=true.
	EventsSubscribe(ctx context.Context, in EventsSubscribeInput) (<-chan EventEnvelope, func(), error)
}

// CWMServiceDef describes the published CWMService for KTSP/HTTP discovery.
//
// Keyed by the Go method name: the KTSP publisher looks a method's
// definition up by that name (internal/kts/local/reflect_dispatch.go), so
// its description, read-only flag, timeout and Hidden flag reach the
// catalog.
var CWMServiceDef = meta.ServiceDef{
	Name:  "cwm",
	Short: "Contexia Workspace Manager — generic substrate",
	Long: `CWM stores a project's objects — one file per object under its
module folder, committed with the code — and exposes a method-neutral,
language-neutral API over them: objects, their files, typed edges,
hooks, transitions and events. Extensions consume CWM and publish
their own slots beside it.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{

		"ObjectEffectiveCapabilities": {Short: "Capabilities in force on an object: its type's, its aspects', its own and those attached to its type, each with its source", ReadOnly: true},
		"ObjectEffectiveSurface":      {Short: "What the type and the capabilities in force contribute to an object: actions, badges, hooks, agent hints, each with its source", ReadOnly: true},

		// object (one object = one committed YAML file under its module folder)
		"ObjectCreate":           meta.Method("Write a new object file under its module, in its type's folder; fails if it exists"),
		"ObjectGet":              {Short: "Load one object by module + id", ReadOnly: true},
		"ObjectUpdate":           meta.Method("Replace an existing object; fails if it does not exist; given the revision read, refuses a newer stored one (conflict)"),
		"ObjectDelete":           meta.Method("Remove one object file"),
		"ObjectArchive":          meta.Method("End an object: its file moves, unchanged, to its module's .ktw/_history/<yyyy-mm>/ and leaves the index; links to it stay valid as ended; one archived event"),
		"ObjectList":             {Short: "List objects from the index, filtered by module / type / parent; at most --limit (50 by default; -1 = all), --fields keeps only the named parts", ReadOnly: true},
		"ObjectTree":             {Short: "Read a containment subtree as nested data — one call, no per-object round trip", ReadOnly: true},
		"ObjectErrorList":        {Short: "Every object carrying errors (invalid, a state outside its workflow, a dangling link) and the files CWM could not read", ReadOnly: true},
		"ObjectScan":             meta.Method("Rebuild the index from the object files and report added / modified / removed"),
		"CatchUpRelease":         meta.Method("Publish the per-object events (operation caught-up) a catch-up of more than 50 moved objects held, by its grouped event's id - the answer to its card; refused (not found) when that catch-up holds none"),
		"ObjectSearch":           {Short: "Search objects by name / title / description / tag / type / module / capability, from the index", ReadOnly: true},
		"ProjectInit":            meta.Method("Mint the project id and write the root project object (identity-and-index.md §7)"),
		"ProjectsKnown":          {Short: "The projects this universe knows - the one it serves and those in the installation's projects folder - each with its type; a capability keeps those whose type carries it", ReadOnly: true},
		"Migrate":                {Short: "Bring the project's data to this binary's data version through its ordered migration steps, then rebuild the index; with dry_run, list every object each step would change and where git stands on the files it writes, and write nothing", TimeoutS: 600},
		"MigrateCommit":          {Short: "Convert the project's data to this binary's data version and commit the files the conversion wrote as one commit (Convert project data from version N to M); refused writing nothing when nothing waits or a file it writes differs from the last commit", Long: "Runs Migrate, then commits only the objects it changed or removed and the project object - another change staged in the index stays out of the commit. Outside a repository, or when a hook refuses the commit, the data is converted and the answer says why no commit was made. Long: from the action surface it answers its run and its panel at once, each step, the index rebuilt and the commit reported into the panel.", TimeoutS: 900, LongRunning: true},
		"ObjectResolve":          {Short: "Resolve a full name or an id to the object's file in this project", ReadOnly: true},
		"ObjectEvaluate":         {Short: "Evaluate expressions over an object as a guard would: each value, with what the capabilities' functions answered", ReadOnly: true},
		"ObjectTransition":       meta.Method("Move an object to another state of its workflow: the transition must exist and its gate hold; state and history written, on_transition hooks run"),
		"ObjectAddCapability":    meta.Method("Give an object a capability: its fields with the defaults, required fields checked, the capabilities it uses required, its workflow started"),
		"ObjectRemoveCapability": meta.Method("Take a capability the object carries itself off it (refused while another uses it)"),
		"ObjectRunHooks":         meta.Method("Run one on-demand hook point (CWM's or an extension's) over an object, its subtree or a module; the raw answers"),
		"ObjectHooks":            {Short: "The hook entries that apply to one object, by point, with who declared each and why one does not apply", ReadOnly: true},
		"ObjectAllowedChildren":  {Short: "List the types creatable under a parent (allowed_children UNION accept_parents), abstract types excluded", ReadOnly: true},
		"ObjectRetype":           meta.Method("Change an object's type keeping its id, its file and its edges: refused for an unknown type, a change of level (project or module), or while the object holds fields the new type does not shape the same way - a data conversion (a migration step) moves them first"),
		"ObjectMove":             meta.Method("Refile an object, minting a new id when the module changes, re-pointing every reference; one moved event, kept by the journal"),
		"ObjectSplit":            meta.Method("Replace one object with several, re-parenting the children each successor adopts; one split event, kept by the journal"),
		"ObjectFileRead":         {Short: "Read a file attached to an object's module; the path is module-relative", ReadOnly: true},
		"ObjectFileWrite":        meta.Method("Write a file into an object's module; returns the project-relative path an attaches edge stores"),
		"ObjectFileList":         {Short: "List an object's module files, object files excluded", ReadOnly: true},
		"ObjectNamedFileList":    {Short: "List the named files every capability in force on an object declares, resolved for it", ReadOnly: true},
		"ObjectNamedFileRead":    {Short: "Read one named file of a capability in force on an object", ReadOnly: true},
		"ObjectNamedFileWrite":   meta.Method("Write or append to one named file of a capability in force on an object"),
		"ObjectNamedFileDeclare": meta.Method("Add or remove a named file on one object -- how a capability gives an object a file at runtime"),
		"ObjectFileDelete":       meta.Method("Remove a file from an object's module"),
		"ObjectFileStat":         {Short: "Describe one of an object's module files without reading it", ReadOnly: true},
		"ObjectFileAttach":       meta.Method("Keep an object's attaches edges in step with the files of its module that globs match: missing edges added, edges to gone files removed, both reported"),
		"ObjectTypeGet":          {Short: "Get one declared object-type by name — the create form reads its capabilities and body schema", ReadOnly: true},
		"ObjectTypeList":         {Short: "List every declared object-type, sorted by name", ReadOnly: true},
		"ObjectAspectList":       {Short: "List every declared aspect — the + Aspect picker", ReadOnly: true},
		"DeclarationFindings":    {Short: "What is wrong with the universe's declarations as CWM reads them: skipped, read leniently (a newer format), not registered, or a gate entry narrowed to its provider's scope", ReadOnly: true},

		// outbox (a reaction's staged triggers; released or dropped by its collector)
		"OutboxRelease": meta.Method("Hand on what a reaction's writes staged: its async matches recorded and handed over, its on_* hook calls journaled and run"),
		"OutboxDrop":    meta.Method("Forget a given-up reaction's staged matches and hook calls; lists the writes it made"),
		"OutboxList":    {Short: "The reactions' outboxes held: writes, matches and hook calls staged in each", ReadOnly: true},

		// change-set (an atomic reaction's staged object writes; opened and committed by its runner)
		"ChangeSetBegin":   meta.Method("Open a reaction's change set, discarding a leftover one: the reaction's object writes are staged until the commit"),
		"ChangeSetCommit":  meta.Method("Write a reaction's staged writes together: checked again, each revision verified, all written or none (conflict)"),
		"ChangeSetDiscard": meta.Method("Drop a reaction's change set: nothing it staged is written"),
		"ChangeSetList":    {Short: "The open change sets and the writes each holds", ReadOnly: true},

		// module (the cross-module subscriptions naming a module)
		"ModuleSubscribers": {Short: "The modules subscribing to a module's events: each module object's on: entry, its event type, point, action, declared writes, and why one does not apply", ReadOnly: true},

		// event (an event that is not an object write, raised on the object it concerns)
		"EventRaise":         meta.Method("Raise an event of a declared type on an object: stamped like a write's event, journaled, its point fired on the object (subscriptions matched, on_* hooks run); the object is not changed"),
		"EventSubscriptions": {Short: "The subscriptions to an event type (or every declared one) across every layer that declares them: module, hook, capability, policy, type and object, each with its source, its action and whether it applies", ReadOnly: true},

		// edge (typed-edge CRUD; the containment parent is the object's `parent`)
		"EdgeCreate": meta.Method("Create a typed edge (kind ≠ parent_id) between two objects"),
		"EdgeList":   {Short: "List typed edges; optional object_id / kind / direction filters", ReadOnly: true},
		"EdgeDelete": meta.Method("Remove a specific (from, to, kind) edge; refused (not found) when there is none"),

		// events
		"EventsSubscribe": {Short: "Subscribe to a topic pattern (server-streamed)", Hidden: true},
	},
}
