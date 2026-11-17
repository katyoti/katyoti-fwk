// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
)

// SetupService is the katyoti engine contract (spec §1). Both the katyoti CLI
// and ktw-gui resolve it in-process. Marketplace authoring/publishing is NOT
// here — that lives in ktw-compose (ADR §9).
//
// Install/Update methods are stubbed in this first slice (marketplace-and-
// catalog); they are filled by the install-and-update-pillars todo.
type SetupService interface {
	// Marketplaces (subscription — writes the user overlay).
	ListMarketplaces(ctx context.Context) ([]Marketplace, error)
	// AddMarketplace subscribes to (or updates) a marketplace. Input.PubKey is the
	// pinned trust root (a minisign base64 key line, "" for none); Input.Trust is
	// TrustNone | TrustPinned ("" defaults to none, or pinned when a key is given).
	AddMarketplace(ctx context.Context, in AddMarketplaceInput) (AddMarketplaceOutput, error)
	RemoveMarketplace(ctx context.Context, in RemoveMarketplaceInput) (RemoveMarketplaceOutput, error)
	SetMarketplaceEnabled(ctx context.Context, in SetMarketplaceEnabledInput) (SetMarketplaceEnabledOutput, error)
	SetMarketplaceChannel(ctx context.Context, in SetMarketplaceChannelInput) (SetMarketplaceChannelOutput, error)
	RefreshCatalog(ctx context.Context, in RefreshCatalogInput) (Catalog, error) // Name "" = all enabled

	// Catalog (read).
	SearchItems(ctx context.Context, in SearchItemsInput) ([]ItemSummary, error)
	GetItem(ctx context.Context, in GetItemInput) (Item, error)

	// Install → AVAILABLE.
	Install(ctx context.Context, ref ItemRef) (Installed, error)
	Uninstall(ctx context.Context, in UninstallInput) (UninstallOutput, error)
	ListInstalled(ctx context.Context) ([]Installed, error) // the available registry

	// Update.
	Outdated(ctx context.Context) ([]Update, error)
	Update(ctx context.Context, in UpdateInput) ([]Update, error) // empty Names = all outdated

	// InfraVersions lists what the infra channel can serve RIGHT NOW.
	//
	// It reads the pointer rather than listing a directory, because neither
	// HTTP nor a synced share offers a reliable listing -- and because the
	// pointer is the authority on which versions are reachable anyway.
	InfraVersions(ctx context.Context) (InfraVersionsOutput, error)

	// InfraOutdated reports the control-plane binaries behind the infra channel.
	// Separate from Outdated, which diffs catalog-installed items against their
	// catalog and so never sees infra at all.
	InfraOutdated(ctx context.Context) ([]Update, error)

	// ListConfigSets returns the `kind: config` items across enabled marketplaces.
	ListConfigSets(ctx context.Context) ([]ItemSummary, error)
	// ShowConfigSet returns one config-set's catalog versions plus, when it is
	// installed, the keys its bundle declares (read from the unpacked manifest).
	ShowConfigSet(ctx context.Context, in ShowConfigSetInput) (ShowConfigSetOutput, error)
	// ApplyConfigSet drops a config-set's templates into a universe and records the
	// supplied key values into the scoped .env. It does NOT prompt: it reports every
	// aggregated key (with the current value as a default) and which are still
	// missing, so the caller — a CLI prompt loop or a GUI form — can collect them and
	// call again. That keeps the prompt in the face and every write in the engine.
	// An empty name applies the universe's own key set: the keys its members
	// declare, saved the same way, with no config-set.
	ApplyConfigSet(ctx context.Context, in ApplyConfigSetInput) (ApplyConfigSetOutput, error)
	// ListSeeds returns the `kind: seed` items across enabled marketplaces.
	ListSeeds(ctx context.Context) ([]ItemSummary, error)
	// ShowSeed describes one seed: its versions and what each requires.
	ShowSeed(ctx context.Context, in ShowSeedInput) (ShowSeedOutput, error)
	// ApplySeed lays a seed out into a universe: its members checked first,
	// then its project (a new folder, its identity re-minted, attached - beside
	// the projects the universe serves already when the attach's member rules
	// accept it) and its tasks. A project the attach refuses is taken back:
	// nothing laid out stays.
	ApplySeed(ctx context.Context, in ApplySeedInput) (ApplySeedOutput, error)
	// ExportKeys seals the keys a manifest declares — and only those — with a
	// password: values read from each key's scope .env, never the process
	// environment.
	ExportKeys(ctx context.Context, in ExportKeysInput) (ExportKeysOutput, error)
	// ImportKeys opens a keys file with its password and writes each key into
	// its declared scope's .env (DryRun: only says what it would do).
	ImportKeys(ctx context.Context, in ImportKeysInput) (ImportKeysOutput, error)
	// ImportVault installs a published vault and imports its keys, opened with
	// its password, each into its scope's .env (as ImportKeys).
	ImportVault(ctx context.Context, in ImportVaultInput) (ImportKeysOutput, error)
	// KeySets says which sealed key sets (vaults) a solution provides, that
	// one team password opens them (tried when given, nothing written), and
	// the required keys still without a value that no opened set provides —
	// the universe's config check, for one form (solutions.md §6).
	KeySets(ctx context.Context, in KeySetsInput) (KeySetsOutput, error)
	// ImportKeySets opens every key set of a solution with one password and
	// imports them all into each of its universes — a set the password does
	// not open refuses the whole import, nothing written — then reports the
	// keys still missing, for one form.
	ImportKeySets(ctx context.Context, in ImportKeySetsInput) (ImportKeySetsOutput, error)
	// ProjectsFolder is the installation's projects folder: the one the
	// person chose, else the one proposed (<Documents>/Katyoti).
	ProjectsFolder(ctx context.Context) (ProjectsFolderOutput, error)
	// SetProjectsFolder chooses it: a full path outside the installation's
	// folder and its roots, created when missing, recorded in katyoti.yml.
	SetProjectsFolder(ctx context.Context, in SetProjectsFolderInput) (ProjectsFolderOutput, error)
	// EarlierProjects lists the projects the earlier installations of this
	// machine serve (katyoti-instances.yaml, then each one's universes and
	// their projects registries), for the person to pick from.
	EarlierProjects(ctx context.Context) (EarlierProjectsOutput, error)
	// ImportProject copies an earlier project into the projects folder and
	// migrates the copy to this release's data version - the original is
	// never written; a failed migration removes the copy - then attaches it
	// to a universe when one is named.
	ImportProject(ctx context.Context, in ImportProjectInput) (ImportProjectOutput, error)
	// EarlierInstallations lists the installations of this machine, each with
	// its size, last use and why it cannot be deleted (rule).
	EarlierInstallations(ctx context.Context) (EarlierInstallationsOutput, error)
	// DeleteInstallation answers what deleting an installation takes away and
	// the token that acknowledges it; given that token, deletes it - its
	// folders, its shortcuts, its registry entry.
	DeleteInstallation(ctx context.Context, in DeleteInstallationInput) (DeleteInstallationOutput, error)

	// ItemVersions lists an installed/available item's catalog versions,
	// newest-first (latest is [0]). Qualified by the item's installed marketplace
	// when it is installed.
	ItemVersions(ctx context.Context, in ItemVersionsInput) (ItemVersionsOutput, error)

	// SetVersion makes an item's given version current: install it (idempotent —
	// an already-placed version is not re-fetched, and newer versions are kept)
	// and, if the item is wired into the universe, repoint its pin. version ""
	// means latest — so this is update-to-latest, downgrade, and pin in one.
	// universeHome empty = the current workspace.
	SetVersion(ctx context.Context, in SetVersionInput) (Installed, error)

	// Configure → USED. Drives the installed item's own config surface.
	ConfigureItem(ctx context.Context, in ConfigureItemInput) (sidecarapi.ConfigCheckReport, error)
	Doctor(ctx context.Context, in DoctorInput) ([]ItemCheck, error)

	// GetItemConfig reads one of an installed item's config files whole (live,
	// or the bundled default) through the item's own `config get` verb, in the
	// named universe (empty = the current workspace — over KTSP that is ADMIN,
	// so a GUI acting on the universe it displays MUST pass its home).
	GetItemConfig(ctx context.Context, in GetItemConfigInput) (GetItemConfigOutput, error)
	// SetItemConfig writes one of an installed item's config files whole through
	// its own `config set` verb (the sidecar validates before writing; invalid
	// content is refused and nothing is written), then re-runs its config check.
	// This is the text-editor path; a schema-driven form produces the same call.
	SetItemConfig(ctx context.Context, in SetItemConfigInput) (SetItemConfigOutput, error)

	// WireIntoUniverse wires an installed item into a universe (AVAILABLE →
	// USED): AddSidecar (store-path pin) + configure. universeHome is the
	// workspace folder; empty = the current workspace. Returns the resolved home.
	WireIntoUniverse(ctx context.Context, in WireIntoUniverseInput) (WireIntoUniverseOutput, error)

	// Unwire removes an item from a universe (USED → AVAILABLE): drops its
	// SidecarSpec from Univers.yaml. universeHome empty = the current workspace.
	// Returns the resolved home. A no-op (not wired) is not an error.
	Unwire(ctx context.Context, in UnwireInput) (UnwireOutput, error)

	// Project attach/detach + universe create/list are owned by the registry-backed UniverseService
	// (orchestrator), NOT the marketplace — the `universe` command resolves it
	// directly. (Removed from SetupService in universe-service-consolidation.)

	// InstallSolution installs a `kind: solution` (modules/katyoti/solutions.md §4): install pinned
	// members → create the universe from the captured wiring → configure. No
	// upgrade taken; secrets are re-collected (never captured). Returns the home.
	InstallSolution(ctx context.Context, in InstallSolutionInput) (InstallSolutionOutput, error)

	// RepinSolution moves the universes a solution installed to another of its
	// versions: the pinned members installed, every universe rewired to them
	// and recording the version. Nothing applied once (config-sets, keys,
	// seed) is applied again. The update path's repin and its rollback.
	RepinSolution(ctx context.Context, in RepinSolutionInput) (RepinSolutionOutput, error)

	// PlanPrereqs plans the programs the sidecars of the named universes need
	// on this machine, read from each sidecar's config check: deduplicated
	// across sidecars at the highest version, required or optional, the recipe
	// chosen for this platform, a program no recipe installs here given its
	// prose fallback (solutions.md §10).
	PlanPrereqs(ctx context.Context, in PrereqsPlanInput) (PrereqsPlan, error)
	// InstallPrereqs installs programs of that plan (none named: every required
	// one missing) in the person's own profile, the administrator steps
	// together behind one prompt, then reads PATH again and checks again.
	InstallPrereqs(ctx context.Context, in PrereqsInstallInput) (PrereqsInstallOutput, error)
}

// InfraService is the control-plane MUTATION surface, and it is deliberately
// NOT part of SetupService.
//
// SetupService is published over KTSP so the GUI can reach it. These three
// replace katyoti.exe, ktw-hub.exe and ktw-gui.exe — and a hub serving a KTSP
// call would be replacing its own binary, and the GUI's, while both run. The
// files survive (rename-aside), but the processes keep executing the old code,
// so any success reported over the wire is true about the disk and false about
// the caller.
//
// Keeping them off the wire makes that unrepresentable rather than merely
// discouraged: the katyoti launcher resolves this interface IN-PROCESS, which is
// the only context where nothing being updated is running. The GUI offers
// `Update & restart`, which hands off to `katyoti gui --update`.
//
// Reading is fine over KTSP — InfraVersions / InfraOutdated stay on SetupService.
type InfraService interface {
	// SetupMachine is FIRST RUN in one call: install the control plane from the
	// channel, remember that channel (with its trust), and subscribe the
	// marketplace published beside it. Idempotent -- safe as a repair command.
	//
	// It does not launch the GUI; the CLI does that after it returns.
	SetupMachine(ctx context.Context) (SetupMachineOutput, error)

	// InstallInfra bootstraps the control-plane set from the infra channel:
	// install the channel's current version of each into the store, then place
	// it at the control-plane location (a running copy is swapped rename-aside).
	// Idempotent. Binaries the channel does not serve FOR THIS PLATFORM are
	// reported in Skipped rather than failing the whole bootstrap.
	InstallInfra(ctx context.Context) (InstallInfraOutput, error)

	// SelfUpdate brings the control-plane binaries to the version the channel
	// points at. A no-op when everything already matches, so it is safe to call
	// on every launch.
	SelfUpdate(ctx context.Context) ([]Update, error)

	// SetInfraVersion rolls the set to an EXPLICIT channel version — the
	// rollback verb. Pinning a single infra binary is refused; moving the whole
	// set is what recovering from a bad publish requires, and it keeps the set
	// coherent.
	SetInfraVersion(ctx context.Context, version string) ([]Update, error)

	// RepinPrograms points every universe of this installation at its own
	// programs: a member whose binary lies in no installation (a checkout's
	// bin/) is re-pinned to the same-named program of the control plane or
	// bin/dev, and the available-items registry is scanned again, keeping only
	// what lies in the installation. Run by make install after it copies the
	// programs; refused outside an installation.
	RepinPrograms(ctx context.Context, in RepinProgramsInput) (RepinProgramsOutput, error)

	// DiagnoseInstallation names what is broken in this installation, reading
	// its own files only (no hub, no network): a program gone or replaced
	// against the checksum it was installed with, a registry row to a vanished
	// path (katyoti-instances.yaml, universes.yaml, sidecars.yaml,
	// marketplaces.yaml), a row a test left in them. It changes nothing.
	DiagnoseInstallation(ctx context.Context) (InstallationDiagnosis, error)

	// RepairInstallation repairs what DiagnoseInstallation names, after
	// copying every file it changes into a backup folder of the installation:
	// a registry row to a vanished path or left by a test is removed, a
	// control-plane program replaced or gone is put back from its verified
	// store copy. What it cannot repair (a store program replaced, an
	// unreadable registry) is left, saying what the person does. A dry run
	// changes nothing.
	RepairInstallation(ctx context.Context, in RepairInstallationInput) (InstallationRepair, error)
}

// RepinProgramsInput asks for the repin of an installation's universes.
type RepinProgramsInput struct {
	DryRun bool `json:"dryRun,omitempty" cli:"dry-run" desc:"Say what would move and change nothing"`
}

// ProgramRepin is one member re-pinned (or that would be).
type ProgramRepin struct {
	Universe string `json:"universe"`
	Member   string `json:"member"`
	From     string `json:"from"`
	To       string `json:"to,omitempty" desc:"Empty: the installation has no such program"`
}

// RepinProgramsOutput says what moved, what could not, and what the registry
// discovered.
type RepinProgramsOutput struct {
	Installation string         `json:"installation"`
	Moved        []ProgramRepin `json:"moved,omitempty"`
	Missing      []ProgramRepin `json:"missing,omitempty"`
	Discovered   []string       `json:"discovered,omitempty" desc:"The programs the registry lists as discovered in the installation"`
}

// ShowConfigSetInput selects one config-set by name.
type ShowConfigSetInput struct {
	Name string `json:"name" cli:"arg" desc:"Config-set name" jsonschema:"A config-set item name" required:"true"`
}

// ConfigSetKey mirrors one entry of the bundle's keys.yaml.
type ConfigSetKey struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

// ShowConfigSetOutput describes a config-set. Keys are only readable once the
// bundle is INSTALLED (they live inside it), so Installed reports whether the
// key list is authoritative or simply not available yet.
type ShowConfigSetOutput struct {
	Name        string         `json:"name"`
	DisplayName string         `json:"displayName,omitempty"`
	Description string         `json:"description,omitempty"`
	Marketplace string         `json:"marketplace,omitempty"`
	Versions    []string       `json:"versions,omitempty"`
	Installed   bool           `json:"installed"`
	Version     string         `json:"version,omitempty"`
	Keys        []ConfigSetKey `json:"keys,omitempty"`
	Templates   []string       `json:"templates,omitempty"`
}

// ApplyConfigSetInput applies one config-set to one universe.
type ApplyConfigSetInput struct {
	// Name is the config-set to apply; empty = the universe's own key set (the
	// keys its members declare): nothing installed, the values saved the same way.
	Name string `json:"name" cli:"arg" desc:"Config-set to apply (name[@version]); empty: the universe's own keys"`
	// Version pins which version to apply; empty = latest.
	Version string `json:"version,omitempty" cli:"version" desc:"Version to apply (default: latest)"`
	// UniverseHome is the target universe's folder; empty = the current workspace.
	UniverseHome string `json:"universeHome,omitempty" cli:"universe" desc:"Target universe folder (default: current workspace)"`
	// Force overwrites templates that already exist instead of skipping them.
	Force bool `json:"force,omitempty" cli:"force" desc:"Overwrite existing config templates"`
	// Values are collected key values to record.
	Values map[string]string `json:"values,omitempty"`
	// DryRun reports the keys, their state and what is missing, and writes
	// NOTHING — no template, no member, no .env: the first call of a caller
	// that collects values (a prompt, a form).
	DryRun bool `json:"dryRun,omitempty" cli:"dry-run" desc:"Report the keys and what is missing; write nothing"`
}

// ShowSeedInput selects one seed by name.
type ShowSeedInput struct {
	Name string `json:"name" cli:"arg" desc:"Seed name" jsonschema:"A seed item name" required:"true"`
}

// ShowSeedOutput describes a seed: its versions, and the members its latest
// version was made with (name@version).
type ShowSeedOutput struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName,omitempty"`
	Description string   `json:"description,omitempty"`
	Marketplace string   `json:"marketplace,omitempty"`
	Versions    []string `json:"versions,omitempty"`
	Requires    []string `json:"requires,omitempty"`
	// Demo: its content is demo data (Item.Demo).
	Demo bool `json:"demo,omitempty"`
}

// ApplySeedInput applies one seed to one universe.
type ApplySeedInput struct {
	Name    string `json:"name" cli:"arg" desc:"Seed to apply" required:"true"`
	Version string `json:"version,omitempty" cli:"version" desc:"Version to apply (default: latest)"`
	// UniverseHome is the universe: a registered name, a new name (created
	// under the global universes root) or a folder. Required.
	UniverseHome string `json:"universeHome" cli:"universe" desc:"Target universe: a registered name, a new name or a folder" required:"true"`
	// ProjectDir is where the project is laid out; empty is <universe
	// name>-project beside the universe. A folder that already holds files is
	// refused (name another): applying a seed never overwrites a project.
	ProjectDir string `json:"projectDir,omitempty" cli:"project" desc:"Project folder (default: <universe name>-project beside the universe; one holding files is refused)"`
}

// ApplySeedOutput reports what applying a seed did.
type ApplySeedOutput struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	UniverseHome string `json:"universeHome"`
	Project      string `json:"project" desc:"The project folder laid out and attached"`
	// ProjectID is the project object's id after the re-mint (the folder's
	// name); empty when the seed carried no project object.
	ProjectID string `json:"projectId,omitempty"`
	Tasks     int    `json:"tasks,omitempty" desc:"Task files given to the universe"`
	// Differs names members the universe has at another version than the
	// seed was made with (name: have → seed's) — a warning.
	Differs []string `json:"differs,omitempty"`
	// NotRecorded is why the project laid out could not be recorded as
	// created on this PC - a warning: it is attached, but
	// would be refused later beside other members of a universe.
	NotRecorded string `json:"notRecorded,omitempty" desc:"Why the project could not be recorded as created on this PC"`
}

// ExportKeysInput selects what a keys file holds: the keys ONE manifest
// declares (a config-set's keys.yaml, or a keys.yaml file) — never every key of
// a .env, so a key added by hand never leaves by inadvertence.
type ExportKeysInput struct {
	UniverseHome string `json:"universeHome" cli:"universe" desc:"Universe whose .env holds the universe-scoped values (a registered name or a folder)" required:"true"`
	ConfigSet    string `json:"configSet,omitempty" cli:"config-set" desc:"Export the keys this config-set declares (name[@version])"`
	Manifest     string `json:"manifest,omitempty" cli:"manifest" desc:"Export the keys a keys.yaml file declares"`
	Password     string `json:"password" desc:"The password that seals the file (12 characters at least)"`
	// SkipMissing leaves out a declared key with no value instead of refusing.
	SkipMissing bool `json:"skipMissing,omitempty" cli:"skip-missing" desc:"Leave out declared keys that have no value (default: refuse)"`
	// Preview only lists what would be sealed (names and scopes), seals nothing.
	Preview bool `json:"preview,omitempty"`
}

// KeyEntry is one key a keys file holds or would hold — never its value.
type KeyEntry struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
	// State, on import: new (not set yet), same (already that value), differs
	// (another value is set — replaced only with Overwrite), written.
	State string `json:"state,omitempty"`
}

// ExportKeysOutput is the sealed file and what it holds.
type ExportKeysOutput struct {
	File    []byte     `json:"file,omitempty" desc:"The sealed keys file (empty on a preview)"`
	Keys    []KeyEntry `json:"keys"`
	Skipped []string   `json:"skipped,omitempty" desc:"Declared keys left out: no value"`
}

// ImportKeysInput opens a keys file into a universe.
type ImportKeysInput struct {
	UniverseHome string `json:"universeHome" cli:"universe" desc:"Universe whose .env receives the universe-scoped keys (a registered name or a folder)" required:"true"`
	File         []byte `json:"file" desc:"The sealed keys file"`
	Password     string `json:"password" desc:"Its password"`
	// Overwrite replaces a key already set to another value (default: kept).
	Overwrite bool `json:"overwrite,omitempty" cli:"overwrite" desc:"Replace keys already set to another value"`
	DryRun    bool `json:"dryRun,omitempty" cli:"dry-run" desc:"Say what would be written; write nothing"`
}

// ImportVaultInput names a published vault and the universe its keys go to.
type ImportVaultInput struct {
	Name         string `json:"name" cli:"arg" desc:"The vault (name[@version])" required:"true"`
	UniverseHome string `json:"universeHome" cli:"universe" desc:"Universe whose .env receives the universe-scoped keys (a registered name or a folder)" required:"true"`
	Password     string `json:"password" desc:"The vault's password"`
	Overwrite    bool   `json:"overwrite,omitempty" cli:"overwrite" desc:"Replace keys already set to another value"`
	DryRun       bool   `json:"dryRun,omitempty" cli:"dry-run" desc:"Say what would be written; write nothing"`
}

// ImportKeysOutput says what the import did (or would do), key by key.
type ImportKeysOutput struct {
	Keys    []KeyEntry `json:"keys"`
	Written []string   `json:"written,omitempty"`
	// Restarts are the running members restarted because they read a key
	// written.
	Restarts []MemberRestart `json:"restarts,omitempty"`
}

// MemberRestart is a running member of a universe restarted through its hub
// because it reads a key just saved, so it uses the key without the universe
// being restarted; Error says why it was not.
type MemberRestart struct {
	Universe string   `json:"universe" desc:"The universe's home"`
	Member   string   `json:"member" desc:"The member (its Univers.yaml name)"`
	Keys     []string `json:"keys" desc:"The keys saved that it reads"`
	Error    string   `json:"error,omitempty" desc:"Why it was not restarted"`
}

// ConfigSetKeyState is one aggregated key plus what is already known about it.
type ConfigSetKeyState struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
	Scope       string `json:"scope,omitempty"`
	// Source says where the requirement came from: the config-set manifest, or a
	// specific sidecar's own config check ("sidecar:<name>").
	Source string `json:"source,omitempty"`
	// Current is the value already in the scoped .env — shown as the default.
	// Redacted for secrets: the caller must never be handed a secret it did not
	// already have.
	Current string `json:"current,omitempty"`
	Present bool   `json:"present"`
	// Optional keys may stay unset; every other key of the config-set's
	// manifest must have a value before it is applied.
	Optional bool `json:"optional,omitempty"`
}

// ApplyConfigSetOutput reports what apply did and what it still needs.
type ApplyConfigSetOutput struct {
	Name             string              `json:"name"`
	Version          string              `json:"version"`
	UniverseHome     string              `json:"universeHome"`
	TemplatesCopied  []string            `json:"templatesCopied,omitempty"`
	TemplatesSkipped []string            `json:"templatesSkipped,omitempty"`
	Keys             []ConfigSetKeyState `json:"keys,omitempty"`
	// Missing names the keys with no value yet — the caller's prompt list.
	Missing []string `json:"missing,omitempty"`
	// Blocking names the NON-optional keys of the config-set's manifest still
	// without a value: a real apply is refused, before anything is written,
	// while one is left.
	Blocking []string `json:"blocking,omitempty"`
	// EnvWritten names the .env files actually modified (may be two: the
	// universe's and the global one, when keys of both scopes were supplied).
	EnvWritten []string `json:"envWritten,omitempty"`
	// KeysWritten names the keys whose value actually changed this call.
	KeysWritten []string `json:"keysWritten,omitempty"`
	// MembersAdded names sidecars this apply wired into the universe, as
	// `name@version`, because the config-set declared them and they were not
	// members yet.
	MembersAdded []string `json:"membersAdded,omitempty"`
	// MembersKept names declared sidecars that were ALREADY members and were
	// therefore left exactly as they were — same version, same wiring.
	MembersKept []string `json:"membersKept,omitempty"`
	// Restarts are the running members restarted because they read a key
	// written.
	Restarts []MemberRestart `json:"restarts,omitempty"`
}

// SetupMachineOutput reports what first run did. MarketplaceError is a
// WARNING, not a failure: the control plane installs fine without a catalog,
// and failing a mostly-successful bootstrap would be worse than saying so.
type SetupMachineOutput struct {
	Channel          string   `json:"channel"`
	Version          string   `json:"version,omitempty"`
	Installed        []string `json:"installed,omitempty"`
	Skipped          []string `json:"skipped,omitempty"`
	Pinned           bool     `json:"pinned,omitempty"`
	Marketplace      string   `json:"marketplace,omitempty"`
	MarketplaceError string   `json:"marketplaceError,omitempty"`
	// Solution is the solution the channel names for a first install
	// (<solution>@<version>); the setup command installs it and opens on its
	// main universe.
	Solution string `json:"solution,omitempty"`
	// GUIRuntime is what setup did about the GUI's runtime (WebView2 on
	// Windows): present, installed, or nothing where none is needed;
	// GUIRuntimeError why it could not.
	GUIRuntime      string `json:"guiRuntime,omitempty"`
	GUIRuntimeError string `json:"guiRuntimeError,omitempty"`
}

// InfraVersionsOutput is the infra channel's reachable set: the version the
// pointer names, and the rollback target it retains. The channel keeps exactly
// two, because nothing pins infra -- anything older is bytes no command can
// resolve. Installed is what THIS machine is running, so a caller can render
// "you are here" without a second call.
type InfraVersionsOutput struct {
	Current   string `json:"current"`
	Previous  string `json:"previous,omitempty"`
	Installed string `json:"installed,omitempty"`
	Channel   string `json:"channel"`
	// Binaries is the control-plane set itself, so a caller can SAY which
	// binaries move together instead of restating the list. Every restatement
	// has drifted: the CLI help, `versions`, the publish script and the GUI strip
	// each carried their own copy, and adding ktw-univers left three of them
	// quietly wrong — still naming three binaries for a set of four.
	Binaries []string `json:"binaries,omitempty"`
	// DevBuild says the control plane is a development build - make dev placed
	// it from a checkout, the dev channel - not a set from the channel: Installed
	// is then its commit, and it is never behind the channel.
	DevBuild bool `json:"devBuild,omitempty"`
}

// InstallInfraOutput reports a control-plane bootstrap: the binaries placed at
// the control-plane location, plus any infra binaries no subscribed catalog serves.
type InstallInfraOutput struct {
	Installed []Installed `json:"installed"`
	Skipped   []string    `json:"skipped,omitempty"`
}

// ItemCheck pairs an installed item's name with its config-check report (Doctor).
type ItemCheck struct {
	Name   string                       `json:"name"`
	Report sidecarapi.ConfigCheckReport `json:"report"`
}
