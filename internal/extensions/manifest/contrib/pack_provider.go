// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/json"
	"log/slog"
	"path"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// pack_provider.go — one producer for every kind of a pack.
//
// A pack is a folder of contributions laid out by kind: each first-level
// folder holds the files of one kind, and a file at the root says its kind in
// a `type:` field. The kind of a folder is the kind whose `source:` names it,
// else the kind of that name; how its files become layers is the kind's
// `source:` block (contribapi.SourceDecl), else the generic reading (the file
// as its source, YAML/JSON as its data, its `.i18n.yaml` as its translations).
// A folder no kind claims is no contribution (a binary's embed/ holds config/,
// docs/, cwm/): left out, unless a remap names it for a kind (WithRemap) — a
// kind no binary declares is then read the generic way, and the census
// reports it as undeclared.
//
// The pack's own `contribution-type` declarations count: a pack may declare a
// kind and ship its entries together. A folder the pack's layer manifest
// records content under is read though it is gone from disk (bulk content
// lives in the object store). Nothing here links a kind's package.
//
// Left out: `_`-prefixed folders and files (a pack's own bookkeeping) and
// `*.cwm.*` files (CWM objects, which a workspace reads, never a catalog).

// PackOption tunes how a pack is read.
type PackOption func(*packOptions)

type packOptions struct {
	remap map[string]string // kind → folder under the pack's root; "" = off
}

// WithRemap reads a kind's files from another folder than its own (a path
// under the pack's root), or not at all when the folder is "" — a pack whose
// layout is not the conventional one says so (ktw-host.yaml `dirs:`). The
// kind's own folder is then not read as that kind. A kind no binary declares
// is read from its folder the generic way.
func WithRemap(remap map[string]string) PackOption {
	return func(o *packOptions) { o.remap = remap }
}

// ContributePack returns an Extension publishing every kind of the pack under
// <root> in <ldr>, the kinds resolved against <kinds> (the system set and any
// other the binary knows) and the pack's own declarations.
func ContributePack(provider string, ldr loader.Loader, root string, kinds contribapi.Kinds, opts ...PackOption) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("manifest", ".pack."+provider+"."+root, func(app *application.Application) error {
			resolved := provider
			if resolved == "" {
				resolved = app.Provider()
			}
			for _, p := range PackProviders(resolved, ldr, root, kinds, opts...) {
				if err := application.Contribute(app, contribapi.ContributionProvider(p)); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

// PackProviders are the providers of one pack: one per kind it holds, plus
// one per companion kind.
func PackProviders(provider string, ldr loader.Loader, root string, kinds contribapi.Kinds, opts ...PackOption) []*EmbedFSResourceProvider {
	var o packOptions
	for _, opt := range opts {
		opt(&o)
	}
	root = strings.TrimSuffix(root, "/")
	pk := packKindsOf(ldr, root, kinds)
	declared := pk.declared

	var out []*EmbedFSResourceProvider
	held := packLayout(ldr, root)
	// The remapped kinds first: each from the folder it names, when that
	// folder holds files; a remapped kind's own folder is not read as it.
	remapped := map[string]bool{}  // the kinds the remap moves or switches off
	elsewhere := map[string]bool{} // the first-level folders a remap reads
	names := make([]string, 0, len(o.remap))
	for kind := range o.remap {
		names = append(names, kind)
	}
	sort.Strings(names)
	for _, kind := range names {
		remapped[kind] = true
		folder := strings.Trim(o.remap[kind], "/")
		if folder == "" {
			continue // switched off
		}
		first, _, _ := strings.Cut(folder, "/")
		elsewhere[first] = true
		if !held.holds(folder) {
			continue
		}
		out = append(out, kindProviders(provider, ldr, joinPath(root, folder), declared(kind), nil)...)
	}
	for _, folder := range held.folders {
		if elsewhere[folder] {
			continue // read as the kind the remap names
		}
		d, former, ok := pk.claims(folder, held)
		if former {
			// A kind's FORMER folder, in a pack with none of its current name:
			// read, with a warning (a pack laid out before the rename).
			slog.Warn("pack: a folder of a kind's former name; rename it", "folder", joinPath(root, folder), "kind", d.Name, "now", d.SourceFolder())
		}
		if !ok || remapped[d.Name] {
			continue // no kind's folder, or a kind read elsewhere
		}
		out = append(out, kindProviders(provider, ldr, joinPath(root, folder), d, nil)...)
	}

	// Files at the root say their kind in a `type:` field.
	byKind := map[string]map[string]bool{}
	for _, rel := range held.rootFiles {
		kind := typeField(ldr, joinPath(root, rel))
		if kind == "" {
			continue
		}
		if byKind[kind] == nil {
			byKind[kind] = map[string]bool{}
		}
		byKind[kind][rel] = true
	}
	kindNames := make([]string, 0, len(byKind))
	for k := range byKind {
		kindNames = append(kindNames, k)
	}
	sort.Strings(kindNames)
	for _, kind := range kindNames {
		files := byKind[kind]
		out = append(out, kindProviders(provider, ldr, root, declared(kind), func(rel string) bool { return !files[rel] })...)
	}
	return out
}

// KindProvider is the provider of one kind's files under dir — the folder
// itself, "." for a loader rooted at it — read as the kind declares
// (KindProviders adds its companions' providers). For a binary that publishes
// one kind's folder rather than a whole pack.
func KindProvider(provider string, ldr loader.Loader, dir string, d contribapi.KindDeclaration) *EmbedFSResourceProvider {
	return kindProviders(provider, ldr, dir, d, nil)[0]
}

// KindProviders are the providers of one kind's folder and of its companions.
func KindProviders(provider string, ldr loader.Loader, dir string, d contribapi.KindDeclaration) []*EmbedFSResourceProvider {
	return kindProviders(provider, ldr, dir, d, nil)
}

// kindProviders reads one kind's files under dir: its own provider, and one
// per companion kind its source names. skip leaves out files of dir that are
// not the kind's (another kind's at a pack's root).
func kindProviders(provider string, ldr loader.Loader, dir string, d contribapi.KindDeclaration, skip func(string) bool) []*EmbedFSResourceProvider {
	left := func(rel string) bool {
		if skip != nil && skip(rel) {
			return true
		}
		return ignored(rel)
	}
	if d.Source == nil {
		return []*EmbedFSResourceProvider{{
			ProviderName: provider, ResourceType: d.Name, Loader: ldr, Root: dir,
			KeepFolderStructure: true, BodyDecoder: structuredDataLayer,
			SiblingLayers: map[string]string{contribapi.LayerTranslations: TranslationsSuffix},
			Skip:          left,
		}}
	}

	siblings := map[string]string{}
	foldOnly := map[string]bool{}
	for l, ls := range d.Source.Layers {
		if ls.From == contribapi.FromSibling {
			siblings[l] = ls.Suffix
		}
		if _, declared := d.Layers[l]; !declared && ls.Fold != "" {
			foldOnly[l] = true
		}
	}
	companions := d.Source.Companions
	metaFrom, metaFields := d.MetaFrom()
	main := &EmbedFSResourceProvider{
		ProviderName: provider, ResourceType: d.Name, Loader: ldr, Root: dir,
		Source: d.Source, SiblingLayers: siblings, FoldOnly: foldOnly,
		MediaType: d.Layers[d.DefaultLayer()].MediaType,
		MetaFrom:  metaFrom, MetaFields: metaFields,
		Skip: func(rel string) bool {
			for _, c := range companions {
				if strings.HasSuffix(rel, c.Suffix) {
					return true
				}
			}
			return left(rel)
		},
	}
	out := []*EmbedFSResourceProvider{main}
	for _, c := range companions {
		c := c
		out = append(out, &EmbedFSResourceProvider{
			ProviderName: provider, ResourceType: c.Kind, Loader: ldr, Root: dir,
			Source: &contribapi.SourceDecl{Format: contribapi.SourceDSL,
				Layers: map[string]contribapi.LayerFrom{contribapi.LayerSource: {From: contribapi.FromFile}}},
			IDTransform: func(rel string) string {
				id := main.entryIDOfStem(strings.TrimSuffix(rel, c.Suffix))
				if id == "" {
					return "" // a companion with no entry beside it publishes nothing
				}
				return strings.ReplaceAll(c.ID, "{id}", id)
			},
			Skip: func(rel string) bool { return !strings.HasSuffix(rel, c.Suffix) || left(rel) },
		})
	}
	return out
}

// ignored is a file a pack carries that is no contribution: `_`-prefixed
// (anywhere in its path) or a CWM object (`*.cwm.*`).
func ignored(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if strings.HasPrefix(seg, "_") {
			return true
		}
	}
	return strings.Contains(path.Base(rel), ".cwm.")
}

// packKindSet is the kind set a pack is read against: the binary's kinds and
// the pack's own declarations.
type packKindSet struct{ kinds, own contribapi.Kinds }

func packKindsOf(ldr loader.Loader, root string, kinds contribapi.Kinds) packKindSet {
	return packKindSet{kinds: kinds, own: ownKinds(ldr, root, kinds)}
}

// declared is a kind's declaration, the pack's own first; a kind no one
// declares is read the generic way.
func (k packKindSet) declared(kind string) contribapi.KindDeclaration {
	if d, ok := k.own.Get(kind); ok {
		return d
	}
	if d, ok := k.kinds.Get(kind); ok {
		return d
	}
	return contribapi.KindDeclaration{Name: kind}
}

// claims is the kind a first-level folder of the pack holds: the kind whose
// folder it is, else — former true — the kind it is a former folder of, when
// the pack holds none of that kind's current name. The one resolution of a
// folder to its kind, for reading a pack and for refusing a folder no kind
// reads.
func (k packKindSet) claims(folder string, held layout) (d contribapi.KindDeclaration, former, ok bool) {
	if folder == declarationsFolder(k.kinds) {
		// The pack's own kind declarations: always its entries.
		return k.declared(contribapi.ResourceTypeContributionType), false, true
	}
	if d, ok := k.own.ForFolder(folder); ok {
		return d, false, true
	}
	if d, ok := k.kinds.ForFolder(folder); ok {
		return d, false, true
	}
	d, ok = formerOf(k.kinds, k.own, folder, held)
	return d, ok, ok
}

// formerOf is the kind whose source names folder as a former folder, when the
// pack holds no folder of that kind's current name.
func formerOf(kinds, own contribapi.Kinds, folder string, held layout) (contribapi.KindDeclaration, bool) {
	for _, set := range []contribapi.Kinds{own, kinds} {
		for _, n := range set.Names() {
			d, _ := set.Get(n)
			if d.Source == nil || held.holds(d.SourceFolder()) {
				continue
			}
			for _, f := range d.Source.Former {
				if f == folder {
					return d, true
				}
			}
		}
	}
	return contribapi.KindDeclaration{}, false
}

// layout is what a pack holds: its first-level folders (on disk, or
// recorded in its layer manifest) and the files at its root, `_`-prefixed
// ones left out; and every path it holds, to say whether a nested folder has
// files.
type layout struct {
	folders, rootFiles []string
	paths              []string // every file, relative to the pack's root
}

// holds says whether the pack has a file under folder.
func (l layout) holds(folder string) bool {
	prefix := strings.Trim(folder, "/") + "/"
	for _, p := range l.paths {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// packLayout lists what a pack under root holds: the files the loader walks
// and the paths its layer manifest records (bulk content, gone from disk).
func packLayout(ldr loader.Loader, root string) layout {
	var l layout
	seen := map[string]bool{}
	add := func(rel string) {
		first, rest, nested := strings.Cut(rel, "/")
		if strings.HasPrefix(first, "_") {
			return
		}
		l.paths = append(l.paths, rel)
		if !nested {
			if !ignored(rel) {
				l.rootFiles = append(l.rootFiles, rel)
			}
			return
		}
		if rest != "" && !seen[first] {
			seen[first] = true
			l.folders = append(l.folders, first)
		}
	}
	for _, f := range ldr.Walk() {
		if rel, ok := underRoot(root, f); ok {
			add(rel)
		}
	}
	if raw, err := ldr.ReadFile(contribapi.LayerManifestFile); err == nil && raw != nil {
		if m, err := contribapi.ParseLayerManifest(raw); err == nil {
			for f := range m.Layers {
				if rel, ok := underRoot(root, f); ok && strings.Contains(rel, "/") {
					add(rel)
				}
			}
		}
	}
	sort.Strings(l.folders)
	sort.Strings(l.rootFiles)
	return l
}

// underRoot is f relative to the pack's root, when it is under it.
func underRoot(root, f string) (string, bool) {
	if root == "" || root == "." {
		return f, f != ""
	}
	if !strings.HasPrefix(f, root+"/") {
		return "", false
	}
	return strings.TrimPrefix(f, root+"/"), true
}

// joinPath is a path under the pack's root, the root "." being the loader's.
func joinPath(root, rel string) string {
	if root == "" || root == "." {
		return rel
	}
	return root + "/" + rel
}

// typeField is the `type:` a structured file at a pack's root declares; ""
// when it declares none or is not structured.
func typeField(ldr loader.Loader, full string) string {
	raw, err := ldr.ReadFile(full)
	if err != nil || raw == nil {
		return ""
	}
	data, err := structuredDataLayer(full, raw)
	if err != nil || len(data) == 0 {
		return ""
	}
	var head struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &head) != nil {
		return ""
	}
	return head.Type
}

// declarationsFolder is the pack folder holding its own kind declarations:
// the contribution-type kind's.
func declarationsFolder(kinds contribapi.Kinds) string {
	if d, ok := kinds.Get(contribapi.ResourceTypeContributionType); ok && d.SourceFolder() != "" {
		return d.SourceFolder()
	}
	return contribapi.ResourceTypeContributionType
}

// ownKinds are the kinds the pack declares itself, in the folder of the
// contribution-type kind.
func ownKinds(ldr loader.Loader, root string, kinds contribapi.Kinds) contribapi.Kinds {
	folder := declarationsFolder(kinds)
	prefix := joinPath(root, folder) + "/"
	var entries []contribapi.Contribution
	for _, f := range ldr.Walk() {
		rel, under := underRoot(root, f)
		if !strings.HasPrefix(f, prefix) || !under || ignored(rel) || strings.HasSuffix(f, TranslationsSuffix) {
			continue
		}
		raw, err := ldr.ReadFile(f)
		if err != nil || raw == nil {
			continue
		}
		data, err := structuredDataLayer(f, raw)
		if err != nil || len(data) == 0 {
			continue
		}
		c := contribapi.Contribution{Type: contribapi.ResourceTypeContributionType, ID: strings.TrimSuffix(path.Base(f), path.Ext(f))}
		c.SetData(data)
		entries = append(entries, c)
	}
	return contribapi.KindsFrom(entries)
}

// FolderManifestFile is a served folder's declaration: its name, its layout
// (`dirs:`) and its config needs — read by the generic host and by the hub
// for a folder member.
const FolderManifestFile = "ktw-host.yaml"

// RemapOfDirs turns a folder's `dirs:` into the pack reader's remap, by kind:
// a key naming a kind's folder (`skills`) is that kind, any other key a kind
// name; a key in notKinds (a folder that is no kind's: `cwm`, `scripts`) is
// left to its reader. A key that is not a valid kind token is reported by
// bad and skipped — a typo must not become a kind.
func RemapOfDirs(dirs map[string]string, kinds contribapi.Kinds, bad func(key string, err error), notKinds ...string) map[string]string {
	out := map[string]string{}
	skip := map[string]bool{}
	for _, k := range notKinds {
		skip[k] = true
	}
	for key, dir := range dirs {
		if skip[key] {
			continue
		}
		if d, ok := kinds.ForFolder(key); ok {
			out[d.Name] = dir
			continue
		}
		if err := contribapi.ValidateKebabToken("kind", key); err != nil {
			if bad != nil {
				bad(key, err)
			}
			continue
		}
		out[key] = dir
	}
	return out
}
