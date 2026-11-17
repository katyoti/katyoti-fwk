// SPDX-License-Identifier: Apache-2.0

// Package api defines the public contract for the ktw-univers sidecar.
//
// ktw-univers is a sidecar that provides universe catalog operations:
//   - Catalog reads: ListUniverses, GetUniverse
//   - Catalog writes: CreateUniverse, DeleteUniverse
//   - System inventory: ScanInventory
//
// ktw-univers does NOT manage active-universes (that responsibility lives in
// ktw-gui's Go side: modules/gui/concepts.md §2 (the ActiveCore model)). It does NOT spawn
// child ktw-agents processes. It does NOT have a daemon mode.
//
// Each method takes (ctx, in T) and returns (U, error) per the reflection-
// based dispatch convention used by AutoKTSPContainer.
//
// See:
//   - (modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md)
//   - modules/katyoti/layout.md
//   - modules/univers/decisions/OD-001-where-does-a-fresh-install-manage-its-universes.cwm.md
package api

import (
	"context"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// OrchestratorService is the contract for the ktw-univers sidecar.
//
// Five KTSP methods total:
//   - ListUniverses: catalog read
//   - GetUniverse: catalog read (single entry, full info)
//   - CreateUniverse: catalog write (folder skeleton + registry entry)
//   - DeleteUniverse: catalog write (archive + registry remove)
//   - ScanInventory: system probe (PATH scan for ktw-* binaries)
//
// Active-universe management (Activate/Display/Deactivate/ListActive) lives
// in ktw-gui's Go side (see app/ktw-gui/internal/orchestrator/).
type OrchestratorService interface {
	ListUniverses(ctx context.Context, in ListUniversesInput) (ListUniversesOutput, error)
	GetUniverse(ctx context.Context, in GetUniverseInput) (UniverseInfo, error)
	CreateUniverse(ctx context.Context, in CreateUniverseInput) (UniverseInfo, error)
	DeleteUniverse(ctx context.Context, in DeleteUniverseInput) (DeleteUniverseOutput, error)
	ScanInventory(ctx context.Context, in ScanInventoryInput) (ScanInventoryOutput, error)

	// AddSidecar appends a sidecar to an existing universe's membership
	// (Univers.yaml `sidecars:` block) and returns the updated universe.
	AddSidecar(ctx context.Context, in AddSidecarInput) (UniverseInfo, error)

	// RemoveSidecar removes a sidecar from an existing universe's
	// membership and returns the updated universe.
	RemoveSidecar(ctx context.Context, in RemoveSidecarInput) (UniverseInfo, error)

	// AttachProject links a project folder to a central universe (writes
	// <projectDir>/.katyoti/project.yaml + records it in the universe's
	// projects.yaml). DetachProject reverses it. ListProjects lists the attached
	// project paths. Project↔universe links are a universe-registry concern.
	AttachProject(ctx context.Context, in AttachProjectInput) (AttachProjectOutput, error)
	DetachProject(ctx context.Context, in DetachProjectInput) (DetachProjectOutput, error)
	ListProjects(ctx context.Context, in ListProjectsInput) (ListProjectsOutput, error)

	// SwitchProject makes a universe serve another project in place of the
	// one it serves (« Ouvrir un dossier »): the served
	// project detached, the other attached - refused, leaving the first
	// attached, when the other cannot be (attach's own rules). Already served:
	// nothing changes. The universe's core reads its project at boot, so a
	// change restarts the member serving it through the universe's hub
	// ; the output says whether it was restarted or why not.
	SwitchProject(ctx context.Context, in SwitchProjectInput) (SwitchProjectOutput, error)

	// CreateProject makes a new project under its name in the installation's
	// projects folder - refused, before anything is created, when the name is
	// not a valid one, a project of that name is registered or its folder
	// exists (the rule of a universe name) - and attaches it to a universe
	// when one is named. Check only answers where it would go and why it is
	// refused, creating nothing.
	CreateProject(ctx context.Context, in CreateProjectInput) (CreateProjectOutput, error)

	// ResolveProject answers "which universe on this machine has the project
	// with this id mounted, and where" — the registry half of object-resolve
	// (identity-and-index.md §1). The object half is the universe core's
	// `cwm object-resolve`.
	ResolveProject(ctx context.Context, in ResolveProjectInput) (ResolveProjectOutput, error)

	// UpdateUniverse edits a universe's mutable fields (display name / description /
	// icon / color / sidecar membership); the slug Name is the immutable key.
	UpdateUniverse(ctx context.Context, in UpdateUniverseInput) (UniverseInfo, error)

	// AddExistingUniverse registers a pre-existing universe folder (one that already
	// contains .katyoti/config/Univers.yaml) into the registry. No filesystem
	// mutation — a catalog-side add. Idempotent when the name+path already match.
	AddExistingUniverse(ctx context.Context, in AddExistingUniverseInput) (UniverseInfo, error)

	// SetUniverseSolution gives a universe whose Univers.yaml names no
	// solution an installed one (a universe made before solutions); refused
	// for a solution not installed and for a universe that has one (moving it
	// is the solution's update).
	SetUniverseSolution(ctx context.Context, in SetUniverseSolutionInput) (UniverseInfo, error)

	// RecordCreatedProject records as created on this PC a project folder a
	// creation path outside the orchestrator laid out - the marketplace's
	// seed - so it may later join a universe beside other
	// members. The orchestrator stays the creation record's
	// one writer. Narrow: an existing folder that holds files, recorded only,
	// nothing attached.
	RecordCreatedProject(ctx context.Context, in RecordCreatedProjectInput) (RecordCreatedProjectOutput, error)

	// ForgetCreatedProject drops a folder from the creation record: a creation
	// path outside the orchestrator that recorded the folder it laid out
	// before attaching it - the setup's sample project - forgets it when the
	// attach fails, so the record never names a place a later clone could
	// fill. Narrow: an absolute path, dropped only.
	ForgetCreatedProject(ctx context.Context, in ForgetCreatedProjectInput) (ForgetCreatedProjectOutput, error)
}

// ForgetCreatedProjectInput names the project folder dropped from the record.
type ForgetCreatedProjectInput struct {
	ProjectDir string `json:"projectDir" cli:"arg" desc:"The project folder dropped from the record" jsonschema:"Absolute path to a folder recorded as created on this PC" required:"true"`
}

// ForgetCreatedProjectOutput says whether the record named the folder.
type ForgetCreatedProjectOutput struct {
	ProjectDir string `json:"projectDir" desc:"The folder dropped"`
	Forgotten  bool   `json:"forgotten"  desc:"The record named the folder and no longer does"`
}

// RecordCreatedProjectInput names the project folder a creation path laid out.
type RecordCreatedProjectInput struct {
	ProjectDir string `json:"projectDir" cli:"arg" desc:"The project folder laid out on this PC" jsonschema:"Absolute path to an existing folder holding the project just laid out" required:"true"`
}

// RecordCreatedProjectOutput says the folder is recorded.
type RecordCreatedProjectOutput struct {
	ProjectDir string `json:"projectDir" desc:"The folder recorded"`
	Recorded   bool   `json:"recorded"   desc:"The folder is in the record of the projects created on this PC"`
}

// SetUniverseSolutionInput names the universe and the solution it is given.
type SetUniverseSolutionInput struct {
	Name     string `json:"name"     cli:"arg"      desc:"Universe slug"                     jsonschema:"Existing universe slug - its Univers.yaml names no solution" required:"true"`
	Solution string `json:"solution" cli:"solution" desc:"The installed solution it is given" jsonschema:"<solution> or <solution>@<version> - a name alone takes its installed version" required:"true"`
}

// UpdateUniverseInput edits the mutable fields of an existing universe.
type UpdateUniverseInput struct {
	Name        string                   `json:"name"                 cli:"arg"          desc:"Universe slug (immutable key)"          jsonschema:"Existing universe slug" required:"true"`
	DisplayName string                   `json:"displayName"          cli:"display"      desc:"Human-readable name"                    jsonschema:"1-128 chars"            required:"true"`
	Description string                   `json:"description,omitempty" cli:"description" desc:"Optional description"`
	Icon        string                   `json:"icon,omitempty"       cli:"icon"         desc:"Icon (kept if empty)"`
	Color       string                   `json:"color,omitempty"      cli:"color"        desc:"Mantine color (kept if empty)"`
	Sidecars    []CreateSidecarSelection `json:"sidecars,omitempty"   cli:"-"            desc:"Full sidecar membership (replaces existing)"`
}

// AddExistingUniverseInput registers a pre-initialized universe folder.
type AddExistingUniverseInput struct {
	Path string `json:"path" cli:"arg" desc:"Path to an existing universe folder (contains .katyoti/config/Univers.yaml)" jsonschema:"Absolute path" required:"true"`
}

// AttachProjectInput links ProjectDir to the central universe Universe.
type AttachProjectInput struct {
	ProjectDir string `json:"projectDir" cli:"arg"      desc:"Project folder to attach" jsonschema:"Absolute path to the project folder" required:"true"`
	Universe   string `json:"universe"   cli:"universe" desc:"Central universe name"   jsonschema:"Name of the central universe to attach to" required:"true"`
}

// AttachProjectOutput echoes the resolved link.
type AttachProjectOutput struct {
	ProjectDir string `json:"projectDir" desc:"The attached project folder"`
	Universe   string `json:"universe"   desc:"The universe it is now attached to"`
	// Restarted and RestartError: a universe that runs serves the change at
	// once - the member serving its objects is restarted through its hub, as
	// on SwitchProject. Neither set: nothing changed, or the universe (or that
	// member) does not run.
	Restarted    bool   `json:"restarted,omitempty"    desc:"The member serving the universe's objects was restarted onto the change"`
	RestartError string `json:"restartError,omitempty" desc:"Why that member could not be restarted: the change is recorded but not served yet"`
}

// DetachProjectInput unlinks ProjectDir from its universe: Universe when
// named, else the one the folder's pointer names, else a registered universe
// whose projects.yaml lists the folder - a folder that no longer exists has no
// pointer left.
type DetachProjectInput struct {
	ProjectDir string `json:"projectDir"         cli:"arg"      desc:"Project folder to detach" jsonschema:"Absolute path to the project folder" required:"true"`
	Universe   string `json:"universe,omitempty" cli:"universe" desc:"Universe the folder is attached to (needed when the folder no longer exists)" jsonschema:"Name of the universe to detach the folder from"`
}

// DetachProjectOutput echoes the detached project.
type DetachProjectOutput struct {
	ProjectDir string `json:"projectDir" desc:"The detached project folder"`
	// Restarted and RestartError: a universe that runs serves the change at
	// once - the member serving its objects is restarted through its hub, as
	// on SwitchProject. Neither set: nothing changed, or the universe (or that
	// member) does not run.
	Restarted    bool   `json:"restarted,omitempty"    desc:"The member serving the universe's objects was restarted onto the change"`
	RestartError string `json:"restartError,omitempty" desc:"Why that member could not be restarted: the change is recorded but not served yet"`
}

// SwitchProjectInput names the universe and the project it is to serve.
type SwitchProjectInput struct {
	Universe   string `json:"universe"   cli:"universe" desc:"Universe name"            jsonschema:"Name of the universe that is to serve the project" required:"true"`
	ProjectDir string `json:"projectDir" cli:"arg"      desc:"Project folder to serve" jsonschema:"Absolute path to the project folder the universe is to serve" required:"true"`
}

// SwitchProjectOutput says what the universe serves now and what it left.
type SwitchProjectOutput struct {
	Universe   string   `json:"universe"   desc:"The universe"`
	ProjectDir string   `json:"projectDir" desc:"The project it serves now"`
	Left       []string `json:"left"       desc:"The project folders it no longer serves"`
	Changed    bool     `json:"changed"    desc:"False when it already served that project"`
	// Restarted and RestartError: the member serving the universe's CWM binds
	// its project once, at start, so a change is applied by restarting it
	// through the universe's hub. Neither set: the universe (or that member)
	// does not run, and it starts on the project.
	Restarted    bool   `json:"restarted,omitempty"    desc:"The member serving the universe's objects was restarted onto the project"`
	RestartError string `json:"restartError,omitempty" desc:"Why that member could not be restarted: the project is selected but not served yet"`
}

// ListProjectsInput lists projects attached to Universe.
type ListProjectsInput struct {
	Universe string `json:"universe" cli:"arg" desc:"Central universe name" jsonschema:"Name of the central universe" required:"true"`
}

// ListProjectsOutput wraps the attached project paths.
type ListProjectsOutput struct {
	Projects []string `json:"projects" desc:"Attached project folder paths"`
}

// CreateProjectInput names a new project; Universe attaches it, Check only
// answers where it would go.
type CreateProjectInput struct {
	Name     string `json:"name"               cli:"arg"      desc:"The project's name: lowercase letters, digits and dashes, like a universe name" jsonschema:"^[a-z0-9-]{1,64}$" required:"true"`
	Universe string `json:"universe,omitempty" cli:"universe" desc:"The universe to attach the new project to (none: the folder is only created); a check with it also says whether that universe can take one"`
	Check    bool   `json:"check,omitempty"    cli:"check"    desc:"Only answer the folder it would make and why it is refused: nothing is created"`
	// Type, Title and Capabilities make a typed project: the
	// new folder is a git repository holding the seed of its project object,
	// which CWM mints at its first opening. No type: as before.
	Type         string                    `json:"type,omitempty"         cli:"type"  desc:"Its project type (a dossier-architecture...): the folder becomes a git repository and its project object is seeded with this type; none means the default project made at its first opening"`
	Title        string                    `json:"title,omitempty"        cli:"title" desc:"Its project object's display name; its name when empty"`
	Capabilities map[string]map[string]any `json:"capabilities,omitempty" desc:"The capability groups its project object carries (its identity: dossier, cadrage...) - checked by CWM when it mints the object"`
}

// CreateProjectOutput is the project's folder and, when refused, why.
type CreateProjectOutput struct {
	Name       string `json:"name"                 desc:"The project's name"`
	ProjectDir string `json:"projectDir"           desc:"The folder it is created in (or would be): <projects folder>/<name>"`
	Universe   string `json:"universe,omitempty"   desc:"The universe it is attached to"`
	Refused    string `json:"refused,omitempty"    desc:"Why it cannot be created - answered by a check; a creation refused is an error"`
	Reason     string `json:"reason,omitempty"     desc:"What refuses it, for a form to say in its language: invalid-name, registered, folder-exists or inside-installation" enum:"invalid-name,registered,folder-exists,inside-installation"`
	Created    bool   `json:"created,omitempty"    desc:"The folder was created"`
	// Restarted and RestartError: a universe that runs serves the change at
	// once - the member serving its objects is restarted through its hub, as
	// on SwitchProject. Neither set: nothing changed, or the universe (or that
	// member) does not run.
	Restarted    bool   `json:"restarted,omitempty"    desc:"The member serving the universe's objects was restarted onto the change"`
	RestartError string `json:"restartError,omitempty" desc:"Why that member could not be restarted: the change is recorded but not served yet"`
}

// ResolveProjectInput names a project by its id, or by a full object name
// whose first segment is the id.
type ResolveProjectInput struct {
	Ref string `json:"ref" cli:"arg" desc:"A project uuid, or a full object name (<project-uuid>.<module>.<local>)" jsonschema:"Project uuid or full object name" required:"true"`
}

// ResolveProjectOutput says where the project is mounted on this machine.
type ResolveProjectOutput struct {
	ProjectID  string `json:"project_id"`
	Universe   string `json:"universe"    desc:"The universe that mounts it"`
	ProjectDir string `json:"project_dir" desc:"The mounted folder"`
	Name       string `json:"name"        desc:"The project object's id (its name)"`
	ObjectID   string `json:"object_id,omitempty" desc:"The authored id, when a full name was given"`
}

// OrchestratorServiceDef declares the OrchestratorService interface for
// auto-generation by the publish framework + AutoKTSPContainer.
var OrchestratorServiceDef = meta.ServiceDef{
	Name:    "orchestrator",
	Short:   "Universe catalog operations (list, get, create, delete, scan-inventory)",
	Long:    `ktw-univers is a sidecar that manages the universe catalog at ~/.config/katyoti/universes.yaml. It provides catalog reads (List, Get), catalog writes (Create, Delete), and system inventory scanning. It does NOT manage active universes — that lives in ktw-gui per the orchestrator architecture ADR.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"ListUniverses": {
			Short:          "List all registered universes",
			Long:           `Returns the catalog of universes from ~/.config/katyoti/universes.yaml.`,
			RenderTemplate: listUniversesTmpl,
		},
		"GetUniverse": {
			Short:          "Get full metadata for a named universe",
			Long:           `Reads <home>/.katyoti/config/Univers.yaml for the universe and returns its full topology (sidecars, layered homes). Errors if the universe is not in the registry.`,
			RenderTemplate: getUniverseTmpl,
		},
		"CreateUniverse": meta.MethodLong(
			"Create a new universe",
			`Writes a folder skeleton at the chosen home path: .katyoti/config/Univers.yaml (identity + the sidecars: block), .katyoti/.env (empty), .katyoti/config/Settings.yaml. Adds the universe to the registry. Activation is a separate concern owned by ktw-gui.`,
		),
		"DeleteUniverse": meta.MethodLong(
			"Archive a universe and remove it from the registry",
			`Moves the universe folder to <parent>/.archived/<name>/ and removes the registry entry. Errors on protected universes (e.g. admin).`,
		),
		"AddSidecar": meta.MethodLong(
			"Add a sidecar to an existing universe",
			`Appends a SidecarSpec row to the universe's Univers.yaml (transport nats, args "sc ktsp nats" — the bus-split convention). Validates: universe exists, no duplicate sidecar name, binary path (when given) exists. Atomic write.`,
		),
		"RemoveSidecar": meta.MethodLong(
			"Remove a sidecar from an existing universe",
			`Removes the named sidecar's row from the universe's Univers.yaml. Errors if the universe or the sidecar is not present. Atomic write.`,
		),
		"ScanInventory": meta.MethodLong(
			"Scan the system for available sidecar binaries",
			`What a universe can be composed from on this machine: every INSTALLED sidecar (the versioned store, with its version and provenance) first, then ktw-* binaries discovered beside the running binary and on $PATH (dev copies), never the control plane. Fresh scan each call; discovered rows are recorded in sidecars.yaml, installed rows are never rewritten. Used by the universe editor and the creation wizard.`,
		),
		"AttachProject": {
			Short:    "Attach a project folder to a central universe",
			Long:     `Writes <projectDir>/.katyoti/project.yaml (universe: <name>) and records the project in the universe's projects.yaml. A universe attaches several projects, each served by a store of its own: a project joining others needs a key no other member holds (its project object's id; before its first opening, the name of the seed CWM mints that object from, else its folder's name) and a contributions token no other member publishes under - refused otherwise, nothing written. A universe that runs is made to serve it: the member serving its objects is restarted through its hub, so the call waits up to the restart's bound; the output says whether it was restarted or why not.`,
			TimeoutS: 300,
		},
		"DetachProject": {
			Short:    "Detach a project from its universe",
			Long:     `Removes the project.yaml pointer + deregisters from the universe's projects.yaml. The project reverts to self-contained. A universe that runs stops serving it: the member serving its objects is restarted through its hub, so the call waits up to the restart's bound; the output says whether it was restarted or why not.`,
			TimeoutS: 300,
		},
		"SwitchProject": {
			Short:    "Make a universe serve another project in place of the one it serves",
			Long:     `Detaches the served project and attaches the other - refused, leaving the first attached, when the other cannot be attached. Already served: nothing changes. A change restarts the member serving the universe's objects through its hub, so the call waits up to the restart's bound; the output says whether it was restarted or why not.`,
			TimeoutS: 300,
		},
		"CreateProject": {
			Short:    "Create a new project under its name in the projects folder",
			Long:     `Makes <projects folder>/<name> and attaches it to the universe named - a universe that runs is made to serve it, its serving member restarted as on AttachProject. Refused before anything is created when the name is not lowercase letters, digits and dashes, when a project of that name is registered in a universe of this machine, when its folder already exists, or when it lies inside the installation - the rule of a universe name. check answers the folder and the refusal without creating anything.`,
			TimeoutS: 300,
		},
		"ResolveProject": {
			Short: "Which universe mounts the project with this id, and where",
			Long:  `Walks the universe registry and each universe's projects.yaml, reading every mounted project's project.cwm.yaml for its uuid. A full object name is accepted: its first segment is the project id.`,
		},
		"ListProjects": {
			Short:          "List projects attached to a universe",
			Long:           `Returns the project folder paths recorded in the universe's projects.yaml.`,
			RenderTemplate: listProjectsTmpl,
		},
		"UpdateUniverse": meta.MethodLong(
			"Edit a universe's mutable fields",
			`Updates display name / description / icon / color / sidecar membership; the slug Name is the immutable key. Atomic write of Univers.yaml.`,
		),
		"SetUniverseSolution": meta.MethodLong(
			"Give a universe with no solution an installed one",
			`For a universe made before solutions: writes solution: <name>@<version> into its Univers.yaml, every other field kept, so a project whose type needs that solution opens in it. Refused for a solution no universe of this machine nor the installation was installed from, for a version not installed, for a name installed in several versions (name one) and for a universe that already has a solution (moving it is katyoti solution update). A universe that runs reads it at its next start.`,
		),
		// A trust attestation: a recorded folder may later join a universe
		// beside other members, past guard against a cloned
		// repository. Kept off every LLM tool surface (NoLLM -> mcp=false);
		// the marketplace calls it in process or over KTSP.
		"RecordCreatedProject": {
			Short: "Record a project folder laid out on this PC as created here",
			Long:  `For the creation paths outside the orchestrator - the marketplace's seed laying its project out: adds the folder to created-projects.yaml beside the universes registry, so it may later join a universe beside other members (until third-party sidecars are secured, only a repository created on this PC does). Refused for a path that is not absolute, not an existing folder, or an empty one; nothing is attached. A folder recorded already is left as it is. Not an agent tool.`,
			NoLLM: true,
		},
		// The record's other half: a creation outside the orchestrator whose
		// attach fails drops what it recorded. Off the LLM surfaces too - the
		// record is plumbing of the creation paths, never an agent's.
		"ForgetCreatedProject": {
			Short: "Drop a project folder from the record of the projects created on this PC",
			Long:  `For the creation paths outside the orchestrator that record the folder they laid out before attaching it - the setup's sample project: when the attach fails, the folder is dropped from created-projects.yaml so the record never names a place a later clone could fill. Refused for a path that is not absolute; a folder the record does not name is left as it is (forgotten=false). Not an agent tool.`,
			NoLLM: true,
		},
		"AddExistingUniverse": meta.MethodLong(
			"Register an existing universe folder",
			`Adds a pre-initialized universe folder (already carrying .katyoti/config/Univers.yaml) to the registry. No filesystem mutation. Idempotent when name+path already match.`,
		),
	},
}

// ============================================================================
// Core entity types — universe model
// ============================================================================

// UniverseSummary is the lightweight catalog entry for a universe.
// Returned by ListUniverses.
//
// No `Default` field exists: default-displayed is GUI state
// (~/ktw-agents/config/GuiSettings.yaml + per-window localStorage), not a
// per-universe property.
type UniverseSummary struct {
	Version     int    `json:"version,omitempty"     desc:"Universe schema version (monotonic integer)" jsonschema:"v1 = 1; bumps on incompatible Univers.yaml format changes."`
	Name        string `json:"name"        desc:"Slug (lowercase, hyphens)"   jsonschema:"Universe slug — lowercase letters, digits, hyphens; matches ^[a-z0-9-]{1,64}$"`
	DisplayName string `json:"displayName" desc:"Human-readable name"          jsonschema:"Human-readable name shown in the IconRibbon flyout and CLI tables"`
	Description string `json:"description,omitempty" desc:"Optional description" jsonschema:"Optional one-line description"`
	Icon        string `json:"icon"        desc:"Single character or short tag" jsonschema:"Single-character icon (or 1-2 chars) shown as the IconRibbon avatar"`
	Color       string `json:"color"       desc:"Mantine color name"            jsonschema:"Mantine color preset (violet, blue, teal, lime, yellow, orange, red, pink, grape)"`
	HomePath    string `json:"homePath"    desc:"Absolute path to the universe home folder" jsonschema:"Absolute path to the universe home folder (contains config/Univers.yaml)"`
	Protected   bool   `json:"protected,omitempty" desc:"True if the universe is protected from delete (e.g. admin)" jsonschema:"Set when the registry entry has protected: true. Only the admin universe carries this flag in v1."`

	// GUI is the gui section of Univers.yaml (cenv.UniverseGUI); absent, the
	// GUI shows everything.
	FirstGuide string            `json:"firstGuide,omitempty" desc:"The guide the universe starts with" jsonschema:"A guide-recipe URI among its sidecars' - <sidecar>:/guide-recipe/<id> - played at its first open"`
	Tags       []string          `json:"tags,omitempty" desc:"What the universe is for: the universe-tag contributions whose allow-lists choose what its GUI shows" jsonschema:"The tags of Univers.yaml - each a universe-tag a member pack declares; none: the GUI shows everything"`
	GUI        *cenv.UniverseGUI `json:"gui,omitempty" desc:"What the universe's GUI hides and lists first" jsonschema:"The gui section of Univers.yaml: hide (Global strip entries and rail items left out unless Mode expert) and first (task hashtags listed first in Home)"`
	// Solution is the solution that installed the universe
	// (<solution>@<version>); empty for a universe made by hand.
	Solution string `json:"solution,omitempty" desc:"The solution that installed the universe" jsonschema:"<solution>@<version> - the solution whose install made this universe; empty for a universe made by hand"`
	// NoWelcome is the universe's setting turning its welcome screens off
	// (Univers.yaml no_welcome): its GUI opens neither its first guide nor
	// « Quoi de neuf » by itself.
	NoWelcome bool `json:"noWelcome,omitempty" desc:"Its GUI opens neither its first guide nor Quoi de neuf by itself" jsonschema:"Univers.yaml no_welcome - for a test or a screen capture's universe; absent: the welcome screens open as usual"`
}

// UniverseInfo is the full metadata for a universe (Univers.yaml contents).
// Returned by GetUniverse.
type UniverseInfo struct {
	Summary  UniverseSummary `json:"summary"  desc:"Catalog metadata"  jsonschema:"Lightweight summary fields (also returned by ListUniverses)"`
	Sidecars []SidecarSpec   `json:"sidecars" desc:"Membership list from Univers.yaml" jsonschema:"Full sidecar membership list with transport + spawn details"`
	// LayeredHomes lists additional home paths declared in the universe's
	// Settings.yaml `homes:` field (skill/task/sidecar source overlay).
	// Empty if the universe doesn't use layered homes.
	LayeredHomes []string `json:"layeredHomes,omitempty" desc:"Additional home paths from Settings.yaml" jsonschema:"Optional list of additional home paths layered on top of the primary universe home"`
}

// SidecarSpec describes a single sidecar in a universe's membership.
//
// Re-exported from sidecar/api — the canonical schema shared between the GUI's
// writer and the runtime's reader. The alias points DOWNWARD at a leaf package
// (4 first-party dependencies); aliasing it from appcore instead would drag the
// whole application framework into every consumer of this API.
type SidecarSpec = sidecarapi.SidecarSpec

// SidecarBinary is one entry from ScanInventory.
type SidecarBinary struct {
	Name       string `json:"name"       desc:"Binary name"                   jsonschema:"Binary name (e.g. ktw-docs)"`
	BinaryPath string `json:"binaryPath" desc:"Absolute path to the binary"   jsonschema:"Absolute path to the discovered binary"`
	Version    string `json:"version,omitempty" desc:"Probed version"         jsonschema:"Version string from <binary> version, if discoverable"`
	Source     string `json:"source"     desc:"Discovery source"              jsonschema:"Provenance: marketplace / infra-channel (an install), or executable-dir / PATH (a discovered dev binary)"`
}

// ============================================================================
// I/O types
// ============================================================================

// ListUniversesInput is intentionally empty (no parameters).
type ListUniversesInput struct{}

// ListUniversesOutput wraps the slice for stable KTSP serialization.
type ListUniversesOutput struct {
	Universes []UniverseSummary `json:"universes" desc:"All registered universes"`
}

type GetUniverseInput struct {
	Name string `json:"name" cli:"arg" desc:"Universe name" jsonschema:"Universe name to fetch" required:"true"`
}

// CreateUniverseInput collects the fields the wizard sends (or the CLI
// `ktw-univers create <name>` accepts via flags).
type CreateUniverseInput struct {
	Name        string                   `json:"name"                 cli:"arg"          desc:"Slug (lowercase, hyphens)" jsonschema:"Universe slug; must match ^[a-z0-9-]{1,64}$"     required:"true"`
	DisplayName string                   `json:"displayName"          cli:"display"      desc:"Human-readable name"        jsonschema:"Human-readable name (1-128 chars)"               required:"true"`
	Description string                   `json:"description,omitempty" cli:"description" desc:"Optional description"       jsonschema:"Optional one-line description"`
	Icon        string                   `json:"icon"                 cli:"icon"         desc:"1-2 character icon"         jsonschema:"Icon shown in IconRibbon (1-2 chars; default: first char of DisplayName uppercased)"`
	Color       string                   `json:"color"                cli:"color"        desc:"Mantine color preset"       jsonschema:"One of: violet, blue, teal, lime, yellow, orange, red, pink, grape" enum:"violet,blue,teal,lime,yellow,orange,red,pink,grape"`
	HomePath    string                   `json:"homePath"             cli:"home"         desc:"Absolute path to home folder" jsonschema:"Absolute path; default suggestion: ~/.config/katyoti/universes/<name>/"`
	Sidecars    []CreateSidecarSelection `json:"sidecars,omitempty"   cli:"-"            desc:"Sidecar membership"         jsonschema:"List of sidecars to include in this universe; populated by the wizard from ScanInventory"`
	// FirstGuide is the guide the universe starts with: one of the chosen
	// sidecars' guides; refused otherwise.
	FirstGuide string `json:"firstGuide,omitempty" cli:"first-guide" desc:"The guide the universe starts with: a guide-recipe URI of one of its sidecars"`
	// NoWelcome creates the universe with its welcome screens off
	// (Univers.yaml no_welcome) - what a scenario case's universe asks.
	NoWelcome bool `json:"noWelcome,omitempty" cli:"no-welcome" desc:"Create it with its welcome screens off: its GUI opens neither its first guide nor Quoi de neuf by itself"`
	// Tags are what the universe is for (Univers.yaml `tags:`): each a
	// universe-tag a member declares - the GUI shows what they allow.
	Tags []string `json:"tags,omitempty" cli:"tag" desc:"What the universe is for: universe-tags its sidecars declare (its GUI shows what they allow)"`
	// A universe holds no objects of its own: they live in the project it
	// attaches. WithProject gives it one — ProjectDir, else ~/Katyoti/
	// [<installation>/]<name> — created when missing, attached at once.
	WithProject bool   `json:"withProject,omitempty" cli:"with-project" desc:"Give the universe a project of its own (ProjectDir, else ~/Katyoti/[<installation>/]<name>): created when missing and attached"`
	ProjectDir  string `json:"projectDir,omitempty"  cli:"project"      desc:"Folder of the universe's project (implies with-project): created when missing and attached"`
}

// CreateSidecarSelection is a wizard-level selection — minimal info; the
// service builds the full SidecarSpec by reading the sidecar's embedded
// default config.
type CreateSidecarSelection struct {
	Name       string `json:"name"        desc:"Sidecar name"      jsonschema:"Matches a name from ScanInventory"     required:"true"`
	BinaryPath string `json:"binaryPath"  desc:"Absolute binary path" jsonschema:"Absolute path; usually copied from the inventory entry"`
}

// AddSidecarInput adds one sidecar to an existing universe.
type AddSidecarInput struct {
	Universe string                 `json:"universe" cli:"arg" desc:"Universe name" jsonschema:"Existing universe from the registry" required:"true"`
	Sidecar  CreateSidecarSelection `json:"sidecar"            desc:"Sidecar to add" jsonschema:"Name + optional absolute binary path" required:"true"`
}

// RemoveSidecarInput removes one sidecar from an existing universe.
type RemoveSidecarInput struct {
	Universe string `json:"universe" cli:"arg" desc:"Universe name" jsonschema:"Existing universe from the registry" required:"true"`
	Sidecar  string `json:"sidecar"  cli:"arg" desc:"Sidecar name to remove" jsonschema:"Must be a current member of the universe" required:"true"`
}

type DeleteUniverseInput struct {
	Name string `json:"name" cli:"arg" desc:"Universe name" jsonschema:"Universe to archive" required:"true"`
}

type DeleteUniverseOutput struct {
	Archived    bool   `json:"archived"    desc:"True if archived; false if did not exist" jsonschema:"True if the universe was archived; false if it was not in the registry"`
	ArchivePath string `json:"archivePath,omitempty" desc:"Where the archived folder was moved to" jsonschema:"Path under <parent>/.archived/<name>/ where the folder now lives"`
}

type ScanInventoryInput struct{}

type ScanInventoryOutput struct {
	Sidecars []SidecarBinary `json:"sidecars" desc:"Discovered sidecar binaries"`
}
