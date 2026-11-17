// SPDX-License-Identifier: Apache-2.0

// Package core assembles every cwm/internal/* sub-service into a
// single Service that satisfies api.CWMService. Used by
// cwm/local's Extension() to register the service with the
// appcore PublishService infrastructure.
//
// Composition order (see service.go::New):
//
//	storage → concurrency (acquired by storage.New)
//	graph    → resolver
//	search   → context  → sharedstate
//	memory   → refactor → source
//	edges    → bootstrap
//
// Workspace folder discovery (the `.katyoti` folder) is
// the application bootstrap layer's job (see
// `api.DiscoverWorkspaceRoot`). This package takes the
// workspace root path as an explicit Config field.
package core

import (
	"encoding/json"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// CWMServiceConfig bundles the per-instance settings for the cwm
// Service. The appcore publish layer maps it to `CWMService.yaml`,
// but every field is injected at construction time (`yaml:"-"`) —
// the file carries no operator-tunable settings today.
type CWMServiceConfig struct {
	// WorkspaceRoot is the absolute path to the workspace folder.
	// COMPUTED, never read from file: the application bootstrap layer
	// resolves it from the active universe/workspace (typically
	// `<project>/.katyoti`) and injects it at construction time.
	WorkspaceRoot string `yaml:"-" json:"-"`

	// ProjectRoot is the folder that holds `modules/` — the workspace FOLDER,
	// i.e. the PARENT of WorkspaceRoot, which is the config root inside it.
	//
	// THEY ARE NOT THE SAME PLACE, and treating them as one hid 474 objects.
	// `ModulesDirName` calls itself "the Project-root folder holding module
	// folders" and its own example is a repository path, but the index was
	// joining `modules/` onto the CONFIG root — so it scanned
	// `<project>/.katyoti/modules/` and never `<project>/modules/`, where the
	// committed corpus actually lives. Every authored module was invisible to
	// the running service while the tree rendered perfectly.
	//
	// It also could not have been fixed by moving the corpus: `.katyoti/` is
	// gitignored, and G2 requires version-bound content to be COMMITTED with
	// the code it describes. A scan root under it can never hold the corpus.
	//
	// EMPTY FALLS BACK TO WorkspaceRoot. Substrate tests construct this config
	// directly with a bare temp dir playing the config root's part, and
	// stepping up from there would scan the whole system temp folder.
	ProjectRoot string `yaml:"-" json:"-"`
	// ProjectsFolder is the installation's projects folder, where the
	// projects this universe knows besides its own live (ProjectsKnown);
	// empty: the installation's, read at the call.
	ProjectsFolder string `yaml:"-" json:"-"`

	// TypeSources is the ordered list of embedded FS sources that
	// contribute item + edge kind YAMLs underneath the workspace's
	// own `types/_kinds/` + `_edge_kinds/` directories. Later
	// entries in the slice win on collision; the workspace dir is
	// always layered last (closer-wins for user overrides). Empty
	// = workspace dir only (substrate-test behaviour).
	//
	// Not serialized — the application bootstrap layer injects this
	// at service-construction time. See
	// `cwmlocal.Options.TypesFS`.
	TypeSources []fs.FS `yaml:"-" json:"-"`

	// CatalogTypes holds CWM type registrations resolved from the
	// universe-wide aggregate catalog at construction time — types
	// contributed by ANY sidecar (over the bus), layered onto the
	// in-binary floor (TypeSources) with FLOOR-WINS semantics. Empty
	// when no catalog is available → the store opens on the floor alone
	// (degraded: types from other sidecars absent). Injected by
	// cwmlocal from `mbapi.Catalog`. Not serialized.
	CatalogTypes CatalogTypeSet `yaml:"-" json:"-"`

	// ObjectTypeValidation is how ObjectCreate/Update treat an object that is
	// not valid (its type, its capability groups against their schemas, its
	// edges): "off" | "warn" | "error" (api.TypeValidation*). Empty — and any
	// unrecognised value — is "error": the write is refused with the reason.
	//
	// IT IS A NO-OP WHEN NO OBJECT-TYPES ARE DECLARED, whatever the mode. A
	// workspace that has not migrated must behave exactly as before rather than
	// refuse every write — otherwise turning the feature on breaks everything at
	// once, which is how a safety feature gets disabled permanently.
	ObjectTypeValidation string `yaml:"objectTypeValidation,omitempty" json:"objectTypeValidation,omitempty"`

	// TenantID + UniverseID + WorkspaceID identify the workspace
	// in the NATS subject scheme (see
	// modules/bus/nats_subjects_list.md). The events service
	// uses these to construct subjects on every emit. Default
	// values (when the bootstrap layer doesn't set them) are
	// "local" / "default" / the workspace-root node UUID.
	TenantID   string `yaml:"-" json:"-"`
	UniverseID string `yaml:"-" json:"-"`
	// UniverseSolution is the solution that installed the universe
	// (<solution>@<version>, Univers.yaml): a project whose type needs
	// another is refused at opening.
	UniverseSolution string `yaml:"-" json:"-"`
	WorkspaceID      string `yaml:"-" json:"-"`

	// MinterDir is the folder of this installation's minter and its open
	// number blocks (TenantDataDir/cwm). Empty: numbers are
	// minted as in a project out of git - the next after the highest.
	MinterDir string `yaml:"-" json:"-"`
	// BackupDir is where a migration keeps a copy of the project files it
	// removes (under the universe's data folder). Empty: a step that would
	// remove files refuses to.
	BackupDir string `yaml:"-" json:"-"`
	// NumberBlockSize is the numbers one block reservation takes (default 50).
	NumberBlockSize int `yaml:"numberBlockSize,omitempty" json:"numberBlockSize,omitempty"`

	// EventBus is the in-process pub/sub used by the events
	// service. nil = events publishing becomes a no-op + every
	// EventsSubscribe returns a closed channel. The bootstrap
	// layer plumbs the application-level EventBus (typically
	// MemoryEventBus today; a NATS-backed bus when the bridge
	// ships).
	EventBus eventbus.EventBus `yaml:"-" json:"-"`

	// MessageBus is the cross-sidecar NATS pub/sub. When non-nil
	// the CWM events service ALSO publishes lifecycle events to
	// NATS so cross-binary subscribers (via EventService — see
	// `internal/extensions/events/`) receive them. The in-process
	// EventBus publish stays in parallel for backward-compat with
	// existing in-process subscribers. cwm-v2-providers US-15
	// (2026-06-08). nil = no NATS publish, the bus stays the
	// in-process EventBus only.
	MessageBus mbapi.MessageBus `yaml:"-" json:"-"`
}

// CatalogTypeSet bundles the CWM type registrations resolved from the
// aggregate catalog (other sidecars' contributions). Layered onto the
// in-binary floor by storage.TypeRegistry.Merge (floor-wins).
type CatalogTypeSet struct {
	Edges []api.EdgeTypeRegistration
	// Objects are `object-type` contributions — what an Object.Type MEANS.
	Objects []api.ObjectType
	// Capabilities are `capability` contributions — what one capability id
	// MEANS. An object-type is a named list of these, so this is what makes a
	// type's bundle resolvable to fields, panels, actions, checks and hooks.
	Capabilities []api.Capability
	// Aspects are `aspect` contributions — named capability lists an object may
	// add in one move (7g). Validated against Capabilities at service init.
	Aspects []api.Aspect
	// Workflows are `workflow` contributions: the state machines lifecycles
	// move through, named by an object type's `workflow`.
	Workflows []api.Workflow
	// Policies are the rules put on objects by type (api.Policy).
	Policies []api.Policy
	// HookTypes are `hook-type` contributions: the points hooks run at (CWM's
	// own and any an extension opens). Hooks are `hook` contributions: hooks
	// at a point, from a pack, applying to the objects their filter admits.
	HookTypes []api.HookType
	Hooks     []api.Hook
	// EventTypes are `event-type` contributions: the events modules emit,
	// each naming the point an async subscription to it is declared at.
	EventTypes []api.EventType
	// CapabilitySchemas are the capabilities' schemas, compiled to JSON
	// Schema, by data-type URI (a capability's `schema` and `type_schema`, a
	// type's `fields`). They are what a stored group, and a type's section,
	// is checked against: resolved and compiled by the host from the
	// catalog, so CWM validates without calling out on each write.
	CapabilitySchemas map[string]json.RawMessage
	// Grants are what the universe grants each sidecar beyond what it owns
	// (Univers.yaml `sidecars[].grants.gate`): where its gate entries may
	// also refuse writes (api.Scope). Replaced, not merged, on a refresh.
	Grants api.Grants
	// Findings are what reading the catalog skipped or read leniently
	// (api.Declarations.Findings). Replaced on a refresh.
	Findings []api.DeclarationFinding
	// Sources, Members and Named say where each declaration comes from, which
	// providers the snapshot read and which URIs their manifests name
	// (api.Declarations): a refresh replaces declarations by them.
	// A declaration with no source (one not read from a catalog) is replaced
	// by name and never dropped.
	Sources map[string]api.DeclarationSource
	Members map[string]bool
	Named   map[string]bool
	// Overridden are the declarations, by api.DeclarationKey, a project's
	// override entry changed (api.Declarations.Overridden): an edge type
	// among them replaces the floor's of its name, where a catalog
	// declaration otherwise never does.
	Overridden map[string]bool
}

// overridingEdges are the catalog's edge types a project's override entry
// changed: they win over the floor.
func (c CatalogTypeSet) overridingEdges() []api.EdgeTypeRegistration {
	var out []api.EdgeTypeRegistration
	for _, e := range c.Edges {
		if c.Overridden[api.DeclarationKey("edge-type", string(e.Name))] {
			out = append(out, e)
		}
	}
	return out
}

// Validate returns an error when the config is unusable.
func (c *CWMServiceConfig) Validate() error {
	if c == nil || c.WorkspaceRoot == "" {
		return apiInvalid("WorkspaceRoot required")
	}
	return nil
}

// apiInvalid is a tiny helper that wraps a string into the
// canonical invalid-input error.
func apiInvalid(msg string) error {
	return wrapErr(api.ErrInvalidInput, msg)
}

// ObjectsRoot is the folder whose `modules/` holds the object corpus.
//
// Node storage, the graph and the type store all stay on WorkspaceRoot: they
// are workspace STATE, recreatable and gitignored. Objects are different —
// they are authored, committed content — so they get their own root, and this
// is the single place that difference is expressed.
func (c *CWMServiceConfig) ObjectsRoot() string {
	if c == nil {
		return ""
	}
	if c.ProjectRoot != "" {
		return c.ProjectRoot
	}
	return c.WorkspaceRoot
}
