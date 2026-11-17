// SPDX-License-Identifier: Apache-2.0

// Package api defines the katyoti marketplace/installer engine contract
// (the SetupService) and its data types. Both the katyoti CLI and ktw-gui
// resolve SetupService in-process. See modules/katyoti/spec.md.
package api

import (
	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// ItemKind discriminates a catalog item.
type ItemKind string

const (
	KindSidecar ItemKind = "sidecar" // a compiled ktw-* binary
	KindContent ItemKind = "content" // a files-only sidecar (folder, runs on ktw-host)
	// KindSolution is a fixed version of a working installation: the infra
	// version, the sidecars at their versions, its config-sets, its seed, its
	// vaults — and its documents (README, Quoi de neuf…) per language.
	// Installed whole by `katyoti solution install`.
	KindSolution ItemKind = "solution"
	KindConfig   ItemKind = "config" // a config-set: templates + a keys.yaml manifest
	// KindSeed is a universe's content: a project — its CWM objects and the
	// whole project folder — and the universe's own tasks. Applied to a
	// universe like a config-set (it adds a project); pinned by a solution.
	KindSeed ItemKind = "seed"
	// KindVault is a sealed keys file (`katyoti keys export`): published as it
	// is, opened only with its password, never by the marketplace.
	KindVault ItemKind = "vault"
)

// Trust level a subscriber attaches to a marketplace (team-marketplace.md §1).
// It is a per-SUBSCRIPTION policy (the consumer's choice), not a property of the
// catalog. Empty is treated as TrustNone.
const (
	TrustNone   = "none"   // local/personal — install with only the sha256 gate
	TrustPinned = "pinned" // team/public — the index signature MUST verify (fail closed)
)

// Marketplace is one subscribed catalog source. Source is computed
// ("seed" | "user"), not persisted.
type Marketplace struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
	Source  string `json:"source,omitempty"`
	// Trust + PubKey implement the install-time provenance gate. PubKey is the
	// pinned trust root (a minisign base64 key line), distributed out-of-band.
	// For TrustPinned, the catalog's `<index>.minisig` MUST verify against
	// PubKey before the catalog is used for any install.
	Trust  string `json:"trust,omitempty"`
	PubKey string `json:"pubKey,omitempty"`
	// Channel is what the installation follows: stable (the default - end
	// users) sees promoted releases only; canary (alpha users) sees the
	// candidates too (decision).
	Channel string `json:"channel,omitempty"`
}

// The release channels of a marketplace: an installation follows one.
const (
	ChannelStable = "stable"
	ChannelCanary = "canary"
)

// Catalog is a fetched marketplace index (katyoti-marketplace.yaml).
type Catalog struct {
	APIVersion  int         `yaml:"apiVersion,omitempty" json:"apiVersion,omitempty"`
	Marketplace CatalogMeta `yaml:"marketplace" json:"marketplace"`
	Items       []Item      `yaml:"items" json:"items"`
}

// CatalogMeta is the marketplace's own identity block.
type CatalogMeta struct {
	Name        string `yaml:"name" json:"name"`
	DisplayName string `yaml:"displayName,omitempty" json:"displayName,omitempty"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// Item is one installable catalog entry.
type Item struct {
	Name        string   `yaml:"name" json:"name"`
	Kind        ItemKind `yaml:"kind" json:"kind"`
	DisplayName string   `yaml:"displayName,omitempty" json:"displayName,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Icon        string   `yaml:"icon,omitempty" json:"icon,omitempty"`
	Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	// Demo marks a seed whose content is demo data, declared by
	// its publisher (build-seed --demo, a case's project.demo); the
	// Marketplace draws it as a « Démo » badge.
	Demo     bool          `yaml:"demo,omitempty" json:"demo,omitempty"`
	Versions []ItemVersion `yaml:"versions,omitempty" json:"versions,omitempty"`
}

// ItemVersion is one published version of an item.
type ItemVersion struct {
	Version string `yaml:"version" json:"version"`
	// Channel is the channel it is published on: stable when empty; canary
	// for a release candidate, seen only by the installations following it.
	Channel   string   `yaml:"channel,omitempty" json:"channel,omitempty"`
	Immutable bool     `yaml:"immutable,omitempty" json:"immutable,omitempty"`
	Signature string   `yaml:"signature,omitempty" json:"signature,omitempty"`
	Assets    []Asset  `yaml:"assets,omitempty" json:"assets,omitempty"`
	Requires  Requires `yaml:"requires,omitempty" json:"requires,omitempty"`
	DependsOn []string `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty"`
	// Includes are a solution's members, pinned: every sidecar of every one of
	// its universes, once (what is downloaded, and what Requires sums).
	Includes []PinnedRef `yaml:"includes,omitempty" json:"includes,omitempty"`
	// Universes are the universes a solution composes, exactly one of them
	// Main (solution_universes.go). An older catalog's single `universe:`
	// still loads, as the one main universe of this list.
	Universes []SolutionUniverse `yaml:"universes,omitempty" json:"universes,omitempty"`
	// Universe is the older single-universe wiring: read only, moved into
	// Universes when a catalog is decoded, never written.
	Universe *SolutionUniverse `yaml:"universe,omitempty" json:"universe,omitempty"`
	// ConfigSets are the `kind: config` items a solution applies to its MAIN
	// universe, in order, so installing it yields a CONFIGURED universe (each
	// other universe carries its own).
	ConfigSets []PinnedRef `yaml:"configSets,omitempty" json:"configSets,omitempty"`
	// Seed pins the `kind: seed` the MAIN universe gets — its project and
	// tasks — so it is ready to use. One: a universe serves one project.
	Seed *PinnedRef `yaml:"seed,omitempty" json:"seed,omitempty"`
	// Vaults are the `kind: vault` items whose keys a solution imports — its
	// key sets, opened together with the one team password (KeySets).
	Vaults []PinnedRef `yaml:"vaults,omitempty" json:"vaults,omitempty"`
	// Documents are a solution's documents (README, Quoi de neuf…), per
	// language, served from `solutions/<name>/<version>/docs/<lang>/`.
	Documents []Document `yaml:"documents,omitempty" json:"documents,omitempty"`
	// Infra is the infra (control plane) version a solution was made with —
	// the version of the channel published beside its catalog at snapshot.
	Infra string `yaml:"infra,omitempty" json:"infra,omitempty"`
}

// Document is one document a solution delivers, in one language.
type Document struct {
	Lang   string `yaml:"lang" json:"lang"`
	Name   string `yaml:"name" json:"name"` // its file name, e.g. readme.md
	URL    string `yaml:"url" json:"url"`
	Sha256 string `yaml:"sha256,omitempty" json:"sha256,omitempty"`
}

// SolutionUniverse is one universe of a `kind: solution` item version: its
// captured Univers.yaml wiring (modules/katyoti/solutions.md §2), its
// config-sets and its seed. It carries the universe's identity + membership;
// secrets are NEVER captured (bootstrap re-collects them via the configure
// engine). The binary paths are re-derived locally at bootstrap (they are
// machine-specific store paths), so SolutionSidecar is mostly identity +
// custom args.
type SolutionUniverse struct {
	// Name names the universe within its solution; a solution of several
	// universes names each (the main one installs under the universe the
	// person names, each other one under <that name>-<Name>).
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
	// Main is the universe that opens after install (katyoti.yml
	// default_universe): exactly one per solution.
	Main bool `yaml:"main,omitempty" json:"main,omitempty"`
	// ConfigSets and Seed are this universe's own; the main universe also
	// gets the version's configSets and seed.
	ConfigSets  []PinnedRef `yaml:"configSets,omitempty" json:"configSets,omitempty"`
	Seed        *PinnedRef  `yaml:"seed,omitempty" json:"seed,omitempty"`
	DisplayName string      `yaml:"displayName,omitempty" json:"displayName,omitempty"`
	Icon        string      `yaml:"icon,omitempty" json:"icon,omitempty"`
	// Sidecars are the universe's members, by name, each pinned in the
	// version's Includes; a solution of one universe takes every member.
	Sidecars []SolutionSidecar `yaml:"sidecars,omitempty" json:"sidecars,omitempty"`
	// GUI is what the universe's GUI hides and lists first (Univers.yaml
	// `gui:`, cenv.UniverseGUI), written into the universe the solution
	// installs: an architect's solution shows only what an architect needs.
	GUI *cenv.UniverseGUI `yaml:"gui,omitempty" json:"gui,omitempty"`
	// Tags are the universe's tags (Univers.yaml `tags:`, the universe-tag
	// allow-lists choosing what its GUI shows), written into the universe
	// the solution installs; the ordered priority tags are gui.first.
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
	// FirstGuide is the guide the universe starts with (Univers.yaml
	// `first_guide:`, a guide-recipe URI of one of its sidecars), written
	// into the universe the solution installs.
	FirstGuide string `yaml:"firstGuide,omitempty" json:"firstGuide,omitempty"`
}

// SolutionSidecar mirrors the fields of a universe SidecarSpec that are portable
// across machines (identity + transport). Binary/Args are captured for
// reference but re-derived from the local store at bootstrap.
type SolutionSidecar struct {
	Name      string   `yaml:"name" json:"name"`
	Transport string   `yaml:"transport,omitempty" json:"transport,omitempty"`
	Binary    string   `yaml:"binary,omitempty" json:"binary,omitempty"`
	Args      []string `yaml:"args,omitempty" json:"args,omitempty"`
}

// Asset is a downloadable artifact. For kind=sidecar there is one per OS/arch;
// for kind=content there is a single platform-agnostic folder bundle.
type Asset struct {
	OS        string `yaml:"os,omitempty" json:"os,omitempty"`
	Arch      string `yaml:"arch,omitempty" json:"arch,omitempty"`
	URL       string `yaml:"url" json:"url"`
	Sha256    string `yaml:"sha256,omitempty" json:"sha256,omitempty"`
	Signature string `yaml:"signature,omitempty" json:"signature,omitempty"`
	// Bundle is a sidecar's companion archive for this platform — files too
	// large to embed in the binary (ktw-display's draw.io editor). Installing
	// the binary unpacks it into `<name>-assets/` beside it.
	Bundle *AssetBundle `yaml:"bundle,omitempty" json:"bundle,omitempty"`
}

// URLs are every file the asset publishes: the binary and its bundle.
func (a Asset) URLs() []string {
	if a.Bundle == nil {
		return []string{a.URL}
	}
	return []string{a.URL, a.Bundle.URL}
}

// AssetBundle is a .tar.gz shipped with one platform's sidecar binary.
type AssetBundle struct {
	URL    string `yaml:"url" json:"url"`
	Sha256 string `yaml:"sha256,omitempty" json:"sha256,omitempty"`
}

// BundleSuffix names a sidecar's companion archive in a build folder:
// `<binary name without .exe>` + BundleSuffix. It unpacks into
// `<binary name>` + BundleDirSuffix beside the installed binary.
const (
	BundleSuffix    = "-assets.tar.gz"
	BundleDirSuffix = "-assets"
)

// Requires previews what configuring an item will need (mirrors APIKeyDef/
// ToolDef; the authoritative contract is the sidecar's own config check).
type Requires struct {
	// Members are the sidecars a seed was made with, at the versions it was
	// made with: applying it refuses a universe missing one, and reports a
	// different version (versions are not ordered — a dev build carries a git
	// revision — so a difference is a warning, never a refusal).
	Members []PinnedRef `yaml:"members,omitempty" json:"members,omitempty"`
	// Formats are the formats (kind → data-type major.minor) a sidecar
	// writes, copied from its describe at indexing. Installing it refuses —
	// before any download — when one is a newer MAJOR than the installing
	// platform reads (update the hub first); a newer minor installs.
	Formats map[string]string `yaml:"formats,omitempty" json:"formats,omitempty"`
	// Tools, APIKeys and Auth are the programs, keys and logins the item
	// needs, copied from its describe at indexing (a content item: from its
	// ktw-host.yaml `config:`); a solution's are the deduplicated sum of its
	// members' (requires.go). Shown before install; an old catalog that
	// listed bare names still loads.
	Tools   []ToolNeed  `yaml:"tools,omitempty" json:"tools,omitempty"`
	APIKeys []KeyNeed   `yaml:"apiKeys,omitempty" json:"apiKeys,omitempty"`
	Auth    []LoginNeed `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// PinnedRef pins a member item to an exact version (pack / release includes[]).
type PinnedRef struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

// ItemSummary is a search-result row (item + which marketplace served it).
type ItemSummary struct {
	Name        string   `json:"name"`
	Kind        ItemKind `json:"kind"`
	DisplayName string   `json:"displayName,omitempty"`
	Description string   `json:"description,omitempty"`
	Marketplace string   `json:"marketplace"`
	Latest      string   `json:"latest,omitempty"`
	// Demo: a seed whose content is demo data (Item.Demo).
	Demo bool `json:"demo,omitempty"`
}

// ItemRef is a name[@version] reference; empty Version means latest.
// Marketplace optionally qualifies the lookup when the same name is served by
// more than one subscribed marketplace; empty means "any" (an ambiguous bare
// name is refused rather than silently resolved to the first match).
type ItemRef struct {
	// Tagged like every other type in this API. Without tags these marshalled as
	// PascalCase on the wire — an inconsistency the generated TS client made
	// visible (InstallInput had `Name` while every sibling input had `name`).
	// Safe to add: Go decodes field names case-insensitively when there is no
	// exact match, so a payload using the old PascalCase keys still unmarshals.
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Marketplace string `json:"marketplace,omitempty"`
}

// Installed is one row of the machine-wide available-items registry.
type Installed struct {
	Name        string `json:"name"`
	Kind        string `json:"kind,omitempty"`
	Version     string `json:"version,omitempty"`
	Source      string `json:"source,omitempty"`
	Marketplace string `json:"marketplace,omitempty"`
	Sha256      string `json:"sha256,omitempty"`
	BinaryPath  string `json:"binaryPath,omitempty"`
	FolderPath  string `json:"folderPath,omitempty"` // content items: ktw-host --folder target
	Wired       bool   `json:"wired,omitempty"`      // USED: pinned by at least one registered universe
	// WiredIn names every registered universe that pins this item.
	//
	// `Wired` alone could not answer the question a caller actually has, which is
	// "is this in the universe I am LOOKING AT". It was computed against "the
	// current universe", and over KTSP that is always admin — so the GUI's
	// "In universe" badge and count silently described admin while the user had
	// a different universe open.
	WiredIn []string `json:"wiredIn,omitempty"`
	// Pins is the version each universe that wires this item runs it at, so
	// a universe on another version than the installed one is shown behind
	// : its Univers.yaml pins a store path, not the registry.
	Pins []UniversePin `json:"pins,omitempty"`
	// DevBuild is the installation's development build of the item - the
	// program make dev placed in its dev channel (<root>/bin/dev), which the
	// installation's universes run - when it has one. The item is then shown as
	// that build, never as behind a marketplace: make dev moves it.
	DevBuild *DevBuild `json:"devBuild,omitempty"`
	// ControlPlane marks one of the control-plane programs (katyoti, ktw-hub,
	// ktw-gui, ktw-univers): the Infra tab shows them, never the sidecars' list.
	ControlPlane bool `json:"controlPlane,omitempty"`
}

// DevBuild is a development build an installation holds: where it is and the
// commit it was built from (its version, "-dirty" when the checkout was modified).
type DevBuild struct {
	Version string `json:"version,omitempty" desc:"The commit the build was made from; -dirty when the checkout was modified"`
	Path    string `json:"path,omitempty"`
}

// UniversePin is the version one universe runs an item at.
type UniversePin struct {
	Universe string `json:"universe"`
	Version  string `json:"version,omitempty" desc:"The store version its pin names; empty when it runs a program outside the store (a development build)"`
}

// Update is an available-update row (installed vs. catalog).
type Update struct {
	Name        string `json:"name"`
	Installed   string `json:"installed"`
	Latest      string `json:"latest"`
	Marketplace string `json:"marketplace,omitempty"` // the marketplace the update comes from
	// Universes are the registered universes whose pin was repointed to Latest.
	// Reported so `update` can SAY what it moved: the pin lives in each
	// universe's Univers.yaml, and an update that changed the store without
	// naming the universes it touched is how "did that take effect?" became
	// unanswerable.
	Universes []string `json:"universes,omitempty"`
	// Behind are the universes installed from a solution that keep the
	// version they pin on purpose: each moves with its solution's update, or
	// by `katyoti universe wire <name> --universe <universe>`.
	Behind []string `json:"behind,omitempty"`
}

// ── KTSP-shaped I/O for the marketplace-subscription methods ────────────────
// (normalized so DiscoverMethods can expose them over KTSP — marketplace-service-ktsp)

// AddMarketplaceInput subscribes to (or updates) a marketplace.
type AddMarketplaceInput struct {
	Name   string `json:"name"             cli:"arg"    desc:"Marketplace name"                            jsonschema:"Short name for the subscription" required:"true"`
	URL    string `json:"url"              cli:"arg"    desc:"Catalog URL (http(s) or a local file path)"  jsonschema:"The marketplace catalog URL"     required:"true"`
	PubKey string `json:"pubKey,omitempty" cli:"pubkey" desc:"Pinned minisign public key (implies trust=pinned)"`
	Trust  string `json:"trust,omitempty"  cli:"trust"  desc:"Trust level: none | pinned (default: pinned when pubKey set, else none)"`
}

// AddMarketplaceOutput echoes the subscribed marketplace.
type AddMarketplaceOutput struct {
	Name string `json:"name" desc:"The subscribed marketplace name"`
}

// RemoveMarketplaceInput unsubscribes a marketplace.
type RemoveMarketplaceInput struct {
	Name string `json:"name" cli:"arg" desc:"Marketplace name to remove" jsonschema:"Name of the subscription to remove" required:"true"`
}

// RemoveMarketplaceOutput echoes the removed marketplace.
type RemoveMarketplaceOutput struct {
	Name string `json:"name" desc:"The removed marketplace name"`
}

// SetMarketplaceEnabledInput toggles a marketplace on/off.
type SetMarketplaceEnabledInput struct {
	Name    string `json:"name"    cli:"arg" desc:"Marketplace name" jsonschema:"Name of the subscription" required:"true"`
	Enabled bool   `json:"enabled" cli:"arg" desc:"Enable (true) or disable (false)"`
}

// SetMarketplaceChannelInput sets the channel an installation follows.
type SetMarketplaceChannelInput struct {
	Name    string `json:"name"    cli:"arg" desc:"Marketplace name" jsonschema:"Name of the subscription" required:"true"`
	Channel string `json:"channel" cli:"arg" desc:"stable (end users - the default) or canary (alpha users: the release candidates too)" required:"true"`
}

// SetMarketplaceChannelOutput echoes the channel followed.
type SetMarketplaceChannelOutput struct {
	Name    string `json:"name"`
	Channel string `json:"channel"`
}

// SetMarketplaceEnabledOutput echoes the new state.
type SetMarketplaceEnabledOutput struct {
	Name    string `json:"name"    desc:"The marketplace name"`
	Enabled bool   `json:"enabled" desc:"The new enabled state"`
}

// RefreshCatalogInput fetches + caches catalogs. Empty Name = all enabled.
type RefreshCatalogInput struct {
	Name string `json:"name,omitempty" cli:"arg" desc:"Marketplace name; empty = all enabled" jsonschema:"Name of one marketplace, or empty for all enabled"`
}

// ── KTSP-shaped I/O for the catalog-read methods ────────────────────────────
type SearchItemsInput struct {
	Query string `json:"query,omitempty" cli:"arg" desc:"Search query; empty = all items" jsonschema:"Free-text search across enabled catalogs"`
}
type GetItemInput struct {
	Name string `json:"name" cli:"arg" desc:"Item name" jsonschema:"Catalog item name" required:"true"`
}
type ItemVersionsInput struct {
	Name string `json:"name" cli:"arg" desc:"Item name" jsonschema:"Installed/available item name" required:"true"`
}
type ItemVersionsOutput struct {
	Versions []string `json:"versions" desc:"Available versions, newest first"`
}

// ── KTSP-shaped I/O for the install/update methods ──────────────────────────
type UninstallInput struct {
	Name string `json:"name" cli:"arg" desc:"Item name to remove (all versions unless a version is given)" jsonschema:"Installed item name" required:"true"`
	// Version removes only that ONE store version and leaves the others
	// installed. Empty removes every version plus the available record.
	Version string `json:"version,omitempty" cli:"version" desc:"Remove only this version (default: all versions)" jsonschema:"A specific installed version"`
}
type UninstallOutput struct {
	Name string `json:"name" desc:"The removed item name"`
	// Version echoes the single version removed, empty when all were removed.
	Version string `json:"version,omitempty" desc:"The removed version, when only one was removed"`
}
type UpdateInput struct {
	Names []string `json:"names,omitempty" cli:"arg" desc:"Items to update; empty = all outdated" jsonschema:"Installed item names, or empty for every outdated item"`
}
type SetVersionInput struct {
	Name         string `json:"name"                   cli:"arg"      desc:"Item name"                                  jsonschema:"Installed/available item" required:"true"`
	Version      string `json:"version,omitempty"      cli:"version"  desc:"Target version; empty = latest"`
	UniverseHome string `json:"universeHome,omitempty" cli:"universe" desc:"Universe to repoint the pin in; empty = current"`
}

// ── KTSP-shaped I/O for configure ───────────────────────────────────────────
type ConfigureItemInput struct {
	Name string `json:"name" cli:"arg" desc:"Installed item to configure" jsonschema:"Installed sidecar name" required:"true"`
	// UniverseHome is the universe whose config dir the check runs against and
	// whose templates get materialized. Empty = the current workspace — over
	// KTSP that is the ADMIN hub's, so a GUI acting on the universe it displays
	// MUST pass its home (the same rule as WireIntoUniverse / Unwire).
	UniverseHome string `json:"universeHome,omitempty" cli:"universe" desc:"Universe to configure in; empty = current"`
}

// ── KTSP-shaped I/O for raw config-file editing ─────────────────────────────
// The text-editor path (sidecar-config-raw-edit): read one of an installed
// item's config files whole, write it back whole. The schema-driven form
// (sidecar-config-schema-and-form) produces the same SetItemConfig call.
type GetItemConfigInput struct {
	Name         string `json:"name"                   cli:"arg"      desc:"Installed item"                                   jsonschema:"Installed sidecar name" required:"true"`
	File         string `json:"file"                   cli:"file"     desc:"Config file name as config check reports it"    jsonschema:"Config file name (e.g. NotifyService.yaml)" required:"true"`
	Default      bool   `json:"default,omitempty"      cli:"default"  desc:"Read the bundled default instead of the live file"`
	UniverseHome string `json:"universeHome,omitempty" cli:"universe" desc:"Universe whose config dir to read; empty = current"`
}
type GetItemConfigOutput struct {
	Name    string `json:"name"           desc:"Installed item"`
	File    string `json:"file"           desc:"Config file name"`
	Path    string `json:"path,omitempty" desc:"Resolved on-disk path (empty for a bundled default)"`
	Content string `json:"content"        desc:"File content (YAML text)"`
}
type SetItemConfigInput struct {
	Name         string `json:"name"                   cli:"arg"      desc:"Installed item"                                   jsonschema:"Installed sidecar name" required:"true"`
	File         string `json:"file"                   cli:"file"     desc:"Config file name as config check reports it"    jsonschema:"Config file name (e.g. NotifyService.yaml)" required:"true"`
	Content      string `json:"content"                cli:"content"  desc:"New content for the whole file; validated by the sidecar before writing" required:"true"`
	UniverseHome string `json:"universeHome,omitempty" cli:"universe" desc:"Universe whose config dir to write; empty = current"`
}
type SetItemConfigOutput struct {
	Name   string                       `json:"name"           desc:"Installed item"`
	File   string                       `json:"file"           desc:"Config file name"`
	Path   string                       `json:"path,omitempty" desc:"Resolved on-disk path that was written"`
	Report sidecarapi.ConfigCheckReport `json:"report"         desc:"The item's config check re-run after the write"`
}

// ── KTSP-shaped I/O for wire/unwire/bootstrap (the enable-orchestration methods) ──
type WireIntoUniverseInput struct {
	Name         string `json:"name"                   cli:"arg"      desc:"Installed item to wire (AVAILABLE→USED)" jsonschema:"Installed item name" required:"true"`
	UniverseHome string `json:"universeHome,omitempty" cli:"universe" desc:"Universe folder; empty = current"`
}
type WireIntoUniverseOutput struct {
	Home string `json:"home" desc:"Resolved universe home"`
}
type UnwireInput struct {
	Name         string `json:"name"                   cli:"arg"      desc:"Item to unwire (USED→AVAILABLE)" jsonschema:"Wired item name" required:"true"`
	UniverseHome string `json:"universeHome,omitempty" cli:"universe" desc:"Universe folder; empty = current"`
}
type UnwireOutput struct {
	Home string `json:"home" desc:"Resolved universe home"`
}
type InstallSolutionInput struct {
	Ref          ItemRef `json:"ref"                    desc:"The solution to install (name[@version])"`
	UniverseHome string  `json:"universeHome,omitempty" cli:"universe" desc:"The universe it goes into: a registered name, a new name or a folder"`
	// ProjectDir is where a solution's seed is unpacked and attached; empty is
	// <universe name>-project beside the universe. A folder that already holds
	// files is kept as it is (installing again repairs, never overwrites).
	ProjectDir string `json:"projectDir,omitempty" cli:"project" desc:"Project folder for the solution's seed (default: <universe name>-project beside the universe)"`
	// Values are collected config-set key values. Installing runs in the
	// engine and cannot prompt, so — like ApplyConfigSet — it takes values and
	// reports what is still missing.
	Values map[string]string `json:"values,omitempty"`
	// KeySetsPassword is the one team password that opens every vault of the
	// solution (its key sets); VaultPasswords, when it names a vault, wins.
	KeySetsPassword string `json:"keySetsPassword,omitempty" desc:"The team password: opens every vault of the solution"`
	// VaultPasswords open the solution's vaults, by vault name.
	VaultPasswords map[string]string `json:"vaultPasswords,omitempty" desc:"Each vault's password, by vault name (wins over keySetsPassword)"`
	// Keys is a sealed keys file (ExportKeys) opened with KeysPassword and
	// written, each key into its scope's .env, before the config-set applies.
	Keys         []byte `json:"keys,omitempty" desc:"A sealed keys file to import before the config-set applies"`
	KeysPassword string `json:"keysPassword,omitempty" desc:"The keys file's password"`
	// DryRun checks the solution's vaults (their passwords) and its
	// config-sets' keys against the target universe and returns them,
	// installing and creating NOTHING — the first call of a caller that
	// collects values.
	DryRun bool `json:"dryRun,omitempty" cli:"dry-run" desc:"Report the vaults and the config-sets' keys and what is missing; install and create nothing"`
	// SolutionName shows the universe under the solution's own display name
	// rather than one made from its folder's name (the solution setup installs:
	// nobody chose its name).
	SolutionName bool `json:"solutionName,omitempty" cli:"solution-name" desc:"Show the universe under the solution's display name instead of its folder's name"`
}
type InstallSolutionOutput struct {
	Home string `json:"home" desc:"Resolved home of the main universe"`
	// Main is the registered name of the main universe: the one that opens
	// after install (katyoti.yml default_universe).
	Main string `json:"main,omitempty" desc:"Registered name of the main universe (the one that opens)"`
	// Universes are every universe the solution installed, the main one first.
	Universes []InstalledUniverse `json:"universes,omitempty" desc:"Every universe the solution installed, the main one first"`
	// ConfigSets report each config-set's apply pass, in the solution's order.
	// Reporting them matters: an install that left keys unset must not look
	// like a clean one.
	ConfigSets []ApplyConfigSetOutput `json:"configSets,omitempty" desc:"Each config-set's apply result"`
	// Vaults are the solution's vaults and whether each opened.
	Vaults []VaultState `json:"vaults,omitempty" desc:"The solution's vaults: opened with the password given, or not"`
	// Project is the folder the solution's seed went to and is attached; empty
	// for a solution without a seed. ProjectKept: it already held files, and
	// was left as it was.
	Project     string `json:"project,omitempty" desc:"Project folder of the solution's seed, attached to the universe"`
	ProjectKept bool   `json:"projectKept,omitempty" desc:"The project folder already held files and was kept as it was"`
	// ProjectNotRecorded is why the project laid out could not be recorded
	// as created on this PC - a warning, never a failure.
	ProjectNotRecorded string `json:"projectNotRecorded,omitempty" desc:"Why the seed's project could not be recorded as created on this PC"`
	// Documents are the ids of the solution's documents placed into the project.
	Documents []string `json:"documents,omitempty" desc:"The ids of the solution's documents placed into the project (CWM documents of its first module)"`
	// Prereqs plans the programs the members' checks report: nothing is
	// installed by the solution install (PlanPrereqs / InstallPrereqs do).
	Prereqs *PrereqsPlan `json:"prereqs,omitempty" desc:"The programs the members need on this machine, planned, not installed"`
}

// InstalledUniverse is one universe a solution installed.
type InstalledUniverse struct {
	Name    string   `json:"name" desc:"Registered name"`
	Home    string   `json:"home" desc:"Universe home"`
	Main    bool     `json:"main,omitempty" desc:"The main universe: the one that opens"`
	Members []string `json:"members,omitempty" desc:"Its sidecars"`
	Project string   `json:"project,omitempty" desc:"The project its seed went to"`
}

// VaultState is one vault of a solution at install.
type VaultState struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Opened  bool     `json:"opened"`
	Keys    []string `json:"keys,omitempty" desc:"The names of the keys it holds, once opened (never a value)"`
	Error   string   `json:"error,omitempty"`
}

// DoctorInput names the universe whose configuration the checks read.
type DoctorInput struct {
	UniverseHome string `json:"universeHome,omitempty" cli:"universe" desc:"Universe to check the sidecars in: a registered name or its folder (default: the default universe)"`
}

// RepinSolutionInput moves the universes a solution installed to another of
// its versions: the update path's repin, and its rollback.
type RepinSolutionInput struct {
	UniverseHome string `json:"universeHome" cli:"universe" desc:"The solution's main universe: a registered name or its folder"`
	Version      string `json:"version,omitempty" desc:"The solution's version to move to (default: its newest)"`
	DryRun       bool   `json:"dryRun,omitempty" cli:"dry-run" desc:"Say what would move and change nothing"`
}

// MemberMove is one member's version before and after.
type MemberMove struct {
	Name string `json:"name"`
	From string `json:"from,omitempty" desc:"Its version before (empty: a new member)"`
	To   string `json:"to,omitempty" desc:"Its version after (empty: no longer a member)"`
}

type RepinSolutionOutput struct {
	Solution  string       `json:"solution" desc:"The solution's name"`
	From      string       `json:"from" desc:"The version the universes recorded"`
	To        string       `json:"to" desc:"The version they record now (or would)"`
	Universes []string     `json:"universes,omitempty" desc:"The homes of the universes moved, the main one first"`
	Members   []MemberMove `json:"members,omitempty" desc:"Each member whose version moves"`
}

// The kinds of a problem `katyoti status` names in an installation
// and `katyoti doctor` repairs.
const (
	// ProblemProgramMissing: a program recorded with its checksum is gone.
	ProblemProgramMissing = "program-missing"
	// ProblemProgramReplaced: a program's bytes no longer match the checksum
	// it was installed with.
	ProblemProgramReplaced = "program-replaced"
	// ProblemVanishedPath: a registry row names a folder or file that is gone.
	ProblemVanishedPath = "vanished-path"
	// ProblemTestEntry: a registry row a test left: a path in the temporary
	// folder, a subscription to a loopback test server.
	ProblemTestEntry = "test-entry"
)

// InstallationProblem is one thing broken in an installation: what, in which
// file, on which row, and what the doctor does about it.
type InstallationProblem struct {
	Kind string `json:"kind"`
	// File is the registry holding the row, or the program's own path.
	File string `json:"file"`
	// Entry is the row's name (an installation, a universe, a program, a
	// marketplace).
	Entry string `json:"entry,omitempty"`
	// Path is the path or URL the row names.
	Path   string `json:"path,omitempty"`
	Detail string `json:"detail"`
	// Repair says what `katyoti doctor` does about it.
	Repair string `json:"repair,omitempty"`
}

// InstallationDiagnosis is what `katyoti status` found broken in the
// installation: the files it read and each problem, none when it is sound.
type InstallationDiagnosis struct {
	Checked  []string              `json:"checked"`
	Problems []InstallationProblem `json:"problems,omitempty"`
}

// RepairInstallationInput asks `katyoti doctor` to repair the installation.
type RepairInstallationInput struct {
	DryRun bool `json:"dryRun,omitempty" cli:"dry-run" desc:"Say what would be repaired and change nothing"`
}

// RepairAction is what the doctor did about one problem, or would do (a dry
// run), or why it left it to the person.
type RepairAction struct {
	Problem InstallationProblem `json:"problem"`
	// Done: the problem is repaired (or would be, in a dry run).
	Done bool   `json:"done"`
	Did  string `json:"did"`
}

// InstallationRepair is what `katyoti doctor` did to the installation: the
// folder holding the copy of every file it changed, made before it changed
// any, and one action per problem status named.
type InstallationRepair struct {
	DryRun  bool           `json:"dryRun,omitempty"`
	Backup  string         `json:"backup,omitempty"`
	Actions []RepairAction `json:"actions,omitempty"`
}
