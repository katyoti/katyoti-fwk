// SPDX-License-Identifier: Apache-2.0

package api

// Marketplace-authoring verbs (the publisher face, ADR §9). These are
// ComposeService methods so they auto-derive KTSP methods + flat CLI verbs
// (BuildMarketplace → build-marketplace) + object-type-addressable actions,
// exactly like the create-* verbs — wrapping the shared marketplace/authoring
// engine. This replaces the earlier hand-written cobra commands.

// BuildMarketplaceInput indexes a folder of binaries into a catalog.
type BuildMarketplaceInput struct {
	Source  string `json:"source"  cli:"arg"      desc:"Folder of built binaries to index" required:"true"`
	Name    string `json:"name"    cli:"name"     desc:"Marketplace name" required:"true"`
	BaseURL string `json:"baseUrl" cli:"base-url" desc:"Asset base URL (default: the source folder itself, for local/dev)"`
	Out     string `json:"output"  cli:"output"   desc:"Output catalog path (default: <source>/katyoti-marketplace.yaml)"`
}

type BuildMarketplaceOutput struct {
	CatalogPath string `json:"catalogPath"`
	ItemCount   int    `json:"itemCount"`
	// Skipped names the binaries that did NOT make the catalog, with the
	// reason. Surfaced so a publisher sees a missing sidecar before
	// distributing, not after.
	Skipped []string `json:"skipped,omitempty"`
}

// InitMarketplaceInput makes a folder a marketplace to publish into.
type InitMarketplaceInput struct {
	Folder    string `json:"folder"    cli:"arg"        desc:"Marketplace folder (created when absent)" required:"true"`
	Name      string `json:"name"      cli:"name"       desc:"Marketplace name (its subscribers' name for it)" required:"true"`
	KeyFolder string `json:"keyFolder" cli:"key-folder" desc:"Signing key folder (default: the marketplace folder; keep the secret key out of a shared folder)"`
	KeyPrefix string `json:"keyPrefix" cli:"key-prefix" desc:"Signing key prefix (<key-folder>/<prefix>.key)" required:"true"`
}

type InitMarketplaceOutput struct {
	Catalog    string `json:"catalog"`
	KeyCreated bool   `json:"keyCreated"`
	PubKeyLine string `json:"pubKeyLine" desc:"The public key subscribers pin"`
}

// BuildInfraChannelInput publishes the control plane of a built tree.
type BuildInfraChannelInput struct {
	Dist      string `json:"dist"      cli:"arg"        desc:"Built tree (a folder of <goos>-<goarch>/ folders)" required:"true"`
	Output    string `json:"output"    cli:"output"     desc:"Marketplace folder (the channel is <output>/infra)" required:"true"`
	Version   string `json:"version"   cli:"version"    desc:"Infra version (immutable once published)" required:"true"`
	KeyFolder string `json:"keyFolder" cli:"key-folder" desc:"Signing key folder" required:"true"`
	KeyPrefix string `json:"keyPrefix" cli:"key-prefix" desc:"Signing key prefix" required:"true"`
	Demo      string `json:"demo,omitempty" cli:"demo" desc:"The solution a first install opens on, as <solution>@<version> (optional)"`
}

type BuildInfraChannelOutput struct {
	Channel   string   `json:"channel"`
	Version   string   `json:"version"`
	Previous  string   `json:"previous,omitempty"`
	Platforms []string `json:"platforms"`
	Pruned    []string `json:"pruned,omitempty"`
}

// BuildVaultInput publishes a sealed keys file as a vault.
type BuildVaultInput struct {
	File      string `json:"file"    cli:"arg"     desc:"The sealed keys file (katyoti keys export)" required:"true"`
	Name      string `json:"name"    cli:"name"    desc:"Vault name" required:"true"`
	Version   string `json:"version" cli:"version" desc:"Version to publish (immutable once published)" required:"true"`
	Out       string `json:"output"  cli:"output"  desc:"Marketplace folder to publish INTO (accumulates)" required:"true"`
	Keep      int    `json:"keep"    cli:"keep"    desc:"Versions to retain (0 = keep all)"`
	KeyFolder string `json:"keyFolder,omitempty" cli:"key-folder" desc:"Signing key folder (re-signs the catalog; required if it is signed)"`
	KeyPrefix string `json:"keyPrefix,omitempty" cli:"key-prefix" desc:"Signing key prefix (the key is <key-folder>/<key-prefix>.key)"`
}

type BuildVaultOutput struct {
	Warning     string `json:"warning,omitempty"`
	CatalogPath string `json:"catalogPath"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	AssetPath   string `json:"assetPath"`
	Sha256      string `json:"sha256"`
}

// ReleaseMarketplaceInput publishes a built tree into an accumulating
// marketplace: assets land under <version>/<goos>-<goarch>/ and the catalog
// gains a version instead of being replaced.
type ReleaseMarketplaceInput struct {
	Source  string `json:"source"  cli:"arg"      desc:"Built tree to publish (a folder of <goos>-<goarch>/ folders)" required:"true"`
	Out     string `json:"output"  cli:"output"   desc:"Marketplace folder to publish INTO (accumulates across releases)" required:"true"`
	Name    string `json:"name"    cli:"name"     desc:"Marketplace name" required:"true"`
	BaseURL string `json:"baseUrl" cli:"base-url" desc:"Asset base URL (default: relative — assets served beside the catalog)"`
	Keep    int    `json:"keep"    cli:"keep"     desc:"Versions to retain per item (0 = keep all)"`
}

// MarketplaceChannelInput puts a published version on a channel.
type MarketplaceChannelInput struct {
	Out           string `json:"output"        cli:"output"     desc:"Marketplace folder" required:"true"`
	Version       string `json:"version"       cli:"arg"        desc:"The published version (2026.10-rc2)" required:"true"`
	Channel       string `json:"channel"       cli:"channel"    desc:"stable or canary (a release candidate, for alpha users)" required:"true"`
	SecretKeyPath string `json:"secretKeyPath" cli:"secret-key" desc:"Minisign secret key: the catalog is signed again"`
}

// MarketplacePromoteInput promotes a candidate to its official version.
type MarketplacePromoteInput struct {
	Out           string `json:"output"        cli:"output"     desc:"Marketplace folder" required:"true"`
	From          string `json:"from"          cli:"arg"        desc:"The candidate version (2026.10-rc2)" required:"true"`
	To            string `json:"to"            cli:"to"         desc:"The official version (2026.10)" required:"true"`
	SecretKeyPath string `json:"secretKeyPath" cli:"secret-key" desc:"Minisign secret key: the catalog is signed again"`
}

// MarketplaceChannelOutput says what a channel change or a promotion touched.
type MarketplaceChannelOutput struct {
	Items   []string `json:"items"`
	Warning string   `json:"warning,omitempty"`
}

type ReleaseMarketplaceOutput struct {
	CatalogPath string   `json:"catalogPath"`
	ItemCount   int      `json:"itemCount"`
	Published   int      `json:"published"`
	AssetCount  int      `json:"assetCount"`
	Skipped     []string `json:"skipped,omitempty"`
	Pruned      []string `json:"pruned,omitempty"`
}

// SnapshotUniverseInput freezes a live universe into an immutable release item.
type SnapshotUniverseInput struct {
	Workspace   string   `json:"workspace"   cli:"arg"         desc:"Universe to snapshot: a registered name or its folder" required:"true"`
	Version     string   `json:"version"     cli:"version"     desc:"Solution version (e.g. v1) — immutable" required:"true"`
	Marketplace string   `json:"marketplace" cli:"marketplace" desc:"The marketplace (its folder or its katyoti-marketplace.yaml) to publish the solution into" required:"true"`
	Name        string   `json:"name"        cli:"name"        desc:"Solution name (default: the universe name)"`
	ConfigSets  []string `json:"configSets,omitempty" cli:"config-set" desc:"Config-sets the solution applies, in order, as <name>@<version> (comma-separated)"`
	Vaults      []string `json:"vaults,omitempty" cli:"vault" desc:"Vaults whose keys the solution imports, as <name>@<version> (comma-separated)"`
	Project     string   `json:"project,omitempty" cli:"project" desc:"Project folder packed as the solution's seed — its objects, documents and files (optional)"`
	Tasks       string   `json:"tasks,omitempty"   cli:"tasks"   desc:"The universe's tasks folder packed into the seed (optional; with --project)"`
	SeedName    string   `json:"seedName,omitempty" cli:"seed-name" desc:"The seed item's name (default: <solution>-seed; with --project)"`
	Docs        string   `json:"docs,omitempty" cli:"docs" desc:"Folder of the solution's documents: <lang>/*.md (README, guides…)"`
	WhatsNew    string   `json:"whatsNew,omitempty" cli:"whats-new" desc:"A version's change log (docs/confluence-tew/changelog/<version>.md) made into the solution's « Quoi de neuf » (French)"`
	KeyFolder   string   `json:"keyFolder,omitempty" cli:"key-folder" desc:"Signing key folder (re-signs the catalog; required if it is signed)"`
	KeyPrefix   string   `json:"keyPrefix,omitempty" cli:"key-prefix" desc:"Signing key prefix (the key is <key-folder>/<key-prefix>.key)"`
	// Others are the solution's other universes, beside the one snapshotted
	// (its main one): each <name>=<universe>, its name in the solution and a
	// registered universe or a folder; each may get its own seed and
	// config-sets.
	Others          []string `json:"others,omitempty" cli:"other" desc:"Another universe of the solution, as <name>=<universe> (a registered name or its folder); the one snapshotted is the main one (comma-separated)"`
	OtherProjects   []string `json:"otherProjects,omitempty" cli:"other-project" desc:"An other universe's seed, as <name>=<project folder>; its tasks are that universe's own tasks folder (comma-separated)"`
	OtherConfigSets []string `json:"otherConfigSets,omitempty" cli:"other-config-set" desc:"A config-set an other universe applies, as <name>=<config-set>@<version> (comma-separated)"`
}

type SnapshotUniverseOutput struct {
	Warning   string   `json:"warning,omitempty"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Members   []string `json:"members"`            // "name@version" (or "name@(unpinned)")
	Unpinned  []string `json:"unpinned,omitempty"` // members not wired from the versioned store
	Seed      string   `json:"seed,omitempty"`     // the seed's asset path, when one was packed
	SeedFiles int      `json:"seedFiles,omitempty"`
	Documents int      `json:"documents,omitempty"` // the documents the solution delivers
	Infra     string   `json:"infra,omitempty"`     // the infra version recorded (the channel beside the catalog)
	// Universes are the solution's universes, the main one first; OtherSeeds
	// the other universes' seeds (name@version).
	Universes  []string `json:"universes,omitempty"`
	OtherSeeds []string `json:"otherSeeds,omitempty"`
}

// KeygenInput generates a marketplace signing keypair. The keys are written
// into Folder (relative or absolute; created if needed) as <Folder>/<Prefix>.key
// and <Folder>/<Prefix>.key.pub.
type KeygenInput struct {
	Folder  string `json:"folder"  cli:"folder"  desc:"Folder to write the keys into (relative or absolute; default: current dir)"`
	Prefix  string `json:"prefix"  cli:"prefix"  desc:"Key filename prefix (writes <folder>/<prefix>.key + <prefix>.key.pub)" required:"true"`
	Comment string `json:"comment" cli:"comment" desc:"Comment embedded in the key files"`
}

type KeygenOutput struct {
	SecretKeyPath string `json:"secretKeyPath"`
	PublicKeyPath string `json:"publicKeyPath"`
	PublicKeyLine string `json:"publicKeyLine"` // hand this to consumers: `marketplace add … --pubkey`
}

// SignMarketplaceInput signs a catalog index. The secret key is <KeyFolder>/
// <KeyPrefix>.key — the same folder + prefix keygen wrote (the marketplace-project
// node binds both from its config, so signing needs no manual key path).
type SignMarketplaceInput struct {
	Catalog   string `json:"catalog"   cli:"arg"        desc:"Catalog (katyoti-marketplace.yaml) to sign" required:"true"`
	KeyFolder string `json:"keyFolder" cli:"key-folder" desc:"Folder holding the signing key (as passed to keygen)"`
	KeyPrefix string `json:"keyPrefix" cli:"key-prefix" desc:"Signing key filename prefix (as passed to keygen)" required:"true"`
	Comment   string `json:"comment"   cli:"comment"    desc:"Trusted comment embedded in the signature"`
}

type SignMarketplaceOutput struct {
	SignaturePath string `json:"signaturePath"`
}

// PublishMarketplaceInput gates a marketplace output for distribution. When
// KeyPrefix is given, the signature is verified against <KeyFolder>/<KeyPrefix>.key.pub
// (the key consumers pin) before declaring it ready.
type PublishMarketplaceInput struct {
	Output    string `json:"output"    cli:"arg"        desc:"Marketplace output folder or catalog path" required:"true"`
	KeyFolder string `json:"keyFolder" cli:"key-folder" desc:"Signing key folder (to verify the signature before distributing)"`
	KeyPrefix string `json:"keyPrefix" cli:"key-prefix" desc:"Signing key prefix — verifies the signature against <folder>/<prefix>.key.pub"`
}

type PublishMarketplaceOutput struct {
	Summary string `json:"summary"`
}

// FreezeHostInput bundles a files-only content folder into a single
// self-contained exe by appending it to a copy of the ktw-host binary — no Go
// rebuild. The frozen exe runs as that sidecar with no `--folder`.
type FreezeHostInput struct {
	Folder string `json:"folder" cli:"arg"    desc:"Content folder (with a ktw-host.yaml) to embed" required:"true"`
	Host   string `json:"host"   cli:"host"   desc:"Path to the ktw-host binary to embed into (default: beside ktw-compose)"`
	Out    string `json:"output" cli:"output" desc:"Output single-file exe path" required:"true"`

	// Sealing is a distribution event, so it is a place to assert origin.
	// Optional: an unsigned pack is valid and usable, and requiring a
	// signature would be an enforcement policy rather than a mechanism.
	KeyFolder string `json:"keyFolder,omitempty" cli:"key-folder" desc:"Signing key folder — signs each contribution's identity as it is sealed"`
	KeyPrefix string `json:"keyPrefix,omitempty" cli:"key-prefix" desc:"Signing key prefix (the key is <key-folder>/<key-prefix>.key)"`
	Publisher string `json:"publisher,omitempty" cli:"publisher"  desc:"Publisher name recorded in the provenance assertion"`
}

type FreezeHostOutput struct {
	ExePath string `json:"exePath"`
	Bytes   int    `json:"bytes"`
}

// PromoteMarketplaceInput commits a published marketplace tree into its git
// repo — the last step of the team-marketplace flow, where git permissions and
// PR review ARE the publish controls.
//
// Push is opt-in: committing is local and reversible, pushing is visible to
// every subscriber before it can be withdrawn.
type PromoteMarketplaceInput struct {
	Marketplace string `json:"marketplace" cli:"arg"     desc:"Marketplace folder inside a git repo" required:"true"`
	Message     string `json:"message"     cli:"message" desc:"Commit message (default: \"publish marketplace\")"`
	Push        bool   `json:"push"        cli:"push"    desc:"Also push to the default remote (opt-in; publishes to every subscriber)"`
}

type PromoteMarketplaceOutput struct {
	Repo      string `json:"repo"`
	Committed bool   `json:"committed"`
	Commit    string `json:"commit,omitempty"`
	Pushed    bool   `json:"pushed"`
}

// CreateContentInput scaffolds a content FOLDER — a files-only sidecar that
// runs on ktw-host with nothing compiled.
//
// Distinct from CreateSidecarInput, which scaffolds a GO sidecar under app/<name>/
// and only works inside this repo. This one writes wherever the author is, which
// is the point: the contributor it serves has files and no Go toolchain.
type CreateContentInput struct {
	Name        string `json:"name"        cli:"arg"          desc:"Sidecar name (kebab-case; becomes the catalog item AND the runtime provider)" required:"true"`
	Out         string `json:"output"      cli:"output"       desc:"Parent folder to create <name>/ in (default: the workspace)"`
	DisplayName string `json:"displayName" cli:"display-name" desc:"Human name (default: the name)"`
	Description string `json:"description" cli:"description"  desc:"One-line description"`
	Include     string `json:"include"     cli:"include"      desc:"Content dirs to prepare (comma list: skills, tasks, personas, references, scripts; default skills)"`
}

type CreateContentOutput struct {
	Folder string   `json:"folder"`
	Files  []string `json:"files,omitempty"`
}

// BuildContentInput publishes a FOLDER as a `kind: content` item — the
// no-Go-toolchain path. A contributor writes skills / tasks / personas /
// references into a folder, declares it with a ktw-host.yaml, and ships it; it
// runs on ktw-host with nothing compiled.
//
// There is no Name field on purpose: the item name comes from the folder's own
// ktw-host.yaml, because that same name is what the host adopts as its runtime
// provider. Letting a publisher override it here would let the catalog and the
// runtime disagree about what the thing is called.
type BuildContentInput struct {
	Folder  string `json:"folder"  cli:"arg"     desc:"Content folder (must contain ktw-host.yaml)" required:"true"`
	Version string `json:"version" cli:"version" desc:"Version to publish (immutable once published)" required:"true"`
	Out     string `json:"output"  cli:"output"  desc:"Marketplace folder to publish INTO (accumulates)" required:"true"`
	Keep    int    `json:"keep"    cli:"keep"    desc:"Versions to retain (0 = keep all)"`
	// A catalog-mutating verb that leaves a SIGNED catalog un-re-signed breaks
	// every consumer pinned to the key -- and breaks them at THEIR next refresh,
	// not here. Pass the key and it re-signs; omit it and the output warns.
	KeyFolder string `json:"keyFolder,omitempty" cli:"key-folder" desc:"Signing key folder (re-signs the catalog; required if it is signed)"`
	KeyPrefix string `json:"keyPrefix,omitempty" cli:"key-prefix" desc:"Signing key prefix (the key is <key-folder>/<key-prefix>.key)"`
	Publisher string `json:"publisher,omitempty" cli:"publisher" desc:"Publisher name recorded in each contribution's provenance assertion"`
}

type BuildContentOutput struct {
	Warning     string   `json:"warning,omitempty"`
	CatalogPath string   `json:"catalogPath"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	AssetPath   string   `json:"assetPath"`
	Sha256      string   `json:"sha256"`
	Pruned      []string `json:"pruned,omitempty"`
	// BulkLayers counts the files written to the object store instead of into
	// the archive. Reported because "this pack is small" and "its content is
	// elsewhere" are the same fact, and an author who cannot see the second
	// has no way to tell a hoisted pack from one that lost its content.
	BulkLayers int `json:"bulkLayers,omitempty"`
}

// BuildConfigSetInput packages a config-set bundle folder (keys.yaml +
// config/ templates) into a `kind: config` catalog version. The bundle is
// VALIDATED first: a template referencing an undeclared ${VAR} is refused here,
// so the consumer is never asked for a key nobody declared.
type BuildConfigSetInput struct {
	Folder      string `json:"folder"      cli:"arg"          desc:"Config-set bundle folder (keys.yaml + config/)" required:"true"`
	Name        string `json:"name"        cli:"name"         desc:"Config-set item name" required:"true"`
	Version     string `json:"version"     cli:"version"      desc:"Version to publish (immutable once published)" required:"true"`
	Out         string `json:"output"      cli:"output"       desc:"Marketplace folder to publish INTO (accumulates)" required:"true"`
	DisplayName string `json:"displayName" cli:"display-name" desc:"Human name for the catalog (default: the item name)"`
	Description string `json:"description" cli:"description"  desc:"One-line description for the catalog"`
	Keep        int    `json:"keep"        cli:"keep"         desc:"Versions to retain (0 = keep all)"`
	KeyFolder   string `json:"keyFolder,omitempty" cli:"key-folder" desc:"Signing key folder (re-signs the catalog; required if it is signed)"`
	KeyPrefix   string `json:"keyPrefix,omitempty" cli:"key-prefix" desc:"Signing key prefix (the key is <key-folder>/<key-prefix>.key)"`
}

// BuildSeedInput packages a project folder as a `kind: seed` version.
type BuildSeedInput struct {
	Project     string   `json:"project"     cli:"arg"          desc:"Project folder (its CWM objects and every file)" required:"true"`
	Tasks       string   `json:"tasks,omitempty" cli:"tasks"    desc:"The universe's tasks folder packed with it (optional)"`
	Name        string   `json:"name"        cli:"name"         desc:"Seed item name" required:"true"`
	Version     string   `json:"version"     cli:"version"      desc:"Version to publish (immutable once published)" required:"true"`
	Out         string   `json:"output"      cli:"output"       desc:"Marketplace folder to publish INTO (accumulates)" required:"true"`
	Requires    []string `json:"requires,omitempty" cli:"requires" desc:"Members the seed was made with, as name@version (comma-separated)"`
	DisplayName string   `json:"displayName" cli:"display-name" desc:"Human name for the catalog (default: the item name)"`
	Description string   `json:"description" cli:"description"  desc:"One-line description for the catalog"`
	Demo        bool     `json:"demo,omitempty" cli:"demo"   desc:"Its content is demo data: the Marketplace shows a Demo badge"`
	Keep        int      `json:"keep"        cli:"keep"         desc:"Versions to retain (0 = keep all)"`
	KeyFolder   string   `json:"keyFolder,omitempty" cli:"key-folder" desc:"Signing key folder (re-signs the catalog; required if it is signed)"`
	KeyPrefix   string   `json:"keyPrefix,omitempty" cli:"key-prefix" desc:"Signing key prefix (the key is <key-folder>/<key-prefix>.key)"`
}

// BuildSeedOutput reports one seed publish.
type BuildSeedOutput struct {
	Warning     string   `json:"warning,omitempty"`
	CatalogPath string   `json:"catalogPath"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	AssetPath   string   `json:"assetPath"`
	Sha256      string   `json:"sha256"`
	Files       int      `json:"files"`
	Pruned      []string `json:"pruned,omitempty"`
}

type BuildConfigSetOutput struct {
	Warning     string   `json:"warning,omitempty"`
	CatalogPath string   `json:"catalogPath"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	AssetPath   string   `json:"assetPath"`
	Sha256      string   `json:"sha256"`
	Keys        []string `json:"keys,omitempty"`
	Templates   []string `json:"templates,omitempty"`
	Pruned      []string `json:"pruned,omitempty"`
}

// RemoveVersionInput withdraws ONE published version of ONE item: the catalog
// entry and that version's asset files go, empty directories are collapsed, and
// the catalog is re-signed (a manual `rm` would leave the catalog advertising
// missing bytes under a now-stale signature).
type RemoveVersionInput struct {
	Ref       string `json:"ref"       cli:"arg"        desc:"Item to withdraw as <item>@<version>" required:"true"`
	Out       string `json:"output"    cli:"output"     desc:"Marketplace folder to remove from" required:"true"`
	KeyFolder string `json:"keyFolder" cli:"key-folder" desc:"Folder holding the signing key (as passed to keygen)"`
	KeyPrefix string `json:"keyPrefix" cli:"key-prefix" desc:"Signing key filename prefix — required, the catalog must be re-signed" required:"true"`
	Comment   string `json:"comment"   cli:"comment"    desc:"Trusted comment embedded in the new signature"`
	Force     bool   `json:"force"     cli:"force"      desc:"Withdraw even if a published solution pins this version (leaves that release unbootstrappable)"`
}

type RemoveVersionOutput struct {
	CatalogPath   string   `json:"catalogPath"`
	Item          string   `json:"item"`
	Version       string   `json:"version"`
	RemovedAssets []string `json:"removedAssets,omitempty"`
	ItemRemoved   bool     `json:"itemRemoved"`
	SignaturePath string   `json:"signaturePath,omitempty"`
}

// BuildFromDefinitionInput publishes a whole marketplace from its definition
// (marketplace.yaml) at a version, from the binaries `make` built.
type BuildFromDefinitionInput struct {
	Definition  string   `json:"definition" cli:"arg"       desc:"The marketplace definition: a marketplace.yaml, or the folder holding it" required:"true"`
	Version     string   `json:"version"    cli:"version"   desc:"The version everything is published at (e.g. v1.003.rc03)" required:"true"`
	Dist        string   `json:"dist,omitempty" cli:"dist"  desc:"The built binaries: <os>-<arch>/ folders (default: dist)"`
	Reinit      bool     `json:"reinit,omitempty" cli:"reinit" desc:"Start over: wipe the published tree first (subscribers' pinned versions stop resolving)"`
	WhatsNew    string   `json:"whatsNew,omitempty" cli:"whats-new" desc:"The change log made into each solution's « Quoi de neuf »"`
	Covers      []string `json:"covers,omitempty" cli:"covers" desc:"More change logs a written « Quoi de neuf » covers beside --whats-new (the KaTyoti version it ships); a solution's letter_covers adds its own (comma-separated)"`
	DryRun      bool     `json:"dryRun,omitempty" cli:"dry-run" desc:"Run the checks and say what would be published; write nothing"`
	Platforms   []string `json:"platforms,omitempty" cli:"platforms" desc:"Publish only these of the definition's platforms (a fast cut), comma-separated"`
	BeforeBuild bool     `json:"beforeBuild,omitempty" cli:"before-build" desc:"With --dry-run: check everything but the build (run before building it)"`
	Channel     string   `json:"channel,omitempty" cli:"channel" desc:"The channel the version is published on: stable (the default) or canary - a release candidate, seen only by alpha users"`
}

// BuildFromDefinitionOutput is what was published, step by step.
type BuildFromDefinitionOutput struct {
	Output  string `json:"output"`
	Version string `json:"version"`
	// LastPublished is the highest version the marketplace carried before the
	// cut (empty for a first one or a start over).
	LastPublished string   `json:"lastPublished,omitempty"`
	Published     []string `json:"published"`
	Mirrors       []string `json:"mirrors,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
	Summary       string   `json:"summary,omitempty"`
}
