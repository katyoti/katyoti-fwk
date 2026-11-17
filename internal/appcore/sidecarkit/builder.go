// SPDX-License-Identifier: Apache-2.0

// Package sidecarkit collapses the ~12-extension boilerplate every
// single-service sidecar declares into a small option-driven builder.
// The hand-written extension list still works (and remains documented) —
// `NewSidecar` is a CONVENIENCE wrapper that wires the framework-mandatory
// bits and returns the same `[]application.Extension` slice the previous
// form produced.
//
// Lives outside `appcore` (and uses `appcore` as a peer) to avoid an
// import cycle with `internal/extensions/llm/local` which already
// imports `appcore`.
package sidecarkit

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	llmlocal "github.com/katyoti/katyoti-fwk/internal/extensions/llm/local"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
)

// NewSidecar returns the canonical extension list for a single-service
// sidecar. See the package doc for the mandatory / opt-in / caller-
// supplied breakdown.
//
// Usage (replaces the typical ~30-LOC bootstrap):
//
//	func buildExtensions(efs embed.FS) []application.Extension {
//	    return sidecarkit.NewSidecar(appcore.AppConfig{
//	        AppName: "ktw-weather", Version: version,
//	        Short: "Weather forecast sidecar", EmbeddedFS: efs,
//	    },
//	        sidecarkit.WithDomain(weatherlocal.Extension()),
//	        sidecarkit.WithKTSPDescription("Weather forecast service"),
//	        sidecarkit.WithFlatCLI(),
//	    )
//	}
//
// Multi-domain binaries (ktw-agents, ktw-hub) keep the hand-written
// form — their wiring is too custom to fit the option family cleanly.
// ktw-docs and the connector sidecars use the kit.
func NewSidecar(cfg appcore.AppConfig, opts ...Option) []application.Extension {
	o := defaultOptions(cfg)
	for _, opt := range opts {
		opt(&o)
	}

	exts := []application.Extension{
		appcore.NewAppExt(cfg),
		appcore.SidecarGroupsExt(),
		appcore.SidecarExt(cfg.EmbeddedFS),
		llmlocal.Extension(),
		manifestlocal.Extension(),
	}

	if o.configResources {
		exts = append(exts, manifestlocal.ContributeConfigResources(cfg.EmbeddedFS))
	}

	exts = append(exts, o.domainExts...)

	// No kind's runtime is wired here: the kit branches on no kind. A binary
	// that runs one (the skills runtime, skillslocal.Extension) passes it with
	// WithDomain; the kind's entries are the pack's (below).

	// The binary's embedded pack, its CWM pack a part of it.
	exts = append(exts, packExtensions(cfg, o)...)

	if o.scripts != nil {
		// Opt-in: wire BOTH the ScriptService runtime AND the loader that feeds
		// it from `embed/scripts`. A sidecar that ships no scripts publishes
		// no ScriptService at all — no empty `scripts` KTSP slot on the bus.
		exts = append(exts, o.scripts(cfg.AppName, cfg.EmbeddedFS)...)
	}

	if o.ktspEnabled {
		ktspOpts := []ktslocal.AutoKTSPOption{}
		if o.flatCLI {
			ktspOpts = append(ktspOpts, ktslocal.WithFlatCLI())
		}
		desc := o.ktspDescription
		if desc == "" {
			desc = cfg.Short
		}
		exts = append(exts, ktslocal.AutoKTSPContainer(cfg.AppName, desc, cfg.Version, ktspOpts...))
	}

	if o.httpServerEnabled {
		exts = append(exts, ktslocal.AutoHTTPServer(cfg.AppName, cfg.Version))
	}

	exts = append(exts, o.extraExtensions...)
	return exts
}

// packExtensions is the binary's embedded pack (on by default; WithPack,
// WithoutPack): every folder of the root a kind claims, and its CWM pack
// (`<root>/cwm`) as a part of the pack. Nil with no pack or no embedded FS.
func packExtensions(cfg appcore.AppConfig, o options) []application.Extension {
	if !o.packEnabled || cfg.EmbeddedFS == nil {
		return nil
	}
	// content is the embed loader for `provider`, transparently overlaid
	// with `.katyoti/overwrites/<provider>/` when the universe opts in
	// (allow_overwrites): the overlay is wired ONCE and is generic across all
	// contribution types.
	content := func(provider string) loader.Loader {
		return contentLoader(loader.NewEmbedFSLoader(cfg.EmbeddedFS, "."), provider)
	}
	// The binary's embedded pack: every folder of the root a kind
	// claims (skills/, tasks/, references/, data-types/, personas/,
	// guide/, display-engines/, contribution-types/, …), read as the
	// kind declares (manifestcontrib.ContributePack); a folder no kind
	// claims (config/, docs/) is not content of the pack. Its cwm/
	// folder is a part of the pack (cwmPart), read beside the parts of
	// the extensions the binary hosts.
	// Its leaf kinds are decided on the embedded pack, at build time:
	// the overlay resolves later and only overrides files of kinds the
	// binary already ships.
	embedded := loader.NewEmbedFSLoader(cfg.EmbeddedFS, ".")
	out := []application.Extension{packExtension(cfg.AppName, content(cfg.AppName), embedded, o.packRoot, o.packRemap, builtin.Leaves())}
	if part := cwmPart(cfg.AppName, cfg.EmbeddedFS, o.packRoot); part != nil {
		out = append(out, part)
	}
	return out
}

// Option configures NewSidecar.
type Option func(*options)

type options struct {
	domainExts      []application.Extension
	extraExtensions []application.Extension

	packEnabled bool
	packRoot    string
	packRemap   map[string]string

	// scripts is an OPTIONAL FEATURE supplied by the caller as a factory, not
	// selected by a boolean. A boolean would be a runtime flag, and Go links
	// by import: a kit that imports the feature package to honour a disabled
	// flag still links it into every binary. Holding a factory here keeps the
	// import at the call site, so only a binary that actually uses the
	// feature pays for it.
	scripts Feature

	configResources   bool
	ktspEnabled       bool
	ktspDescription   string
	flatCLI           bool
	httpServerEnabled bool
}

func defaultOptions(cfg appcore.AppConfig) options {
	return options{
		packEnabled: true,
		packRoot:    "embed",
		// scripts are an OPT-IN feature — most sidecars don't ship them.
		configResources:   true,
		ktspEnabled:       true,
		httpServerEnabled: true,
	}
}

// WithDomain appends one or more domain extensions — the actual reason
// the sidecar exists (e.g. `weatherlocal.Extension()`).
func WithDomain(ext ...application.Extension) Option {
	return func(o *options) { o.domainExts = append(o.domainExts, ext...) }
}

// WithExtraExtensions appends arbitrary extensions to the tail of the
// returned slice — for sidecars that need a single bit of custom
// wiring on top of the builder defaults. If you find yourself reaching
// for this for more than 1-2 lines, fall back to the explicit list.
func WithExtraExtensions(ext ...application.Extension) Option {
	return func(o *options) { o.extraExtensions = append(o.extraExtensions, ext...) }
}

// WithPack reads the binary's embedded pack from another root than `embed`.
// The pack is every folder of the root a kind claims, read as the kind
// declares (`system:/contribution-type/<kind>`, its `source:` block): skills,
// tasks, references, data-types, personas, guide recipes, display engines,
// the binary's own kind declarations. On by default.
func WithPack(root string) Option {
	return func(o *options) {
		o.packEnabled = true
		o.packRoot = root
	}
}

// WithoutPack publishes nothing from the embedded pack — a binary whose
// content comes from elsewhere (ktw-host serves a folder) or that has no
// embedded FS.
func WithoutPack() Option {
	return func(o *options) { o.packEnabled = false }
}

// WithPackRemap reads a kind of the pack from another folder (a path under
// the pack's root), or not at all when the folder is "" (manifestcontrib.
// WithRemap).
func WithPackRemap(remap map[string]string) Option {
	return func(o *options) { o.packRemap = remap }
}

// packExtension publishes the pack under root of ldr, its kinds resolved
// through the built-in kind reader — compiled into the binary, so a set that
// does not read is a broken build, refused at boot. A leaf kind of leaves
// whose folder the embedded pack holds is declared by the binary itself
// (builtin.ContributeLeafKinds), once; no held leaf, no call — the helper
// called with no leaf would publish nothing anyway, and builtin.Leaves()
// with no kind named lists them all.
func packExtension(provider string, ldr, embedded loader.Loader, root string, remap map[string]string, leaves []leaf.Declaration) application.Extension {
	kinds, err := builtin.Kinds()
	if err != nil {
		return func(init *application.ApplicationInit) {
			init.AddContribute("manifest", ".pack."+provider, func(*application.Application) error {
				return fmt.Errorf("sidecar pack of %s: the built-in kinds: %w", provider, err)
			})
		}
	}
	pack := manifestcontrib.ContributePack(provider, ldr, root, kinds, manifestcontrib.WithRemap(remap))
	held := heldLeaves(embedded, root, remap, kinds, leaves)
	if len(held) == 0 {
		return pack
	}
	declare := builtin.ContributeLeafKinds(provider, held...)
	return func(init *application.ApplicationInit) {
		pack(init)
		declare(init)
	}
}

// cwmFolder is the CWM pack's place under the pack's root: a layout constant
// of the kit, never a kind name. Its folders are read by the kinds whose
// `source:` names them (object types, capabilities, aspects, data-types, …).
const cwmFolder = "cwm"

// cwmPart is the binary's CWM pack (`<root>/cwm`, when the embedded FS holds
// it) as a part of the binary's pack (manifestcontrib.ContributePackPart):
// one provider per kind over it and the parts of the extensions the binary
// hosts, the universe override tree (`overwrites/<provider>/embed/cwm`) last.
// Nil when the binary ships no CWM pack.
func cwmPart(provider string, efs fs.FS, root string) application.Extension {
	dir := path.Join(root, cwmFolder)
	if fi, err := fs.Stat(efs, dir); err != nil || !fi.IsDir() {
		return nil
	}
	sub, err := fs.Sub(efs, dir)
	if err != nil {
		return nil
	}
	return manifestcontrib.ContributePackPart(provider, provider, cwmFolder, sub)
}

// heldLeaves are the leaves whose folder — the kind's source folder, or the
// folder remap names ("" = switched off) — holds files under root of ldr.
func heldLeaves(ldr loader.Loader, root string, remap map[string]string, kinds contribapi.Kinds, leaves []leaf.Declaration) []leaf.Declaration {
	if len(leaves) == 0 || ldr == nil {
		return nil
	}
	files := ldr.Walk()
	var out []leaf.Declaration
	for _, l := range leaves {
		folder, remapped := remap[l.Kind]
		if !remapped {
			if d, ok := kinds.Get(l.Kind); ok {
				folder = d.SourceFolder()
			}
		}
		folder = strings.Trim(folder, "/")
		if folder == "" {
			continue
		}
		prefix := path.Join(root, folder) + "/"
		for _, f := range files {
			if strings.HasPrefix(path.Clean(f), prefix) {
				out = append(out, l)
				break
			}
		}
	}
	return out
}

// Feature builds the extensions for an optional capability. The CALLER supplies
// it, so the capability's package is imported by the binary that uses it — never
// by this kit, which every sidecar links.
//
// A boolean option would not do: it is a runtime flag, and Go links by import,
// so a kit importing the package to honour a disabled flag still links it
// everywhere. Each feature package exposes a ready-made constructor.
type Feature func(appName string, efs fs.FS) []application.Extension

// WithScripts enables the scripts extension: the ScriptService runtime
// (KTSP `scripts` slot) PLUS the loader feeding it from the embedded
// `embed/scripts/` folder. OPT-IN — only sidecars that actually ship
// scripts should pass it, so a script-less binary publishes no empty
// ScriptService to the bus. Override the embed root via WithScriptsFrom.
func WithScripts(f Feature) Option {
	return func(o *options) { o.scripts = f }
}

// WithScriptsFrom is like WithScripts but lets the caller override the
// provider name and embedded root path for the script loader.
func WithScriptsFrom(f Feature) Option { return WithScripts(f) }

// WithoutConfigResources disables `ContributeConfigResources`. Rare —
// only useful when a sidecar doesn't want its config-file/program/env
// declarations to appear in `resources list`.
func WithoutConfigResources() Option {
	return func(o *options) { o.configResources = false }
}

// WithKTSPDescription sets the description passed to AutoKTSPContainer.
// Defaults to cfg.Short.
func WithKTSPDescription(desc string) Option {
	return func(o *options) { o.ktspDescription = desc }
}

// WithFlatCLI flattens the auto-generated CLI: single-service sidecars
// expose method commands directly at the top level (e.g.
// `ktw-weather get-forecast`) rather than under a slot group.
func WithFlatCLI() Option {
	return func(o *options) { o.flatCLI = true }
}

// WithoutKTSP disables the AutoKTSPContainer extension entirely.
func WithoutKTSP() Option {
	return func(o *options) { o.ktspEnabled = false }
}

// WithoutHTTPServer disables the AutoHTTPServer extension.
func WithoutHTTPServer() Option {
	return func(o *options) { o.httpServerEnabled = false }
}
