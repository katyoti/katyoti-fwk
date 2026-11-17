// SPDX-License-Identifier: Apache-2.0

package contribution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// systemContribution is the in-memory snapshot of one embedded YAML
// file after the loader has parsed it. Cached so Enumerate / Get on
// the produced ContributionProvider don't re-read the FS on every call.
type systemContribution struct {
	// uri is the canonical URI derived from the file path:
	//   embed/<type>/<basename>.yaml → system:/<type>/<basename>
	uri string

	// typ is the contribution type discriminator (the file's parent
	// directory under embed/).
	typ string

	// id is the basename without `.yaml` — the addressable identifier
	// within (system, typ).
	id string

	// body is the parsed YAML re-marshalled as canonical JSON — for kinds
	// whose content is a structured descriptor a consumer decodes.
	body json.RawMessage

	// structured is whether the file was read as a tree (its kind's format
	// structured, or no format declared): its data layer is JSON. A kind
	// read as dsl or raw is its source alone.
	structured bool

	// source is the authored text of the file, as itself. For a data-type it
	// is the DSL and the only layer — there is no structured body to put
	// beside it. For every other kind it is the YAML the descriptor was
	// decoded from, comments included: the declaration a reader opens, where
	// `body` is the descriptor a consumer decodes. Two layers, two questions,
	// each answered once (the kind declares `source: application/yaml`).
	source []byte
}

// loadSystemContributions walks systemFS, parses every *.yaml under
// `embed/<type>/`, and returns the resulting in-memory cache.
//
// File-format rules per contribution-meta-model.md §4:
//
//   - File path `embed/<type>/<id>.yaml` produces URI `system:/<type>/<id>`
//   - The YAML content IS the Contribution's data layer — parsed to a generic
//     tree and re-marshalled as JSON. No magic envelope wrapping.
//   - Non-YAML files (`.md`, etc.) are silently ignored — drop docs in
//     subdirectories without breaking the loader.
//
// Errors here ARE fatal — the system layer is the bootstrap chain;
// every other contribution kind depends on it. A typo in a YAML file
// should fail compile-time CI (via the snapshot test), not at sidecar
// startup, but the loader still hard-fails as a defense.
func loadSystemContributions() ([]systemContribution, error) {
	type file struct{ path, typ, id string }
	var files []file
	err := fs.WalkDir(systemFS, "embed", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") {
			return nil
		}
		// Path shape: embed/<type>/<id>.<ext>
		rel := strings.TrimPrefix(p, "embed/")
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("system contribution: malformed path %q (expected embed/<type>/<id>)", p)
		}
		files = append(files, file{path: p, typ: parts[0], id: strings.TrimSuffix(parts[1], path.Ext(parts[1]))})
		return nil
	})
	if err != nil {
		return nil, err
	}

	read := func(f file, structured bool) (systemContribution, error) {
		raw, err := fs.ReadFile(systemFS, f.path)
		if err != nil {
			return systemContribution{}, fmt.Errorf("system contribution %s: read: %w", f.path, err)
		}
		c := systemContribution{
			uri:        contribapi.SystemProvider + ":/" + f.typ + "/" + f.id,
			typ:        f.typ,
			id:         f.id,
			structured: structured,
			source:     raw,
		}
		if structured {
			// The decoded tree is the data layer, the authored YAML the
			// source layer. Content, not configuration: decoded through the
			// codec with no variable expansion (a declaration may say ${…}).
			var tree any
			if err := marshal.Unmarshal(raw, marshal.YAML, &tree); err != nil {
				return systemContribution{}, fmt.Errorf("system contribution %s: yaml parse: %w", f.path, err)
			}
			if c.body, err = json.Marshal(tree); err != nil {
				return systemContribution{}, fmt.Errorf("system contribution %s: json re-marshal: %w", f.path, err)
			}
		}
		return c, nil
	}

	// The kinds first: a contribution-type is a structured declaration by
	// definition (the meta-kind ends the chain), and the declarations say how
	// every other folder's files are read — the kind's source format.
	out := []systemContribution{}
	var declared []contribapi.Contribution
	for _, f := range files {
		if f.typ != contribapi.ResourceTypeContributionType {
			continue
		}
		c, err := read(f, true)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
		declared = append(declared, c.entry())
	}
	kinds := contribapi.KindsFrom(declared)
	for _, f := range files {
		if f.typ == contribapi.ResourceTypeContributionType {
			continue
		}
		c, err := read(f, readsTree(kinds, f.typ))
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}

	// Sort by URI for deterministic Enumerate order. The catalog
	// doesn't require it, but the snapshot test compares the emitted
	// URIs and stable order keeps regressions easy to spot.
	sort.Slice(out, func(i, j int) bool { return out[i].uri < out[j].uri })
	return out, nil
}

// systemContributionProvider is a single ContributionProvider that
// serves ONE type of system contribution. The loader instantiates one
// per distinct `<type>` directory under embed/ — so a binary registers
// one provider for "contribution-type", one for "data-type", one for
// "service", etc. This matches the existing ContributionProvider
// shape (one Type per provider).
type systemContributionProvider struct {
	typ   string
	items []systemContribution
	epoch contribapi.ImmutableEpoch
}

// readsTree says whether a folder's files are read as a tree: its kind
// declares the structured format, or no format at all (the generic reading).
func readsTree(kinds contribapi.Kinds, folder string) bool {
	d, ok := kinds.ForFolder(folder)
	if !ok || d.Source == nil {
		return true
	}
	return d.Source.Format == contribapi.SourceStructured
}

// mediaType is what this system contribution's bytes ARE. Enumerate and Get
// must agree: a consumer that reads through one and renders by the other's
// answer gets YAML labelled as JSON.
func (c systemContribution) mediaType() string {
	if !c.structured {
		return "application/yaml"
	}
	return "application/json"
}

func (p *systemContributionProvider) Type() string     { return p.typ }
func (p *systemContributionProvider) Provider() string { return contribapi.SystemProvider }

// entry is the ONE envelope for a system contribution; Enumerate lists it
// and Get answers from it, so the two cannot disagree on a layer.
func (c systemContribution) entry() contribapi.Contribution {
	e := contribapi.Contribution{
		Provider: contribapi.SystemProvider,
		Type:     c.typ,
		ID:       c.id,
		URI:      c.uri,
		Metadata: &contribapi.ContributionMeta{MediaType: c.mediaType()},
	}
	if len(c.body) > 0 {
		e.SetData(c.body)
	}
	if len(c.source) > 0 {
		e.SetSource(c.source, "application/yaml")
	}
	return e
}

func (p *systemContributionProvider) Enumerate(_ context.Context) ([]contribapi.Contribution, error) {
	out := make([]contribapi.Contribution, 0, len(p.items))
	for _, item := range p.items {
		out = append(out, item.entry())
	}
	return out, nil
}

// Open implements ContributionProvider.Open from this provider's own
// enumeration — one answer, the entry Enumerate already carries.
func (p *systemContributionProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

// SystemContributionsExtension returns an Extension that registers
// every embedded system contribution as a `ContributionProvider`
// against the existing `ContributionProvidersPoint`. After it runs,
// `<binary>:Contributions/list {filter: {Providers: ["system"]}}`
// returns the bootstrap metadata that every binary embeds identically.
//
// Per contribution-meta-model.md §4 & §5: the hub aggregation rule
// dedups these across sidecars — they appear once in the universe-wide
// catalog even though every binary carries them.
//
// Wire up in a binary's bootstrap immediately after the manifest
// extension:
//
//	exts := []application.Extension{
//	    appcore.AppCoreExt,
//	    manifestlocal.Extension(),
//	    contribution.SystemContributionsExtension(),
//	    // … other extensions
//	}
//
// Loading happens once at AppInit. Failures (malformed YAML, missing
// embed) fail bootstrap loudly — this is the bootstrap chain and a
// silent regression here would leave consumers unable to validate
// anything.
func SystemContributionsExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		// AppInit phase — after RegisterPoints (which is where
		// ContributionProvidersPoint gets registered by the manifest
		// extension) but before Bootstrap (which is when the catalog
		// builder walks providers).
		init.AddAppInit("contribution", ".system", func(app *application.Application) error {
			items, err := loadSystemContributions()
			if err != nil {
				return fmt.Errorf("system contributions load: %w", err)
			}
			// Group by type — one ContributionProvider per <type>
			// matches the existing provider-per-type convention in the
			// manifest extension.
			byType := map[string][]systemContribution{}
			for _, it := range items {
				byType[it.typ] = append(byType[it.typ], it)
			}
			// Deterministic order over the type set so the
			// ContributionProvidersPoint's ListPoint receives
			// providers in the same order on every run.
			types := make([]string, 0, len(byType))
			for t := range byType {
				types = append(types, t)
			}
			sort.Strings(types)

			for _, t := range types {
				p := &systemContributionProvider{typ: t, items: byType[t]}
				if err := application.Contribute(app, contribapi.ContributionProvider(p)); err != nil {
					return fmt.Errorf("contribute system %q provider: %w", t, err)
				}
			}
			return nil
		})
	}
}

// Identity: the system meta-model is compiled into every binary, so it is
// immutable and names its version by its enumeration digest (computed once).
func (p *systemContributionProvider) Identity() contribapi.Identity { return p.epoch.Identity(p) }

// SystemKinds is the kind registry of the system set alone — every
// `system:/contribution-type/*` this binary embeds — for a reader that holds
// no manifest (a lint of one file on disk). A reader holding contributions
// builds its registry from them (contribapi.KindsFrom), which includes these.
func SystemKinds() (contribapi.Kinds, error) {
	entries, err := SystemKindEntries()
	if err != nil {
		return contribapi.Kinds{}, err
	}
	return contribapi.KindsFrom(entries), nil
}

// SystemKindEntries is every `system:/contribution-type/*` entry this binary
// embeds — what a reader overlays with other declarations (builtin.Kinds).
func SystemKindEntries() ([]contribapi.Contribution, error) {
	items, err := loadSystemContributions()
	if err != nil {
		return nil, err
	}
	entries := make([]contribapi.Contribution, 0, len(items))
	for _, it := range items {
		if it.typ == contribapi.ResourceTypeContributionType {
			entries = append(entries, it.entry())
		}
	}
	return entries, nil
}

// SystemFormats is the format (data-type major.minor) of every kind the system
// set this binary embeds declares — what a host built from it reads
// (contribapi.JudgeFormat). Computed once.
func SystemFormats() map[string]string {
	systemFormatsOnce.Do(func() {
		items, err := loadSystemContributions()
		if err != nil {
			return
		}
		entries := make([]contribapi.Contribution, 0, len(items))
		for _, it := range items {
			entries = append(entries, it.entry())
		}
		kinds := contribapi.KindsFrom(entries)
		versions := map[string]string{}
		for _, e := range entries {
			if e.Type == contribapi.ResourceTypeDataType {
				if f := contribapi.DataTypeFormat(e.Source()); f != "" {
					versions[e.ResolvedURI()] = f
				}
			}
		}
		out := map[string]string{}
		for _, name := range kinds.Names() {
			if f := versions[kinds.SchemaFor(name)]; f != "" {
				out[name] = f
			}
		}
		systemFormats = out
	})
	out := make(map[string]string, len(systemFormats))
	for k, v := range systemFormats {
		out[k] = v
	}
	return out
}

var (
	systemFormatsOnce sync.Once
	systemFormats     map[string]string
)
