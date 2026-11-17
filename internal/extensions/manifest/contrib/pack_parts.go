// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// pack_parts.go — several packs of ONE provider read as one.
//
// A binary hosts its own pack and the packs of the extensions it links, each
// an fs.FS its owner ships. They are parts of one pack: one provider per kind
// for the provider name, the parts layered in contribution order, then the
// provider's universe override tree (`overwrites/<provider>/embed/<path>`,
// gated by allow_overwrites), last wins by path. Kinds come only from the kind
// set — the system kinds and the parts' own declarations — never a list of
// names. Two parts declaring the same entry of a kind, or a part holding a
// folder no kind reads, are refused at AppInit, naming the owners: layering
// is last-wins, so either would be a silent loss. The override tree is not a
// part: overriding is its purpose.

// PackPart is one owner's part of a provider's pack.
type PackPart struct {
	Provider string // "" = the binary's own name, resolved at AppInit
	Owner    string // who ships it — named in a refusal
	Path     string // the pack's place in the binary's embed tree: its override subtree
	FS       fs.FS
}

// ContributePackPart is how an extension or a binary ships its part of a
// provider's pack. The providers are built once per provider name, on the
// first part's AppInit, over every part contributed by then.
func ContributePackPart(provider, owner, packPath string, fsys fs.FS) application.Extension {
	part := PackPart{Provider: provider, Owner: owner, Path: strings.Trim(packPath, "/"), FS: fsys}
	return func(init *application.ApplicationInit) {
		// A fresh point per application: two applications in one process (tests,
		// the census, a GUI universe core) must not share parts.
		init.AddRegisterPoints("manifest", ".pack.parts.point", func(app *application.Application) error {
			err := application.RegisterPoint(app, em.NewListPoint[PackPart]())
			if errors.As(err, new(em.ErrDuplicatePointID)) {
				return nil // another part registered it first
			}
			return err
		})
		init.AddContribute("manifest", ".pack.parts."+owner, func(app *application.Application) error {
			return application.Contribute(app, part)
		})
		// AppInit: every part is contributed by then, and app.Provider() is set.
		init.AddAppInit("manifest", ".pack.parts.providers."+owner, func(app *application.Application) error {
			provider := partProvider(app, part.Provider)
			if provider == "" {
				return fmt.Errorf("pack part %q: no provider name", owner)
			}
			if partsProvidersBuilt(app, provider) {
				return nil
			}
			return contributePartsProviders(app, provider)
		})
	}
}

// contributePartsProviders checks a provider's parts and publishes one
// provider per kind over them.
func contributePartsProviders(app *application.Application, provider string) error {
	kinds, err := contribution.SystemKinds()
	if err != nil {
		return fmt.Errorf("pack of %s: the system kinds: %w", provider, err)
	}
	parts := partsOf(app, provider)
	ldr, err := newPartsLoader(provider, parts)
	if err != nil {
		return fmt.Errorf("pack of %s: %w", provider, err)
	}
	if err := checkParts(parts, kinds); err != nil {
		return fmt.Errorf("pack of %s: %w", provider, err)
	}
	for _, p := range PackProviders(provider, ldr, ".", kinds) {
		if err := application.Contribute(app, contribapi.ContributionProvider(p)); err != nil {
			return err
		}
	}
	return nil
}

func partProvider(app *application.Application, provider string) string {
	if provider == "" {
		return app.Provider()
	}
	return provider
}

// partsOf is the parts of one provider name, in contribution order.
func partsOf(app *application.Application, provider string) []PackPart {
	var out []PackPart
	for _, p := range application.UseAppExtensions[PackPart](app) {
		if partProvider(app, p.Provider) == provider {
			out = append(out, p)
		}
	}
	return out
}

// partsProvidersBuilt says whether the providers of this provider name exist:
// the first part's AppInit builds them for every part.
func partsProvidersBuilt(app *application.Application, provider string) bool {
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		if t, ok := p.(*EmbedFSResourceProvider); ok && t.ProviderName == provider {
			if _, ours := t.Loader.(*partsLoader); ours {
				return true
			}
		}
	}
	return false
}

// checkParts refuses a folder no kind reads and an entry two parts declare,
// each naming its owners.
func checkParts(parts []PackPart, kinds contribapi.Kinds) error {
	fss := make([]fs.FS, 0, len(parts))
	for _, p := range parts {
		fss = append(fss, p.FS)
	}
	whole := layered(fss)
	pk := packKindsOf(whole, ".", kinds)
	held := packLayout(whole, ".")

	var unread, clashes []string
	seen := map[string]string{} // kind/id → owner
	for _, p := range parts {
		entries, _ := fs.ReadDir(p.FS, ".")
		for _, e := range entries {
			name := e.Name()
			if ignored(name) {
				continue
			}
			var d contribapi.KindDeclaration
			var rels []string
			if e.IsDir() {
				var ok bool
				d, _, ok = pk.claims(name, held)
				if !ok {
					unread = append(unread, fmt.Sprintf("%s/ in %s", name, p.Owner))
					continue
				}
				_ = fs.WalkDir(p.FS, name, func(fp string, de fs.DirEntry, err error) error {
					if err == nil && !de.IsDir() {
						rels = append(rels, strings.TrimPrefix(fp, name+"/"))
					}
					return nil
				})
			} else {
				kind := typeField(loader.NewEmbedFSLoader(p.FS, "."), name)
				if kind == "" {
					continue
				}
				d = pk.declared(kind)
				rels = []string{name}
			}
			for _, rel := range rels {
				id, ok := entryID(d, rel)
				if !ok {
					continue
				}
				key := d.Name + "/" + id
				if prev, dup := seen[key]; dup && prev != p.Owner {
					clashes = append(clashes, fmt.Sprintf("%s declared by both %s and %s", key, prev, p.Owner))
					continue
				}
				seen[key] = p.Owner
			}
		}
	}
	var msgs []string
	if len(unread) > 0 {
		sort.Strings(unread)
		msgs = append(msgs, "pack folders no kind reads (a kind's declaration names its folder): "+strings.Join(unread, "; "))
	}
	if len(clashes) > 0 {
		sort.Strings(clashes)
		msgs = append(msgs, "parts overlap: "+strings.Join(clashes, "; "))
	}
	if len(msgs) > 0 {
		return errors.New(strings.Join(msgs, "; "))
	}
	return nil
}

// entryID is the id a file of a kind's folder declares, by the kind's own id
// rule; false for a file that is no entry (ignored, a sibling layer, a
// companion, a file the kind does not match or skips).
func entryID(d contribapi.KindDeclaration, rel string) (string, bool) {
	if ignored(rel) {
		return "", false
	}
	s := d.Source
	if s == nil {
		if strings.HasSuffix(rel, TranslationsSuffix) {
			return "", false
		}
		return sourceID(&contribapi.SourceDecl{ID: contribapi.SourceIDPath}, rel), true
	}
	for _, ls := range s.Layers {
		if ls.From == contribapi.FromSibling && ls.Suffix != "" && strings.HasSuffix(rel, ls.Suffix) {
			return "", false
		}
	}
	for _, c := range s.Companions {
		if strings.HasSuffix(rel, c.Suffix) {
			return "", false
		}
	}
	if sourceSkips(s, rel) || !sourceMatches(s, rel) {
		return "", false
	}
	id := sourceID(s, rel)
	return id, id != ""
}

// PackOverlayFS is a provider's universe override tree of the pack at
// packPath (`<home>/overwrites/<provider>/embed/<packPath>`), when the
// universe at home allows overwrites and the folder exists; else nil. The
// pack reader and any runtime reading the same pack layer this one tree.
func PackOverlayFS(home, provider, packPath string) fs.FS {
	if home == "" || provider == "" || !cenv.AllowOverwrites(home) {
		return nil
	}
	dir := filepath.Join(cenv.OverwritesDir(home, provider), "embed", filepath.FromSlash(strings.Trim(packPath, "/")))
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil
	}
	slog.Info("pack: universe override tree active", "provider", provider, "dir", dir)
	return os.DirFS(dir)
}

// overlayHome is the universe the override tree is read from — a variable so
// a test points it at a temporary universe, never the development one.
var overlayHome = func() string { return application.Env().UniverseDir() }

// partsLoader is one provider's parts as one source: the parts in
// contribution order, then the universe override tree, found on first read
// (the universe is known by then, not when the extension list is built).
type partsLoader struct {
	provider, path string
	parts          []fs.FS
	once           sync.Once
	active         loader.Loader
	overlaid       bool
}

func newPartsLoader(provider string, parts []PackPart) (*partsLoader, error) {
	l := &partsLoader{provider: provider}
	for i, p := range parts {
		if i > 0 && p.Path != parts[0].Path {
			return nil, fmt.Errorf("parts name two pack paths: %q by %s and %q by %s", parts[0].Path, parts[0].Owner, p.Path, p.Owner)
		}
		l.path = p.Path
		l.parts = append(l.parts, p.FS)
	}
	return l, nil
}

func (l *partsLoader) resolve() loader.Loader {
	l.once.Do(func() {
		sources := append([]fs.FS(nil), l.parts...)
		if ov := PackOverlayFS(overlayHome(), l.provider, l.path); ov != nil {
			sources = append(sources, ov)
			l.overlaid = true
		}
		l.active = layered(sources)
	})
	return l.active
}

// layered reads the sources as one, a later source's file winning over an
// earlier one's of the same path.
func layered(sources []fs.FS) loader.Loader {
	var out loader.Loader
	for _, f := range sources {
		next := loader.NewEmbedFSLoader(f, ".")
		if out == nil {
			out = next
			continue
		}
		out = loader.NewOverlayLoader(out, next, nil)
	}
	if out == nil {
		return loader.NewEmbedFSLoader(emptyFS{}, ".")
	}
	return out
}

type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// Immutable: a part is the binary's embedded content, fixed for the process;
// an override tree is a folder a person edits — the source is mutable exactly
// when one is present.
func (l *partsLoader) Immutable() bool {
	l.resolve()
	return !l.overlaid
}

var _ loader.ImmutableSource = (*partsLoader)(nil)

func (l *partsLoader) ReadFile(p string) ([]byte, error) { return l.resolve().ReadFile(p) }
func (l *partsLoader) List(pattern string) []string      { return l.resolve().List(pattern) }
func (l *partsLoader) Walk() []string                    { return l.resolve().Walk() }
func (l *partsLoader) Exists(p string) bool              { return l.resolve().Exists(p) }
func (l *partsLoader) Reset(p string)                    { l.resolve().Reset(p) }
