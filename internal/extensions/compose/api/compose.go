// SPDX-License-Identifier: Apache-2.0

// Package api defines ComposeService — ktw-compose's verb surface: universe
// composition (wrapping the orchestrator service in-process), files-only
// sidecar scaffolding, and per-artifact file emission from the canonical
// templates (internal/extensions/compose/internal/templates).
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// ComposeService is the files-only composer. Method names auto-derive the
// kebab CLI/KTSP verbs (CreateSkill → create-skill). Naming is <verb>-<target>
// per modules/compose/verbs.md. Reading (list/get) is NOT a compose
// verb — it belongs to the orchestrator / catalog read surface (verbs.md §4).
type ComposeService interface {
	// Universe + membership composition — wraps the orchestrator (in-process).
	CreateUniverse(ctx context.Context, in CreateUniverseInput) (orchapi.UniverseInfo, error)
	AddMember(ctx context.Context, in AddMemberInput) (orchapi.UniverseInfo, error)
	RemoveMember(ctx context.Context, in RemoveMemberInput) (orchapi.UniverseInfo, error)

	// Sidecar scaffolding.
	CreateSidecar(ctx context.Context, in CreateSidecarInput) (CreateSidecarOutput, error)

	// ApplyUniverse is the headless assembly: optionally scaffold + build a
	// new files-only sidecar, create the universe, add every member. The
	// interactive `compose-universe` CLI and --from-spec both converge here.
	ApplyUniverse(ctx context.Context, in ApplyUniverseInput) (ApplyUniverseOutput, error)

	// Artifact emission into an existing sidecar's embed tree (target=source,
	// the default) OR the workspace overlay (target=overwrite — served live
	// without recompiling; promote when ready).
	CreateSkill(ctx context.Context, in CreateSkillInput) (CreateItemOutput, error)
	CreateTask(ctx context.Context, in CreateTaskInput) (CreateItemOutput, error)
	CreatePersona(ctx context.Context, in CreatePersonaInput) (CreateItemOutput, error)
	CreateReference(ctx context.Context, in CreateReferenceInput) (CreateItemOutput, error)

	// Promote copies a workspace-overlay artifact into the sidecar's source
	// embed tree (app/<sidecar>/embed/…) and REMOVES the overlay copy — the
	// overlay always wins, so it must go once the source is authoritative.
	// Returns the source path + a rebuild/PR note.
	Promote(ctx context.Context, in PromoteInput) (PromoteOutput, error)

	// EnsureContributions gives the project its sys-contributions module when
	// it has none - a system module that operates contributions - and says
	// where its contributions go (the folder and token projectfile names).
	// Every --target project verb runs it first.
	EnsureContributions(ctx context.Context, in EnsureContributionsInput) (EnsureContributionsOutput, error)
	// CreateOverride overrides an entry of a sidecar for the project: an
	// override object in sys-contributions carrying the capability
	// `contributes` with `overrides`, and only the layers named, each a file
	// of the module seeded from the entry's current layer.
	CreateOverride(ctx context.Context, in CreateOverrideInput) (CreateOverrideOutput, error)

	// LintSkill checks a SKILL file against the skill-format contract. When
	// `served` is given (the in-binary body the host fetched via
	// get_skill_instructions), it ALSO runs the embedded-freshness check — a
	// stale sidecar is caught deterministically before a test scores the wrong
	// version. The generic basics (file-exists, markdown-frontmatter, …) are
	// NOT here — they belong to the methods-owned LinterService. The
	// `compose-skill` node type lists this URI as a `linter:` (path only →
	// format-only) so object-verify folds the findings in.
	LintSkill(ctx context.Context, in LintSkillInput) (verify.LintResult, error)
	// LintTask is the sibling artifact-specific linter for TASK files (the
	// task-format contract). Declared by the `compose-task` node type.
	LintTask(ctx context.Context, in LintTaskInput) (verify.LintResult, error)

	// LintReference lints one reference document (file or published resource):
	// relative links and URIs.
	LintReference(ctx context.Context, in LintReferenceInput) (verify.LintResult, error)

	// LintPersona lints one persona (file or published resource): its
	// config keys, through the agent strategy's own key check, then relative
	// links and URIs.
	LintPersona(ctx context.Context, in LintPersonaInput) (verify.LintResult, error)

	// LintGuideRecipe lints one guide recipe (file or published resource)
	// against the guide-recipe contract: fields, steps, targets, URIs.
	LintGuideRecipe(ctx context.Context, in LintGuideRecipeInput) (verify.LintResult, error)

	// LintDeck lints one slide deck (.deck.md) against the deck grammar, and
	// against the deck-layout contributions of a compiled sidecar when one is
	// given. Each finding names its line.
	LintDeck(ctx context.Context, in LintDeckInput) (verify.LintResult, error)

	// LintSidecar lints every skill and task a COMPILED sidecar publishes —
	// format by kind, relative links, own-provider resource and tool URIs —
	// from its manifest and sources (`sc resources manifest` / `read`), so it
	// works on an executable built anywhere. The same check a sidecar's own
	// census runs at build.
	LintSidecar(ctx context.Context, in LintSidecarInput) (ContentLintOutput, error)

	// LintUniverse lints every skill and task of every executable in a folder
	// against each other: every provider, resource, slot and method a body
	// names must be published by SOME binary in the folder. What no single
	// sidecar can check about itself.
	LintUniverse(ctx context.Context, in LintUniverseInput) (ContentLintOutput, error)

	// I18n reports what a pack (a folder on disk) or a compiled sidecar still
	// has to translate into one language — each text with its source beside
	// it — through the one translation lint; on a folder, `scaffold` writes
	// the `<name>.i18n.yaml` stubs to fill (skill translate-contributions).
	I18n(ctx context.Context, in I18nInput) (I18nOutput, error)

	// Marketplace-authoring (the publisher face, ADR §9) — wraps the shared
	// marketplace/authoring engine. As service methods these are KTSP-callable,
	// generate the flat CLI verbs (build-marketplace, …), and are addressable as
	// object-type actions like every other verb.
	InitMarketplace(ctx context.Context, in InitMarketplaceInput) (InitMarketplaceOutput, error)
	BuildMarketplace(ctx context.Context, in BuildMarketplaceInput) (BuildMarketplaceOutput, error)
	BuildInfraChannel(ctx context.Context, in BuildInfraChannelInput) (BuildInfraChannelOutput, error)
	ReleaseMarketplace(ctx context.Context, in ReleaseMarketplaceInput) (ReleaseMarketplaceOutput, error)
	ChannelMarketplace(ctx context.Context, in MarketplaceChannelInput) (MarketplaceChannelOutput, error)
	PromoteCandidate(ctx context.Context, in MarketplacePromoteInput) (MarketplaceChannelOutput, error)
	SnapshotUniverse(ctx context.Context, in SnapshotUniverseInput) (SnapshotUniverseOutput, error)
	// BuildFromDefinition publishes a whole marketplace from its marketplace.yaml.
	BuildFromDefinition(ctx context.Context, in BuildFromDefinitionInput) (BuildFromDefinitionOutput, error)
	Keygen(ctx context.Context, in KeygenInput) (KeygenOutput, error)
	SignMarketplace(ctx context.Context, in SignMarketplaceInput) (SignMarketplaceOutput, error)
	PublishMarketplace(ctx context.Context, in PublishMarketplaceInput) (PublishMarketplaceOutput, error)
	RemoveVersion(ctx context.Context, in RemoveVersionInput) (RemoveVersionOutput, error)
	PromoteMarketplace(ctx context.Context, in PromoteMarketplaceInput) (PromoteMarketplaceOutput, error)
	CreateContent(ctx context.Context, in CreateContentInput) (CreateContentOutput, error)
	BuildContent(ctx context.Context, in BuildContentInput) (BuildContentOutput, error)
	BuildConfigSet(ctx context.Context, in BuildConfigSetInput) (BuildConfigSetOutput, error)
	BuildSeed(ctx context.Context, in BuildSeedInput) (BuildSeedOutput, error)
	BuildVault(ctx context.Context, in BuildVaultInput) (BuildVaultOutput, error)
	FreezeHost(ctx context.Context, in FreezeHostInput) (FreezeHostOutput, error)
}

// ArtifactRef names ONE artifact to lint, either as a file on disk
// (`--file`) or as a resource a compiled sidecar publishes (`--uri` with
// `--exe`). Every per-type linter carries these three fields directly (the
// auto-CLI reads direct fields, not embedded ones) and hands them over as
// one value; lint-sidecar takes the exe alone and runs all of them.
type ArtifactRef struct {
	File string `json:"file,omitempty" cli:"file" desc:"File to lint (workspace-relative or absolute)"`
	URI  string `json:"uri,omitempty" cli:"uri" desc:"Resource URI (<provider>:/<type>/<id>) to lint as the sidecar publishes it"`
	Exe  string `json:"exe,omitempty" cli:"exe" desc:"Sidecar executable that publishes the URI (required with --uri)"`
}

// LintTaskInput lints one TASK file or resource.
type LintTaskInput struct {
	File string `json:"file,omitempty" cli:"file" desc:"File to lint (workspace-relative or absolute)"`
	URI  string `json:"uri,omitempty" cli:"uri" desc:"Resource URI (<provider>:/<type>/<id>) to lint as the sidecar publishes it"`
	Exe  string `json:"exe,omitempty" cli:"exe" desc:"Sidecar executable that publishes the URI (required with --uri)"`
}

// LintReferenceInput lints one reference document: its links and URIs.
type LintReferenceInput struct {
	File string `json:"file,omitempty" cli:"file" desc:"File to lint (workspace-relative or absolute)"`
	URI  string `json:"uri,omitempty" cli:"uri" desc:"Resource URI (<provider>:/<type>/<id>) to lint as the sidecar publishes it"`
	Exe  string `json:"exe,omitempty" cli:"exe" desc:"Sidecar executable that publishes the URI (required with --uri)"`
}

// LintDeckInput lints one slide deck file.
type LintDeckInput struct {
	File string   `json:"file" cli:"file" desc:"Deck file to lint (workspace-relative or absolute)"`
	Exe  []string `json:"exe,omitempty" cli:"exe" desc:"Sidecar executables whose deck-layout and deck-theme contributions the deck is checked against, comma-separated (e.g. ktw-docs,ktw-acme); without it neither layouts nor themes are checked"`
}

// LintPersonaInput lints one persona: its config keys (the agent strategy's check) and its links and URIs.
type LintPersonaInput struct {
	File string `json:"file,omitempty" cli:"file" desc:"File to lint (workspace-relative or absolute)"`
	URI  string `json:"uri,omitempty" cli:"uri" desc:"Resource URI (<provider>:/<type>/<id>) to lint as the sidecar publishes it"`
	Exe  string `json:"exe,omitempty" cli:"exe" desc:"Sidecar executable that publishes the URI (required with --uri)"`
}

// LintGuideRecipeInput lints one guide recipe.
type LintGuideRecipeInput struct {
	File string `json:"file,omitempty" cli:"file" desc:"Recipe file to lint (workspace-relative or absolute)"`
	URI  string `json:"uri,omitempty" cli:"uri" desc:"Resource URI (<provider>:/guide-recipe/<id>) to lint as the sidecar publishes it"`
	Exe  string `json:"exe,omitempty" cli:"exe" desc:"Sidecar executable that publishes the URI (required with --uri)"`
}

// LintSidecarInput names one compiled sidecar.
type LintSidecarInput struct {
	Exe string `json:"exe" cli:"exe" desc:"Path to the sidecar executable" required:"true"`
}

// LintUniverseInput names a folder of compiled sidecars.
type LintUniverseInput struct {
	BinDir string `json:"bin_dir" cli:"bin-dir" desc:"Folder holding the sidecar executables (default: bin/)"`
	// Project is a project folder whose contributions folder is judged with
	// the members: its token, its files.
	Project string `json:"project,omitempty" cli:"project" desc:"A project folder: its contributions token must be no member's name nor reserved, and its contributions folder must hold no *.cwm.* file"`
}

// I18nInput names what to translate and into which language.
type I18nInput struct {
	Target   string `json:"target" cli:"arg" desc:"A pack folder on disk (embed/cwm, a binary's embed/, or above), or a sidecar executable (path or name on PATH)" required:"true"`
	Lang     string `json:"lang,omitempty" cli:"lang" desc:"The language to translate into (default fr)"`
	Missing  bool   `json:"missing,omitempty" cli:"missing" desc:"List only the entries that miss texts, each text with its source"`
	Scaffold bool   `json:"scaffold,omitempty" cli:"scaffold" desc:"Write a ~ stub per missing text into the <name>.i18n.yaml beside each entry (a folder only)"`
	Into     string `json:"into,omitempty" cli:"into" desc:"With --scaffold on a sidecar executable: project writes the stubs into an override object of the project per entry - its translations layer only, the entry's existing languages kept"`
}

// I18nOutput is a pack's translation state in one language.
type I18nOutput struct {
	Status     string      `json:"status" desc:"complete | incomplete | invalid (a translations file has errors)"`
	Lang       string      `json:"lang"`
	Entries    int         `json:"entries" desc:"Translatable entries read"`
	Missing    int         `json:"missing" desc:"Texts not translated"`
	Scaffolded []string    `json:"scaffolded,omitempty" desc:"Translations files the scaffold wrote"`
	Objects    []string    `json:"objects,omitempty" desc:"Override objects the scaffold into the project created"`
	Report     []I18nEntry `json:"report,omitempty" desc:"Per entry: what it misses and the errors of its translations"`
}

// I18nEntry is one entry's translation state in the language.
type I18nEntry struct {
	Entry      string               `json:"entry" desc:"The entry: its file on disk, or its URI"`
	Kind       string               `json:"kind"`
	SourceLang string               `json:"source_lang" desc:"The language the entry is written in"`
	Missing    []I18nText           `json:"missing,omitempty" desc:"Texts not translated into the language"`
	Errors     []verify.LintFinding `json:"errors,omitempty" desc:"Errors of its translations (a text the kind does not declare, a bad file)"`
}

// I18nText is one text to translate.
type I18nText struct {
	Path   string `json:"path" desc:"The text's path in the entry (label, states.draft.name…)"`
	Source string `json:"source" desc:"The text in the entry's own language"`
}

// ContentLintOutput is the report of a sidecar or universe content lint.
type ContentLintOutput struct {
	Status   string                   `json:"status" desc:"pass | fail"`
	Binaries []string                 `json:"binaries" desc:"Executables read"`
	Skipped  []string                 `json:"skipped,omitempty" desc:"Executables that do not answer as a sidecar (no manifest), with the reason — a helper binary in bin/ is not a finding"`
	Findings []verify.ResourceFinding `json:"findings,omitempty" desc:"One per (entry, rule)"`
}

// LintSkillInput lints one SKILL file or resource. When Served is set, the
// freshness check also runs (served-vs-source diff).
type LintSkillInput struct {
	File string `json:"file,omitempty" cli:"file" desc:"File to lint (workspace-relative or absolute)"`
	URI  string `json:"uri,omitempty" cli:"uri" desc:"Resource URI (<provider>:/<type>/<id>) to lint as the sidecar publishes it"`
	Exe  string `json:"exe,omitempty" cli:"exe" desc:"Sidecar executable that publishes the URI (required with --uri)"`
	// Served, when given, is the body the platform currently serves for this
	// skill (from get_skill_instructions on the host). Present → also run the
	// embedded-freshness check. Absent → format-only.
	Served string `json:"served,omitempty" desc:"Optional: the served (in-binary) body — when given, also runs the stale-embedded-skill freshness check"`
	// Skill names the platform skill URI/name — used only to name the sidecar
	// to rebuild in a freshness finding.
	Skill string `json:"skill,omitempty" desc:"Optional: platform skill URI/name (<provider>:/skill/<name>) — names the sidecar in a freshness finding"`
}

// ComposeServiceDef declares the service for publication.
var ComposeServiceDef = meta.ServiceDef{
	Name:    "compose",
	Short:   "Files-only solution composer",
	Long:    "Compose katyoti solutions: create universes + members, scaffold files-only sidecars from the canonical templates, and emit skills/tasks/personas/references into a sidecar's embed tree. In-repo only (Q-A1): file-emitting verbs run from the ktw-agents repo root.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"CreateUniverse":      meta.Method("Create a new universe (empty member list)"),
		"AddMember":           meta.Method("Add a sidecar member to an existing universe"),
		"RemoveMember":        meta.Method("Remove a sidecar member from an existing universe"),
		"CreateSidecar":       meta.Method("Scaffold a files-only sidecar under app/<name>/ from the canonical templates (in-repo only)"),
		"ApplyUniverse":       meta.Method("Realize a solution spec: optionally scaffold+build a new files-only sidecar, create the universe, add all members (the headless core of `compose-universe`)"),
		"CreateSkill":         meta.Method("Emit a skill file into a sidecar's embed/skills/"),
		"CreateTask":          meta.Method("Emit a flat task file into a sidecar's embed/tasks/"),
		"CreatePersona":       meta.Method("Emit a persona file into a sidecar's embed/personas/"),
		"CreateReference":     meta.Method("Emit a reference doc into a sidecar's embed/references/"),
		"Promote":             meta.Method("Promote a universe-overlay artifact (<sidecar> <path>) or a project override object (--object) into the sidecar's source embed/ and remove the overlay copy or the object (rebuild + open a PR)"),
		"EnsureContributions": meta.Method("Give the project its sys-contributions module (a system module operating contributions) when it has none, and say its folder and token; run again, it changes nothing"),
		"CreateOverride":      meta.Method("Override an entry of a sidecar for the project: `create-override --uri <provider:/kind/id> [--layers a,b]` creates an override object in sys-contributions carrying only those layers, each a file seeded from the entry's current layer"),

		"LintSkill":       meta.Method("Lint one SKILL — `--file <path>` or `--uri <provider>:/skill/<id> --exe <sidecar>` — against the skill-format contract, its relative links and its URIs; with `served`, also fails stale-embedded-skill when the served body diverges from source"),
		"LintTask":        meta.Method("Lint one TASK — `--file` or `--uri --exe` — against the task-format contract (required keys, agent|task|workflow|simulator type, closed root-key set, no runtime config at root, kind:object|collection ⇒ types, agent-must-complete), its links and its URIs"),
		"LintReference":   meta.Method("Lint one reference document — `--file` or `--uri --exe` — its relative links and URIs"),
		"LintPersona":     meta.Method("Lint one persona — `--file` or `--uri --exe` — its relative links and URIs"),
		"LintGuideRecipe": meta.Method("Lint one guide recipe — `--file <path>` or `--uri <provider>:/guide-recipe/<id> --exe <sidecar>` — against the guide-recipe contract: fields, steps, targets and URIs"),
		"LintDeck":        meta.Method("Lint one slide deck — `--file <deck>.deck.md [--exe <sidecar>,…]` — against the deck grammar and, with --exe, the layouts and themes those sidecars publish; each finding names its line"),
		"LintSidecar":     {Short: "Lint every skill and task a compiled sidecar publishes (format, relative links, own-provider URIs) from its manifest — works on an executable built anywhere", TimeoutS: 300},
		"I18n":            meta.Method("What a pack folder or a compiled sidecar still has to translate into `--lang` (default fr): `i18n <folder|exe> [--missing] [--scaffold]` — each missing text with its source; --scaffold writes the <name>.i18n.yaml stubs to fill, then rerun to check"),
		"LintUniverse":    {Short: "Lint every skill and task of every executable in a folder against each other: every provider, resource, slot and method a body names must be published by some binary", TimeoutS: 900},

		// build-marketplace runs `describe` over every binary in the folder — far
		// longer than the 5s bus default; advertise a generous per-call timeout.
		// marketplace build: the whole marketplace from its definition — a cross-
		// platform release plus every item; tens of minutes on a full matrix.
		"BuildFromDefinition": {Short: "Publish a whole marketplace from its marketplace.yaml at a version: sidecars, infra, config-sets, seeds, vaults (sealed from an .env), solutions (from their scenario cases), checked first, then mirrored", CLIName: "marketplace build", TimeoutS: 7200},
		"BuildMarketplace":    {Short: "Build a katyoti-marketplace.yaml by indexing a folder of built binaries (describe + sha256)", TimeoutS: 300},
		// Release copies every asset into the marketplace tree on top of the
		// describe pass — minutes for a full multi-platform matrix.
		"ChannelMarketplace": {Short: "Put a published version on a channel: canary for a release candidate (alpha users), stable for end users; the catalog signed again", CLIName: "marketplace channel"},
		"PromoteCandidate":   {Short: "Promote a candidate to its official version on stable: its own artifacts, nothing rebuilt; the catalog signed again", CLIName: "marketplace promote"},
		"ReleaseMarketplace": {Short: "Publish a built tree into an accumulating marketplace (versioned assets, catalog gains a version, retention pruned)", TimeoutS: 900},
		"SnapshotUniverse":   {Short: "Freeze a universe into an immutable solution: its members pinned, its config-sets, seed and vaults, its documents (README, Quoi de neuf) per language; zero secrets", TimeoutS: 120},
		"BuildVault":         meta.Method("Publish a sealed keys file (katyoti keys export) as a vault - as it is: its password is never the marketplace's"),
		"InitMarketplace":    meta.Method("Make a folder a marketplace to publish into: its signing key (created when absent - never replaced), an empty signed catalog, the public key beside it"),
		"BuildInfraChannel":  {Short: "Publish the control plane (katyoti, ktw-hub, ktw-gui, ktw-univers) of a built tree into <marketplace>/infra at a version: signed SHA256SUMS per platform, the signed pointer (the previous version kept for roll back), the public key, the installers", TimeoutS: 600},
		"Keygen":             meta.Method("Generate an Ed25519 signing keypair for a marketplace (writes <folder>/<prefix>.key + .key.pub)"),
		"SignMarketplace":    meta.Method("Sign a marketplace catalog index (writes <catalog>.minisig) — consumers verify it against the pinned key"),
		"PublishMarketplace": meta.Method("Verify a marketplace is ready to distribute (index parses, signed, signature verifies) + print the consumer subscribe command"),
		"RemoveVersion":      meta.Method("Withdraw one published <item>@<version>: drop the catalog entry + its assets, GC emptied folders, re-sign the catalog"),
		"PromoteMarketplace": meta.Method("Commit a published marketplace into its git repo (--push to publish to subscribers; opt-in)"),
		"CreateContent":      meta.Method("Scaffold a content FOLDER (files-only sidecar: ktw-host.yaml + content dirs + a starter skill) — no Go toolchain"),
		"BuildContent":       meta.Method("Publish a FOLDER as a kind:content item (files-only sidecar; identity from its ktw-host.yaml) — the no-Go-toolchain path"),
		"BuildSeed":          meta.Method("Package a project folder (its CWM objects and every file) and a universe's tasks as a kind:seed catalog version - the content of a universe, applied like a config-set and pinned by a solution; refuses a .env, leaves .katyoti/.git/node_modules out"),
		"BuildConfigSet":     meta.Method("Package a config-set bundle (keys.yaml + config/ templates) into a kind:config catalog version; refuses a template with an undeclared ${VAR}"),
		"FreezeHost":         meta.Method("Freeze a content folder into a single self-contained exe (embed it into ktw-host — runs with no --folder)"),
	},
}

// === Universe + membership verbs ===

type CreateUniverseInput struct {
	Name        string `json:"name"        cli:"arg"     desc:"Universe name (kebab)" required:"true"`
	DisplayName string `json:"displayName" cli:"display" desc:"Human-readable name (default: capitalized name)"`
	Icon        string `json:"icon"        cli:"icon"    desc:"1-2 char icon"`
	Color       string `json:"color"       cli:"color"   desc:"Mantine color preset"`
	HomePath    string `json:"homePath"    cli:"home"    desc:"Universe home folder (default: ~/.config/katyoti/universes/<name>/)"`
}

type AddMemberInput struct {
	Universe string `json:"universe" cli:"arg"    desc:"Universe name" required:"true"`
	Sidecar  string `json:"sidecar"  cli:"arg"    desc:"Sidecar member name"  required:"true"`
	Binary   string `json:"binary"   cli:"binary" desc:"Absolute binary path (validated when given)"`
}

type RemoveMemberInput struct {
	Universe string `json:"universe" cli:"arg" desc:"Universe name" required:"true"`
	Sidecar  string `json:"sidecar"  cli:"arg" desc:"Sidecar member name"  required:"true"`
}

// === Sidecar verbs ===

type CreateSidecarInput struct {
	Name string `json:"name" cli:"arg" desc:"Sidecar name (kebab, conventionally ktw-*)" required:"true"`
	// Include selects the embed content dirs the scaffold prepares.
	// Comma list; default "skills". Recognized: skills, tasks, personas,
	// references, templates, config.
	Include string `json:"include" cli:"include" desc:"Embed dirs to prepare (comma list; default skills)"`
	Short   string `json:"short"   cli:"short"   desc:"Short description (default derived from name)"`
}

type CreateSidecarOutput struct {
	Path  string   `json:"path"  desc:"Created sidecar folder (app/<name>)"`
	Files []string `json:"files" desc:"Files written"`
	Note  string   `json:"note"  desc:"Follow-up instructions (Makefile APPS list, build command)"`
}

// === Solution flow (realized by apply-universe / compose-universe) ===

// SolutionSpec is the full description of a solution — what the interactive
// `compose-universe` flow collects, and what --from-spec decodes from YAML.
type SolutionSpec struct {
	Name        string `json:"name"        yaml:"name"        desc:"Universe name (kebab)" required:"true"`
	DisplayName string `json:"displayName" yaml:"display_name" desc:"Human-readable name (default: capitalized name)"`
	Icon        string `json:"icon"        yaml:"icon"`
	Color       string `json:"color"       yaml:"color"`
	HomePath    string `json:"homePath"    yaml:"home_path"   desc:"Universe home folder (default: ~/.config/katyoti/universes/<name>/)"`
	// Sidecars are EXISTING members (from scan-inventory; Q-A2: no
	// registry pull — not-installed sidecars are an error).
	Sidecars []SolutionSidecar `json:"sidecars" yaml:"sidecars"`
	// NewSidecar optionally scaffolds one files-only sidecar that joins
	// the universe too.
	NewSidecar *SolutionNewSidecar `json:"newSidecar,omitempty" yaml:"new_sidecar,omitempty"`
	// Replace archives an existing universe of the same name first.
	Replace bool `json:"replace,omitempty" yaml:"replace,omitempty"`
	// SkipBuild skips `go build` of the scaffolded sidecar (tests/CI).
	SkipBuild bool `json:"skipBuild,omitempty" yaml:"skip_build,omitempty"`
}

type SolutionSidecar struct {
	Name   string `json:"name"   yaml:"name"   required:"true"`
	Binary string `json:"binary" yaml:"binary" desc:"Absolute binary path (validated when given)"`
}

type SolutionNewSidecar struct {
	Name    string `json:"name"    yaml:"name"    required:"true"`
	Include string `json:"include" yaml:"include" desc:"Embed dirs to prepare (comma list; default skills)"`
	Short   string `json:"short"   yaml:"short"`
}

type ApplyUniverseInput struct {
	Spec SolutionSpec `json:"spec" desc:"The solution description" required:"true"`
}

type ApplyUniverseOutput struct {
	Universe    orchapi.UniverseInfo `json:"universe"`
	Scaffolded  *CreateSidecarOutput `json:"scaffolded,omitempty" desc:"Present when a new sidecar was scaffolded"`
	BuiltBinary string               `json:"builtBinary,omitempty" desc:"Path of the built scaffolded binary"`
	Steps       []string             `json:"steps" desc:"Human-readable log of what happened, in order"`
}

// === Artifact verbs ===

// A Create* verb's Target selects where it writes. `source` (default) emits into
// the sidecar's in-repo embed tree (needs rebuild + go:embed). `overwrite` emits
// into the workspace overlay (`.katyoti/overwrites/<sidecar>/embed/…`) — served
// live when the universe sets allow_overwrites, no recompile; promote when ready.
type CreateSkillInput struct {
	Sidecar     string `json:"sidecar"     cli:"arg"         desc:"Target sidecar (app/<sidecar> must exist); none with --target project"`
	Name        string `json:"name"        cli:"arg"         desc:"Skill name (kebab)"`
	Description string `json:"description" cli:"description" desc:"What + when description (the L0 line)"`
	Category    string `json:"category"    cli:"category"    desc:"Category (default guide)"`
	Advertised  bool   `json:"advertised"  cli:"advertised"  desc:"Advertise in skill listings (default false — sub-skill)"`
	Target      string `json:"target"      cli:"target"      desc:"Where to write: source (in-repo embed/, needs rebuild) | overwrite (universe overlay, served live, no recompile) | project (the project contributions folder; no sidecar argument). Default source."`
}

type CreateTaskInput struct {
	Sidecar     string `json:"sidecar"     cli:"arg"  desc:"Target sidecar; none with --target project"`
	Name        string `json:"name"        cli:"arg"  desc:"Task name (kebab)"`
	Type        string `json:"type"        cli:"type" desc:"Task type: agent | simple | workflow (default agent)"`
	Description string `json:"description" cli:"description" desc:"One-line description"`
	Target      string `json:"target"      cli:"target"      desc:"Where to write: source (in-repo embed/, needs rebuild) | overwrite (universe overlay, served live, no recompile) | project (the project contributions folder; no sidecar argument). Default source."`
}

type CreatePersonaInput struct {
	Sidecar     string `json:"sidecar"     cli:"arg" desc:"Target sidecar" required:"true"`
	Name        string `json:"name"        cli:"arg" desc:"Persona name (kebab)" required:"true"`
	Description string `json:"description" cli:"description" desc:"One-line description"`
	Target      string `json:"target"      cli:"target"      desc:"Where to write: source (in-repo embed/, needs rebuild) | overwrite (universe overlay, served live, no recompile) | project (the project contributions folder; no sidecar argument). Default source."`
}

type CreateReferenceInput struct {
	Sidecar string `json:"sidecar" cli:"arg"   desc:"Target sidecar" required:"true"`
	Path    string `json:"path"    cli:"arg"   desc:"Reference path under embed/references/ (e.g. topic/file.md)" required:"true"`
	Title   string `json:"title"   cli:"title" desc:"Document title (default derived from path)"`
	Target  string `json:"target"  cli:"target" desc:"Where to write: source (in-repo embed/, needs rebuild) | overwrite (universe overlay, served live, no recompile) | project (the project contributions folder; no sidecar argument). Default source."`
}

type CreateItemOutput struct {
	Path string `json:"path" desc:"File written"`
	Note string `json:"note" desc:"Follow-up instructions (rebuild, go:embed coverage)"`
}

// PromoteInput addresses one overlay artifact by sidecar + embed-relative
// path, or one override object of the project by its id.
type PromoteInput struct {
	Sidecar string `json:"sidecar" cli:"arg" desc:"Sidecar whose overlay artifact to promote (not with --object)"`
	Path    string `json:"path"    cli:"arg" desc:"Overlay file relative to embed/ (e.g. skills/foo.md, tasks/bar.md)"`
	Object  string `json:"object,omitempty" cli:"object" desc:"An override object of the project (sys-contributions.override-...): its layers are written into the files of app/<sidecar>/embed/ they are read from, then the object and its files are removed"`
}

type PromoteOutput struct {
	Source  string   `json:"source"  desc:"Source file written under app/<sidecar>/embed/"`
	Removed string   `json:"removed" desc:"Overlay file or override object removed"`
	Written []string `json:"written,omitempty" desc:"Source files an override object's layers were written into"`
	Note    string   `json:"note"    desc:"Follow-up: rebuild + open a PR"`
}

// EnsureContributionsInput takes nothing: the project is the universe's.
type EnsureContributionsInput struct{}

// EnsureContributionsOutput says what EnsureContributions found or made.
type EnsureContributionsOutput struct {
	Module   string `json:"module" desc:"The module holding the project's contributions"`
	Created  bool   `json:"created,omitempty" desc:"True when the module was created now"`
	Provider string `json:"provider" desc:"The token the project's contributions are published under"`
	Folder   string `json:"folder" desc:"Their folder, relative to the project root"`
}

// CreateOverrideInput names the entry to override and its layers.
type CreateOverrideInput struct {
	URI    string   `json:"uri" cli:"uri" desc:"The entry to override (provider:/kind/id), an entry of a sidecar" required:"true"`
	Layers []string `json:"layers,omitempty" cli:"layers" desc:"The layers to override, comma-separated (default every layer the entry has)"`
	Target string   `json:"target,omitempty" cli:"target" desc:"Where the override goes: project (an override object in sys-contributions). Default project."`
}

// CreateOverrideOutput is the override object made.
type CreateOverrideOutput struct {
	Object    string            `json:"object" desc:"The override object's id"`
	Overrides string            `json:"overrides" desc:"The entry it overrides"`
	Layers    map[string]string `json:"layers" desc:"Each layer it carries and its file in the module"`
	Note      string            `json:"note,omitempty" desc:"Follow-up: edit the files, then promote when ready"`
}

// Ref is the artifact this input names.
func (in LintTaskInput) Ref() ArtifactRef {
	return ArtifactRef{File: in.File, URI: in.URI, Exe: in.Exe}
}

// Ref is the artifact this input names.
func (in LintReferenceInput) Ref() ArtifactRef {
	return ArtifactRef{File: in.File, URI: in.URI, Exe: in.Exe}
}

// Ref is the artifact this input names.
func (in LintPersonaInput) Ref() ArtifactRef {
	return ArtifactRef{File: in.File, URI: in.URI, Exe: in.Exe}
}

// Ref is the artifact this input names.
func (in LintGuideRecipeInput) Ref() ArtifactRef {
	return ArtifactRef{File: in.File, URI: in.URI, Exe: in.Exe}
}

// Ref is the artifact this input names.
func (in LintSkillInput) Ref() ArtifactRef {
	return ArtifactRef{File: in.File, URI: in.URI, Exe: in.Exe}
}
