// SPDX-License-Identifier: Apache-2.0

package api

import "github.com/katyoti/katyoti-fwk/internal/core/meta"

// SetupServiceDef declares the marketplace management surface for KTSP/HTTP
// exposure (served in the admin universe — (modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md)).
// AutoKTSPContainer builds slots from the SetupService impl via reflection; only
// KTSP-shaped methods (ctx, InputStruct)(Output, error) are dispatchable — the
// wire/unwire/bootstrap orchestration methods are not yet normalized and are
// served elsewhere (gui-pure-ktsp-client). Auto-CLI is suppressed — the hand-
// written `katyoti` commands own the CLI surface.
// The control-plane MUTATION verbs (InstallInfra / SelfUpdate /
// SetInfraVersion, SetupMachine) are deliberately ABSENT from this def, and from
// SetupService itself. This def drives the KTSP surface, so listing them
// here would advertise over the wire an operation that replaces the binary
// of the process answering the call. They live on InfraService, registered
// in-process only. See internal/extensions/marketplace/api/service.go.
var SetupServiceDef = meta.ServiceDef{
	Name:    "Marketplace",
	BusName: "marketplace",
	Short:   "Marketplace management — subscribe, browse, install/update, configure",
	Long:    `The katyoti marketplace engine: subscribe to marketplaces (catalog URLs), browse the available catalog, install/update items into the versioned store, and drive each installed sidecar's own config check. Served in the admin universe; the GUI + katyoti CLI are pure KTSP clients. Authoring/publishing is NOT here (that is ktw-compose).`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		// subscription
		"ListMarketplaces":      meta.MethodLong("List subscribed marketplaces", "The effective list (beside-exe seed ∪ user overlay), with enabled + trust state."),
		"AddMarketplace":        meta.MethodLong("Subscribe to (or update) a marketplace", "Adds a catalog URL to the user overlay; PubKey pins a minisign trust root (implies trust=pinned)."),
		"RemoveMarketplace":     meta.MethodLong("Unsubscribe a marketplace", "Removes the subscription from the user overlay (a seed entry is recorded disabled)."),
		"SetMarketplaceEnabled": meta.MethodLong("Enable/disable a marketplace", "Toggles a subscription without removing it."),
		"SetMarketplaceChannel": meta.MethodLong("Follow a marketplace's stable or canary channel", "stable (end users - the default) sees promoted releases only; canary (alpha users) sees the release candidates too."),
		"RefreshCatalog":        meta.MethodLong("Fetch + cache catalogs", "Fetches one marketplace's catalog (or all enabled when Name is empty); a pinned marketplace's signature is verified."),
		// catalog read
		"SearchItems":  meta.MethodLong("Search catalog items", "Free-text search across enabled catalogs; empty query returns all."),
		"GetItem":      meta.MethodLong("Get one catalog item", "Full metadata (versions, assets, requires) for a named item."),
		"ItemVersions": meta.MethodLong("List an item's versions", "Available versions, newest first."),
		// install → available
		"Install":       long(meta.MethodLong("Install an item into the versioned store", "Fetches + sha256-verifies the OS/arch asset into the shared version store (→ available).")),
		"Uninstall":     meta.MethodLong("Uninstall an item", "Removes all store versions + the available record; refuses a wired item."),
		"ListInstalled": meta.MethodLong("List installed items", "The available-items registry on this machine."),
		// update
		"Outdated":             meta.MethodLong("Show outdated items", "Installed items with a newer version in their marketplace."),
		"Update":               long(meta.MethodLong("Update installed items", "Update named items (or all outdated when empty) to latest; repoints wired items.")),
		"ListConfigSets":       meta.MethodLong("List config-sets", "The `kind: config` items offered across enabled marketplaces."),
		"ShowConfigSet":        meta.MethodLong("Show a config-set", "Its catalog versions plus, once installed, the keys its bundle declares."),
		"ListSeeds":            meta.MethodLong("List seeds", "The `kind: seed` items offered across enabled marketplaces: the content of a universe (a project, its tasks)."),
		"ShowSeed":             meta.MethodLong("Show a seed", "Its catalog versions and the members its latest version was made with."),
		"ApplySeed":            meta.MethodLong("Apply a seed to a universe", "Checks the universe has the members the seed was made with (a missing one refuses, a different version is reported), then lays its project out into a new folder (its identity re-minted), attaches it, and gives the universe its tasks."),
		"ExportKeys":           meta.MethodLong("Export keys sealed with a password", "Seals the keys ONE manifest declares (a config-set's keys.yaml or a keys.yaml file) — values read from each key's scope .env, never the process environment — into a file only the password opens. Preview lists names and scopes, never values."),
		"ImportVault":          meta.MethodLong("Import a published vault", "Installs the vault (a sealed keys file published in a marketplace), opens it with its password and writes each key into its declared scope's .env, as ImportKeys."),
		"KeySets":              {Short: "The key sets a solution provides, and the keys still missing", Long: "Its sealed key sets (vaults) — one team password opens them all, tried when given with nothing written — then the required keys still without a value that no opened set provides, as the universe's config check reports them (each config-set's report, each wired sidecar's own check): the one form.", TimeoutS: 300},
		"ImportKeySets":        {Short: "Import every key set of a solution with one password", Long: "Opens every key set (vault) of the solution with the one password FIRST — a set it does not open refuses the whole import, nothing written — then writes each key into its scope's .env in every universe of the solution, and reports the keys still missing for one form.", TimeoutS: 300},
		"ProjectsFolder":       meta.MethodLong("The installation's projects folder", "Where every project of the person lives: the folder chosen at first start, else the one proposed (<Documents>/Katyoti); never inside the installation."),
		"SetProjectsFolder":    meta.MethodLong("Choose the installation's projects folder", "A full path outside the installation's folder and its roots (config, data, cache) - refused inside them, where an uninstall would take the projects - created when missing and recorded in the installation's katyoti.yml."),
		"EarlierProjects":      meta.MethodLong("The projects of earlier installations", "Every project a universe of an earlier installation of this machine attaches (katyoti-instances.yaml, then its universes and their projects registries), with its data version and whether it was imported already; an installation that cannot be read is skipped with why."),
		"ImportProject":        meta.MethodLong("Import an earlier project as a copy", "Copies the project into the projects folder under its name (a name already taken is refused), migrates the copy to this release's data version - the original is only read, and a failed migration removes the copy - then attaches it to the universe named, if any."),
		"EarlierInstallations": meta.MethodLong("The installations of this machine", "Every installation katyoti-instances.yaml lists, with its own name, version, folder, size and last use, whether it is the one answering, and why it cannot be deleted (the one answering, the last one, one whose projects folder lies inside it, one sharing a folder with another)."),
		"DeleteInstallation":   meta.MethodLong("Delete an earlier installation once acknowledged", "Without ack: what deleting the installation takes away - each folder with its size and last use, its shortcuts - and the token that acknowledges it; nothing is touched. With that token: its folders, shortcuts and registry entry go. Refused for the installation answering, one whose projects folder lies inside it, one sharing a folder with another, and the last one unless delete_last."),
		"ImportKeys":           meta.MethodLong("Import a sealed keys file", "Opens it with its password and writes each key into its declared scope's .env; a key already set to another value is kept unless overwrite. DryRun says what it would do."),
		"ApplyConfigSet":       meta.MethodLong("Apply a config-set to a universe", "Copies its config templates in and records supplied key values into the scoped .env; returns every aggregated key (config-set manifest UNION each wired sidecar's own config-check needs) and which are still missing. Does not prompt — the caller collects and calls again."),
		"SetVersion":           long(meta.MethodLong("Pin/downgrade an item to a version", "Install a specific version (idempotent) + repoint the universe pin if wired; empty version = latest.")),
		// configure
		"ConfigureItem": meta.MethodLong("Configure an installed sidecar", "Drives the sidecar's own `config check` — reports env/auth/program/config items with next-step hints."),
		"Doctor":        {Short: "Aggregate config check", Long: "Runs the config check across all installed items — each item's binary started once, in parallel.", TimeoutS: 300, LongRunning: true},
		"GetItemConfig": meta.MethodLong("Read an installed sidecar's config file", "One config file whole (live, or the bundled default) via the sidecar's own `config get`, in the named universe."),
		"SetItemConfig": meta.MethodLong("Write an installed sidecar's config file", "Replaces one config file whole via the sidecar's own `config set` (validated first; invalid content is refused), then re-runs its config check."),
		// enable orchestration (AVAILABLE↔USED). NOTE: these still address the
		// universe by folder path + write Univers.yaml directly; the write-delegation
		// to the universe owner (single-writer) lands with gui-pure-ktsp-client.
		"WireIntoUniverse": meta.MethodLong("Wire an installed item into a universe (AVAILABLE→USED)", "Adds the item to the universe's Univers.yaml (version-pinned) + configures it."),
		"Unwire":           meta.MethodLong("Remove an item from a universe (USED→AVAILABLE)", "Drops the item's SidecarSpec from Univers.yaml. A no-op when not wired."),
		// prereqs: the programs the sidecars need (solutions.md §10)
		"PlanPrereqs":     {Short: "Plan the programs a universe's sidecars need", Long: "Reads each wired sidecar's config check: one row per program, deduplicated across sidecars at the highest version, required or optional (an optional one is offered, not preselected), the install recipe chosen for this platform, user or administrator scope; a program no recipe installs here carries its prose fallback.", TimeoutS: 300},
		"InstallPrereqs":  {Short: "Install the programs a universe's sidecars need", Long: "Installs the named programs (none: every required one missing) in the person's own profile, the administrator steps together behind one prompt; a download is refused unless its sha256 matches. Then reads PATH again and runs the sidecars' checks again.", TimeoutS: 1800, LongRunning: true},
		"InstallSolution": long(meta.MethodLong("Install a solution", "Opens its vaults and checks every key of its config-sets before anything installs → installs its pinned members → recreates the universe wiring → imports the vaults' keys → applies its config-sets → lays its seed out → places its documents in the project → configures.")),
		"RepinSolution":   meta.MethodLong("Move a solution's universes to another of its versions", "Installs that version's pinned members and rewires every universe the solution installed to exactly them, each recording the version; config-sets, keys and the seed are not applied again. With dry-run, says which members move. The repin of katyoti solution update, and its rollback."),
	},
}

// long declares a method that runs longer than a person waits for
// : run from the action surface it is a method run that owns
// a panel, and it reports its steps into it (internal/core/report.go).
func long(d meta.MethodDef) meta.MethodDef {
	d.LongRunning = true
	return d
}
