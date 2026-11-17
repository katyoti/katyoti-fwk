// SPDX-License-Identifier: Apache-2.0

// Package contrib holds the generic contribution PRODUCERS: the file-backed
// provider and the per-kind helpers that wire it.
//
// It is a LEAF — the contribution contract, a loader and the extension
// mechanism, nothing else. Publishing content must not link the runtime that
// consumes it, and since the linker's unit is the package, producers cannot
// share a package with a runtime.
//
// EmbedFSResourceProvider is the canonical generic ContributionProvider: it
// scans a folder in an embedded loader (fs.FS or filesystem) and emits
// one Contribution per file. Parameterized by ResourceType, Root, and
// KeepFolderStructure — adding a new resource type is changing the
// parameters, not writing a new provider.
package contrib

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path"
	"sort"
	"strings"
	"sync"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// EmbedFSResourceProvider is the canonical generic ContributionProvider.
// It enumerates files under Root in Loader and emits one Contribution per
// file (or per folder, depending on KeepFolderStructure semantics).
type EmbedFSResourceProvider struct {
	// ProviderName is the binary that hosts these resources.
	ProviderName string

	// ResourceType is the discriminator written into Contribution.Type.
	ResourceType string

	// Loader provides the underlying file access (typically NewEmbedFSLoader).
	Loader loader.Loader

	// Root is the path within the loader to scan (e.g., "embed/skills").
	Root string

	// KeepFolderStructure controls the ID shape:
	//   false → ID is the basename without extension (skills, tasks: flat)
	//   true  → ID is the full path under Root (references, templates: tree)
	KeepFolderStructure bool

	// KeepExtension preserves the file extension in the derived ID. Default
	// (false) strips the trailing single extension. Set true for resource
	// types where the extension is identity-bearing (references can be .md,
	// .pdf, .png — the agent addresses them by full filename).
	KeepExtension bool

	// FileExtensions, if non-empty, restricts enumeration to files with one
	// of these extensions (without the dot, e.g. ["md", "yaml"]).
	// Empty = include all files.
	FileExtensions []string

	// BodyDecoder, if set, transforms a file's bytes into the typed body
	// carried as the entry's data layer (SetData). If nil, the file's full
	// content is carried as a JSON string (after escaping). Most callers set this for
	// resource types with structured bodies.
	BodyDecoder func(itemPath string, fileContent []byte) (json.RawMessage, error)

	// IDTransform, if set, derives the ID written into Contribution.ID in
	// place of deriveID. Used by resource types with multi-part extensions
	// (e.g. `.dsl.yaml`) that the default single-extension strip mishandles.
	// Receives the file's path under Root, extension included; returns the
	// final ID.
	IDTransform func(rel string) string

	// Source, when set, is the kind's `source:` block: it decides which
	// files are entries (match), their ids (id) and how a file becomes its
	// layers (format, layers) — the pack provider reads every kind through
	// it. Its sibling layers are given in SiblingLayers. KeepFolderStructure,
	// KeepExtension and BodyDecoder are not used with it; IDTransform, when
	// set, still derives the id (a companion's).
	Source *contribapi.SourceDecl

	// Skip, if set, leaves out a file (by its path under Root) that is part
	// of another entry: a companion of another kind, a sidecar file.
	Skip func(rel string) bool

	// MediaType, when set, is the media type the index records for an entry
	// read through Source: how the entry renders — the kind's default
	// layer's. Unset: the file's.
	MediaType string

	// FoldOnly names the sibling layers that are folded into data and not
	// published as layers (a part the kind does not declare as a layer).
	FoldOnly map[string]bool

	// MetaFrom and MetaFields are the kind's derived picker fields (its
	// `meta.from` layer and `meta.fields`), filled here by the reader that
	// holds the declaration: an entry of a kind its binary does not declare
	// (a foreign kind, whose builder cannot derive it) is listed with them
	// too. The builder derives the same fields again for a kind it knows.
	MetaFrom   string
	MetaFields []string

	// DropLog records the files met and not published, with a `source:`
	// block: one that cannot be read as its kind says, or that names no id.
	// Skipping keeps the enumeration whole; saying so is what packaging
	// reads (Manifest.Drops).
	contribapi.DropLog
	contribapi.OverrideLog

	// MetadataDecoder, if set, derives the body-free ContributionMeta
	// for an entry from its relative path + raw bytes. Lets file-backed
	// providers (references, config-files, data-types) promote a
	// MediaType (and any other picker-level field) into the index plane
	// so the GUI can render without fetching the body. Returns nil to
	// leave Metadata unset.
	MetadataDecoder func(itemPath string, fileContent []byte) *contribapi.ContributionMeta

	// SiblingLayers maps a layer name to a file extension (".html"): the file
	// of the same name with that extension, beside an entry's file, is carried
	// as that layer of the entry, and is not an entry itself. An entry without
	// the sibling simply lacks the layer.
	SiblingLayers map[string]string

	// memo for an immutable source; unused when the source is mutable. The
	// memoised enumeration IS the index Open answers from: byID maps an id
	// to its position in cached.
	once      sync.Once
	cached    []contribapi.Contribution
	cachedErr error
	byID      map[string]int
	epoch     contribapi.ImmutableEpoch

	// addresses remembers what a bulk-band file hashed to, by what it was
	// streamed at. A mutable source is re-walked on every enumeration, and
	// re-streaming a gigabyte to learn an address the file's stamp already
	// vouches for is the cost this exists to avoid.
	addrMu    sync.Mutex
	addresses map[string]bulkAddress
}

// bulkAddress is a bulk-band file's source layer and the stamp it was
// streamed at. A file whose stamp still matches has the same address.
type bulkAddress struct {
	stat loader.FileStat
	ref  contribapi.LayerRef
}

// Type implements ContributionProvider.
func (p *EmbedFSResourceProvider) Type() string { return p.ResourceType }

// Provider implements ContributionProvider.
func (p *EmbedFSResourceProvider) Provider() string { return p.ProviderName }

// SourceLoader returns the underlying file source. Lets consumers (e.g.
// SkillManager) reach the loader for runtime ops without round-tripping
// through Enumerate + Open. Satisfies the consumer-defined `loaderBacked`
// interface in skills/internal/core.
func (p *EmbedFSResourceProvider) SourceLoader() loader.Loader { return p.Loader }

// SourceRoot returns the root path within the loader (e.g., "embed/references").
// Paired with SourceLoader for consumers that need to do their own walking.
func (p *EmbedFSResourceProvider) SourceRoot() string { return p.Root }

// Enumerate walks the loader and returns one Contribution per matching file.
//
// IDs are derived per KeepFolderStructure:
//   - false: relative path under Root, ext-stripped, slashes collapsed to '-'
//   - true:  relative path under Root, ext-stripped if a single ext applies
func (p *EmbedFSResourceProvider) Enumerate(ctx context.Context) ([]contribapi.Contribution, error) {
	return p.enumerateOnce(ctx)
}

// check refuses a provider that cannot walk anything.
func (p *EmbedFSResourceProvider) check() error {
	if p.Loader == nil {
		return fmt.Errorf("EmbedFSResourceProvider %q: nil Loader", p.ResourceType)
	}
	if p.Root == "" {
		return fmt.Errorf("EmbedFSResourceProvider %q: empty Root", p.ResourceType)
	}
	return nil
}

func (p *EmbedFSResourceProvider) enumerate(_ context.Context) ([]contribapi.Contribution, error) {
	if err := p.check(); err != nil {
		return nil, err
	}
	p.ResetDrops()
	p.ResetOverrides()

	// Recorded descriptors, read once. A pack whose bulk content was moved to
	// the object store carries them; most packs have none and this is an
	// absent file.
	recorded, rerr := p.recordedLayers()
	if rerr != nil {
		return nil, rerr
	}

	files := withRecorded(p.listAll(), p.Root, recorded)
	out := make([]contribapi.Contribution, 0, len(files))
	for _, full := range files {
		c, ok, err := p.entryFor(full, recorded)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// idOf derives the id a file under Root publishes as, or "" when the file
// publishes nothing (wrong extension, outside Root, no id). Cheap: names
// only, no content.
func (p *EmbedFSResourceProvider) idOf(full string) (id, rel string) {
	if !p.acceptExt(full) || p.isSibling(full) {
		return "", ""
	}
	rel, err := relPath(p.Root, full)
	if err != nil {
		return "", ""
	}
	if p.Skip != nil && p.Skip(rel) {
		return "", ""
	}
	if p.Source != nil {
		if !sourceMatches(p.Source, rel) || sourceSkips(p.Source, rel) || p.claimedByAnother(rel) {
			return "", ""
		}
		id = sourceID(p.Source, rel)
		if byName(p.Source) {
			id = rel // provisional: the entry's data names it (entryFor)
		}
		if p.IDTransform != nil {
			id = p.IDTransform(rel)
		}
		return id, rel
	}
	id = p.deriveID(rel)
	if p.IDTransform != nil {
		id = p.IDTransform(rel)
	}
	return id, rel
}

// entryFor builds the one Contribution a file publishes. ok is false when
// the file publishes none: wrong extension, no id, or nothing readable there
// — a missing entry is better than a failed enumeration, and a directory or
// an unreadable file is not content.
func (p *EmbedFSResourceProvider) entryFor(full string, recorded map[string]contribapi.LayerRef) (contribapi.Contribution, bool, error) {
	id, rel := p.idOf(full)
	if id == "" {
		return contribapi.Contribution{}, false, nil
	}
	// An override the loader refused that has no embedded copy to fall back
	// to (an untrusted project's new task): nothing is published, the
	// refusal is said.
	if r, ok := p.Loader.(loader.RefusalSource); ok {
		if why := r.Refusal(full); why != "" && !p.Loader.Exists(full) {
			p.dropFile(full, why)
			return contribapi.Contribution{}, false, nil
		}
	}

	// Size decides everything that follows. A file in the bulk band is
	// never read: decoding a body out of a multi-gigabyte file is not a
	// thing to attempt, and its content IS the layer rather than something
	// the index summarises.
	ref, bulk, lerr := p.layerFor(full, recorded)
	if lerr != nil {
		return contribapi.Contribution{}, false, lerr
	}
	if ref.Hash == "" {
		return contribapi.Contribution{}, false, nil
	}

	var body json.RawMessage
	var meta *contribapi.ContributionMeta
	var read map[string]readLayer
	if !bulk && p.Source != nil {
		content, rerr := p.Loader.ReadFile(full)
		if rerr != nil {
			return contribapi.Contribution{}, false, fmt.Errorf("read %s: %w", full, rerr)
		}
		var err error
		if body, read, err = readSource(p.Source, rel, content); err != nil {
			p.dropFile(full, err.Error())
			return contribapi.Contribution{}, false, nil
		}
		if !dataMatches(body, p.Source.Where) {
			return contribapi.Contribution{}, false, nil // another kind's file: not an entry, not a drop
		}
		if byName(p.Source) {
			id = dataName(body)
			if id == "" && p.Source.ID == contribapi.SourceIDNameFile {
				// Named by its file: the name is written into its data, which
				// is where every reader looks for it.
				id = path.Base(strings.TrimSuffix(rel, path.Ext(rel)))
				if named, err := foldInto(body, "name", id); err == nil {
					body = named
				}
			}
			if id == "" {
				p.dropFile(full, "its data names no `name`, which is its id")
				return contribapi.Contribution{}, false, nil
			}
		}
		// A host-relative URI in the data (`:/skill/x`) names this provider.
		body = ResolveHostURIs(p.ProviderName, body)
		if p.MetadataDecoder != nil {
			meta = p.MetadataDecoder(rel, content)
		} else {
			// The index says how the entry renders; the picker fields are
			// the kind's (derived from data when it declares meta.from).
			mt := p.MediaType
			if mt == "" {
				mt = detectMediaType(rel)
			}
			meta = &contribapi.ContributionMeta{MediaType: mt}
		}
	} else if !bulk && (p.BodyDecoder != nil || p.MetadataDecoder != nil) {
		content, rerr := p.Loader.ReadFile(full)
		if rerr != nil {
			return contribapi.Contribution{}, false, fmt.Errorf("read %s: %w", full, rerr)
		}
		if p.BodyDecoder != nil {
			var err error
			body, err = p.BodyDecoder(rel, content)
			if err != nil {
				return contribapi.Contribution{}, false, fmt.Errorf("decode %s/%s: %w", p.ResourceType, id, err)
			}
		}
		if p.MetadataDecoder != nil {
			meta = p.MetadataDecoder(rel, content)
		}
	}

	c := contribapi.Contribution{
		Provider: p.ProviderName,
		Type:     p.ResourceType,
		ID:       id,
		Metadata: meta,
		// Every file-backed contribution has a `source` layer: the bytes on
		// disk, addressed by hash. Declaring it here means a consumer can fetch
		// the readable content WITHOUT the index having to carry it — which is
		// what lets a body stop inlining markdown, and what makes content larger
		// than a message payload representable at all.
		Layers: map[string]contribapi.LayerRef{contribapi.LayerSource: ref},
	}
	// After the source layer, never before: SetData adds to the map, while
	// assigning a fresh map replaces it — and a contribution that lost its
	// structured content to a layer assignment is a listing that shows
	// nothing.
	for layer, l := range read {
		if p.FoldOnly[layer] {
			continue // folded into data, not a layer of its own
		}
		c.Layers[layer] = p.sourceLayer(l.raw, l.name)
	}
	for layer, ext := range p.SiblingLayers {
		sibling := strings.TrimSuffix(full, path.Ext(full)) + ext
		raw := p.rawBytes(sibling)
		if raw == nil {
			if p.FoldOnly[layer] {
				// A part folded into data is part of the entry: without it
				// the entry is incomplete, not merely short of a layer.
				p.dropFile(full, "missing "+sibling)
				return contribapi.Contribution{}, false, nil
			}
			continue
		}
		if p.Source != nil {
			if fold := p.Source.Layers[layer].Fold; fold != "" {
				folded, err := foldInto(body, fold, string(raw))
				if err != nil {
					p.dropFile(full, err.Error())
					return contribapi.Contribution{}, false, nil
				}
				body = folded
			}
			if p.FoldOnly[layer] {
				continue
			}
		}
		// A YAML sibling is structure, like the entry's own YAML: it is
		// published as canonical JSON, so every reader (the GUI, an agent)
		// decodes it without a YAML parser. Unparseable YAML is published as
		// written, for the lint to name.
		if low := strings.ToLower(sibling); strings.HasSuffix(low, ".yaml") || strings.HasSuffix(low, ".yml") {
			if j, err := yamlDocToJSON(sibling, raw); err == nil {
				c.Layers[layer] = p.sourceLayer(j, strings.TrimSuffix(sibling, path.Ext(sibling))+".json")
				continue
			}
		}
		c.Layers[layer] = p.sourceLayer(raw, sibling)
	}
	c.SetData(body)
	if err := contribapi.ApplyMetaFrom(&c, p.MetaFrom, p.MetaFields); err != nil {
		p.dropFile(full, err.Error())
		return contribapi.Contribution{}, false, nil
	}
	p.noteOverrides(c, full)
	return c, true, nil
}

// noteOverrides records which of the entry's files an override tree served
// (its source and the layers read from it, then each sibling layer), and
// reports an override the loader refused (an untrusted project's executable
// content) as a drop: the embedded copy was published, the override was not.
func (p *EmbedFSResourceProvider) noteOverrides(c contribapi.Contribution, full string) {
	origins, _ := p.Loader.(loader.OriginSource)
	refusals, _ := p.Loader.(loader.RefusalSource)
	if origins == nil && refusals == nil {
		return
	}
	uri := c.ResolvedURI()
	files := map[string]string{full: contribapi.LayerSource}
	for layer, ext := range p.SiblingLayers {
		files[strings.TrimSuffix(full, path.Ext(full))+ext] = layer
	}
	for file, layer := range files {
		if refusals != nil {
			if why := refusals.Refusal(file); why != "" {
				p.dropFile(file, why)
			}
		}
		if origins == nil {
			continue
		}
		from := origins.Origin(file)
		if from == "" {
			continue
		}
		if file == full {
			// The source file carries the entry's data and every layer read
			// from it: all of them are the override's.
			for l := range c.Layers {
				if _, sibling := p.SiblingLayers[l]; !sibling {
					p.RecordOverride(contribapi.Override{URI: uri, Layer: l, From: from})
				}
			}
			continue
		}
		if _, ok := c.Layers[layer]; ok {
			p.RecordOverride(contribapi.Override{URI: uri, Layer: layer, From: from})
		}
	}
}

// isSibling reports whether a file is a sibling layer of another entry (its
// name ends with a suffix SiblingLayers names — a whole suffix, so
// `welcome.i18n.yaml` is the `.i18n.yaml` sibling of `welcome.yaml` and not a
// `.yaml` entry of its own), so it publishes no entry itself.
func (p *EmbedFSResourceProvider) isSibling(full string) bool {
	name := strings.ToLower(path.Base(full))
	for _, want := range p.SiblingLayers {
		if strings.HasSuffix(name, strings.ToLower(want)) && len(name) > len(want) {
			return true
		}
	}
	return false
}

// Open implements ContributionProvider.Open — one answer, the entry
// Enumerate carries under id.
//
// An immutable source answers from its memoised enumeration, by index. A
// mutable source is re-read, as Enumerate re-reads it, but for ONE file: the
// walk is names only, and the one file whose id matches is the only content
// touched. Re-enumerating the whole root to open one entry read every other
// file — and, for a bulk-band file, streamed its whole content — per Open.
func (p *EmbedFSResourceProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	c, err := p.entry(ctx, id)
	if err != nil {
		return nil, contribapi.LayerRef{}, err
	}
	return contribapi.OpenLayerFrom(c, layer, contribapi.LayerLanes{})
}

// entry finds the contribution published under id.
func (p *EmbedFSResourceProvider) entry(ctx context.Context, id string) (contribapi.Contribution, error) {
	uri := p.ProviderName + ":/" + p.ResourceType + "/" + id
	if !p.mutable() {
		items, err := p.enumerateOnce(ctx)
		if err != nil {
			return contribapi.Contribution{}, fmt.Errorf("open %s: %w", uri, err)
		}
		if i, ok := p.byID[id]; ok {
			return items[i], nil
		}
		return contribapi.Contribution{}, contribapi.ErrContributionNotFound{URI: uri}
	}
	if err := p.check(); err != nil {
		return contribapi.Contribution{}, fmt.Errorf("open %s: %w", uri, err)
	}
	recorded, err := p.recordedLayers()
	if err != nil {
		return contribapi.Contribution{}, fmt.Errorf("open %s: %w", uri, err)
	}
	readsID := p.Source != nil && byName(p.Source)
	for _, full := range withRecorded(p.listAll(), p.Root, recorded) {
		// An id read from the data is known once the file is read.
		if got, _ := p.idOf(full); got == "" || (got != id && !readsID) {
			continue
		}
		c, ok, err := p.entryFor(full, recorded)
		if err != nil {
			return contribapi.Contribution{}, fmt.Errorf("open %s: %w", uri, err)
		}
		if ok && c.ID == id {
			return c, nil
		}
	}
	return contribapi.Contribution{}, contribapi.ErrContributionNotFound{URI: uri}
}

// rawBytes reads a file for layer addressing, returning nil when unreadable —
// a missing layer is better than a failed enumeration.
func (p *EmbedFSResourceProvider) rawBytes(full string) []byte {
	b, err := p.Loader.ReadFile(full)
	if err != nil {
		return nil
	}
	return b
}

// layerFor describes a file's `source` layer, and says whether it is bulk.
//
// The band is derived from the size, so the size has to be known BEFORE the
// content is touched — which is the whole reason `loader.Streamer` exists. A
// loader that cannot stat cannot have bulk content in it either (an embedded
// FS is already resident), so falling back to reading is correct rather than
// merely tolerable.
//
// A recorded descriptor wins over the file. After packaging writes a bulk layer
// to the object store it removes the file, so the installed pack has nothing to
// stat — the descriptor is the only remaining statement of what the layer is,
// and it was made when the content was still there to measure.
func (p *EmbedFSResourceProvider) layerFor(full string, recorded map[string]contribapi.LayerRef) (contribapi.LayerRef, bool, error) {
	// `full` is already slash-separated — the Loader's paths are glob paths,
	// not OS paths — and the manifest's keys are written the same way.
	if ref, ok := recorded[full]; ok {
		return ref, ref.IsBulk(), nil
	}

	st, streams := p.Loader.(loader.Streamer)
	if !streams {
		raw := p.rawBytes(full)
		if raw == nil {
			return contribapi.LayerRef{}, false, nil
		}
		return p.sourceLayer(raw, full), false, nil
	}

	stat, ok := st.StatFile(full)
	if !ok {
		return contribapi.LayerRef{}, false, nil
	}
	if !contribapi.BandOf(stat.Size).IsBulk() {
		raw := p.rawBytes(full)
		if raw == nil {
			return contribapi.LayerRef{}, false, nil
		}
		return p.sourceLayer(raw, full), false, nil
	}

	// Bulk: addressed by streaming, once per (path, size, mtime). The stamp
	// is the same bound every staleness check here takes: a change that
	// preserves both size and mtime is not seen.
	if ref, ok := p.knownAddress(full, stat); ok {
		return ref, true, nil
	}
	rc, err := st.OpenFile(full)
	if err != nil {
		return contribapi.LayerRef{}, false, fmt.Errorf("open %s: %w", full, err)
	}
	defer rc.Close()
	hash, n, err := contribapi.HashReader(rc)
	if err != nil {
		return contribapi.LayerRef{}, false, fmt.Errorf("hash %s: %w", full, err)
	}
	ref := contribapi.LayerRef{Hash: hash, Size: n, MediaType: detectMediaType(full)}
	p.rememberAddress(full, stat, ref)
	return ref, true, nil
}

// knownAddress answers a bulk file's address from the memo when the file's
// stamp is the one it was streamed at.
func (p *EmbedFSResourceProvider) knownAddress(full string, stat loader.FileStat) (contribapi.LayerRef, bool) {
	p.addrMu.Lock()
	defer p.addrMu.Unlock()
	a, ok := p.addresses[full]
	if !ok || a.stat.Size != stat.Size || !a.stat.ModTime.Equal(stat.ModTime) {
		return contribapi.LayerRef{}, false
	}
	return a.ref, true
}

// rememberAddress records what a bulk file hashed to at a stamp.
func (p *EmbedFSResourceProvider) rememberAddress(full string, stat loader.FileStat, ref contribapi.LayerRef) {
	p.addrMu.Lock()
	defer p.addrMu.Unlock()
	if p.addresses == nil {
		p.addresses = make(map[string]bulkAddress)
	}
	p.addresses[full] = bulkAddress{stat: stat, ref: ref}
}

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

// listAll returns every file under p.Root, recursively, at any depth.
//
// Names only: the Loader's Walk answers with files and never directories, so
// nothing is read to tell the two apart. Reading each match to see whether it
// was a directory read every file in the tree — a bulk-band file whole — on
// every enumeration, and a depth cap on the glob it replaced lost deeper files
// with no word.
//
// Ordered as a per-depth glob would order them (depth, then each path
// component), so what a provider publishes does not change order with the
// walk behind it.
func (p *EmbedFSResourceProvider) listAll() []string {
	prefix := p.Root + "/"
	var out []string
	for _, f := range p.Loader.Walk() {
		if p.Root == "." || strings.HasPrefix(f, prefix) {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return globBefore(out[i], out[j]) })
	return out
}

// globBefore orders paths as iterative glob patterns (`*`, `*/*`, …) list
// them: shallower first, then component by component.
func globBefore(a, b string) bool {
	as, bs := strings.Split(a, "/"), strings.Split(b, "/")
	if len(as) != len(bs) {
		return len(as) < len(bs)
	}
	for i := range as {
		if as[i] != bs[i] {
			return as[i] < bs[i]
		}
	}
	return false
}

func (p *EmbedFSResourceProvider) acceptExt(full string) bool {
	if len(p.FileExtensions) == 0 {
		return true
	}
	ext := strings.TrimPrefix(path.Ext(full), ".")
	for _, allowed := range p.FileExtensions {
		if ext == allowed {
			return true
		}
	}
	return false
}

// deriveID converts a relative path into an ID.
//   - flat:  "doc-create.md"          → "doc-create"
//   - flat:  "subdir/x.md"             → "subdir-x"  (slashes → hyphens)
//   - tree:  "subdir/x.md"             → "subdir/x"  (ext stripped by default)
//   - tree+KeepExtension: "subdir/x.md" → "subdir/x.md"  (extension preserved)
func (p *EmbedFSResourceProvider) deriveID(rel string) string {
	if p.KeepFolderStructure {
		if p.KeepExtension {
			return rel
		}
		// Strip ONE extension from the basename, only if the file has a dot
		// in its basename.
		dir, base := path.Split(rel)
		if dot := strings.LastIndex(base, "."); dot > 0 {
			base = base[:dot]
		}
		return strings.TrimSuffix(dir+base, "/")
	}
	// Flat: collapse path to a single token.
	// Strip extension, then replace slashes with hyphens.
	noExt := rel
	if dot := strings.LastIndex(rel, "."); dot >= 0 && !strings.Contains(rel[dot:], "/") {
		noExt = rel[:dot]
	}
	return strings.ReplaceAll(noExt, "/", "-")
}

func relPath(root, full string) (string, error) {
	if root == "." {
		return full, nil // the loader is rooted at the folder itself
	}
	if !strings.HasPrefix(full, root+"/") && full != root {
		return "", fmt.Errorf("path %q not under root %q", full, root)
	}
	rel := strings.TrimPrefix(full, root+"/")
	return rel, nil
}

func detectMediaType(file string) string {
	if mt := render.MediaTypeForPath(file); mt != "" {
		return mt
	}
	return render.MediaTypeOctet
}

// Identity reports whether this provider's source can change — delegated to
// the loader, which is the thing that actually knows — and, for an immutable
// source, names the version it serves: the digest of the (memoised)
// enumeration, computed once with it. A mutable source carries no digest.
func (p *EmbedFSResourceProvider) Identity() contribapi.Identity {
	if p.mutable() {
		return contribapi.Identity{Mutable: true}
	}
	return p.epoch.Identity(p)
}

// mutable is the loader's answer, asked directly so the memo below never
// re-enters Identity (which enumerates to compute the digest).
func (p *EmbedFSResourceProvider) mutable() bool { return !loader.IsImmutable(p.Loader) }

// Enumerate memoises its result for an immutable source: content compiled into
// the binary cannot change, so walking it more than once per process is pure
// waste. The same walk stamps the epoch and builds the id index Open reads. A
// mutable source is re-walked every time — correctness first.
func (p *EmbedFSResourceProvider) enumerateOnce(ctx context.Context) ([]contribapi.Contribution, error) {
	if p.mutable() {
		return p.enumerate(ctx)
	}
	p.once.Do(func() {
		p.cached, p.cachedErr = p.enumerate(ctx)
		if p.cachedErr != nil {
			return
		}
		p.epoch.Record(p.cached)
		// First occurrence wins, as a scan would: two files deriving one id
		// both enumerate, and Open answers the first.
		p.byID = make(map[string]int, len(p.cached))
		for i, c := range p.cached {
			if _, dup := p.byID[c.ID]; !dup {
				p.byID[c.ID] = i
			}
		}
	})
	return p.cached, p.cachedErr
}

// recordedLayers loads the pack's recorded descriptors through this provider's
// own Loader.
//
// Through the Loader rather than the filesystem, because a provider may be
// backed by an embedded FS with no path to read — and because the Loader's root
// IS the pack root, which is what the manifest's keys are relative to.
func (p *EmbedFSResourceProvider) recordedLayers() (map[string]contribapi.LayerRef, error) {
	raw, err := p.Loader.ReadFile(contribapi.LayerManifestFile)
	if err != nil || len(raw) == 0 {
		return nil, nil
	}
	m, err := contribapi.ParseLayerManifest(raw)
	if err != nil {
		return nil, err
	}
	return m.Layers, nil
}

// withRecorded adds the recorded paths under `root` that have no file.
//
// This is the half of the recording that matters. Packaging moves a bulk layer
// to the object store and leaves the file out of the pack — so without this the
// CONTRIBUTION disappears along with the file, and the pack ships short. A
// provider enumerates files; a recorded descriptor says "there is content here
// that is not a file any more", and it has to be enumerated on that word alone.
//
// Paths that ARE on disk are left as they are. A descriptor beside a present
// file describes it (layerFor prefers the record, which is what a sealed pack
// carrying both wants); a descriptor with no file IS the contribution.
func withRecorded(files []string, root string, recorded map[string]contribapi.LayerRef) []string {
	if len(recorded) == 0 {
		return files
	}
	have := make(map[string]bool, len(files))
	for _, f := range files {
		have[f] = true
	}
	var added []string
	prefix := strings.TrimSuffix(root, "/") + "/"
	for rel := range recorded {
		if have[rel] || !strings.HasPrefix(rel, prefix) {
			continue
		}
		added = append(added, rel)
	}
	// Sorted, so a manifest's map iteration order cannot reorder what a
	// provider publishes. Enumeration order reaches the index, and an index
	// that reordered between identical runs would churn its own digest.
	sort.Strings(added)
	return append(files, added...)
}

// sourceLayer describes a file's content and CARRIES it up to the bulk band.
//
// The provider does not decide residency. It has the bytes — it just hashed
// them — so it hands them over, and the manifest builder keeps or drops them by
// what the KIND declares (contribapi.Kinds.Resident). Deciding here meant a Go
// field set at one call site stood in for a declaration a files-only author
// could never see; an 18 KiB pack DSL that shipped as a hash with no
// bytes behind it was that field being gated on the wrong band.
//
// The bulk band is the one ceiling this respects: content no process can hold
// is not something to hold, whatever the kind says.
func (p *EmbedFSResourceProvider) sourceLayer(raw []byte, full string) contribapi.LayerRef {
	ref := contribapi.LayerRef{
		Hash:      contribapi.HashContent(raw),
		Size:      int64(len(raw)),
		MediaType: detectMediaType(full),
	}
	// A kind that DECLARES its source resident is answering "does a listing
	// need this content" — and the ceiling that can overrule it is the point
	// above which content cannot be carried at all, which is the bulk
	// boundary, not the inline one.
	//
	// Gating on the inline band instead meant a 4 KiB threshold silently
	// revoked the kind's declaration. A pack's 18 KiB DSL shipped as a hash
	// with no bytes and no fetch behind it: the hub could not register the
	// schema, and every entry of the kind then failed validation with
	// `data-type not found`. A size deciding whether a schema
	// exists is not a rule anyone wrote down.
	if contribapi.BandOf(ref.Size) != contribapi.BandNameBulk {
		ref.Inline = raw
	}
	return ref
}

// dropFile records a file this provider met and could not publish, and says
// so in the log: loud for whoever watches a universe, recorded for packaging.
func (p *EmbedFSResourceProvider) dropFile(full, reason string) {
	slog.Warn("contribution not published: file could not be read as its kind",
		"provider", p.ProviderName, "kind", p.ResourceType, "file", full, "reason", reason)
	p.Record(full, p.ResourceType, reason)
}

// claimedByAnother says whether a file lies in a folder another file claims
// (Source.Claims: TASK.md is the entry of its folder) — part of that entry,
// never one of its own. The claiming file itself is not claimed by another.
func (p *EmbedFSResourceProvider) claimedByAnother(rel string) bool {
	if p.Source == nil || p.Source.Claims == "" || path.Base(rel) == p.Source.Claims {
		return false
	}
	claimed := map[string]bool{}
	for _, full := range p.listAll() {
		if r, err := relPath(p.Root, full); err == nil && path.Base(r) == p.Source.Claims {
			claimed[path.Dir(r)] = true
		}
	}
	for d := path.Dir(rel); d != "." && d != "/" && d != ""; d = path.Dir(d) {
		if claimed[d] {
			return true
		}
	}
	return false
}

// entryIDOfStem is the id of this provider's entry whose file, under Root,
// has the given path without its extension (`review/TASK` for
// `review/TASK.md`): what its companions name in their own id. "" when no
// entry has that stem. With id: name, the entry's file is read for it.
func (p *EmbedFSResourceProvider) entryIDOfStem(stem string) string {
	for _, full := range p.listAll() {
		id, rel := p.idOf(full)
		if id == "" || strings.TrimSuffix(rel, path.Ext(rel)) != stem {
			continue
		}
		if p.Source == nil || !byName(p.Source) {
			return id
		}
		raw, err := p.Loader.ReadFile(full)
		if err != nil {
			return ""
		}
		data, _, err := readSource(p.Source, rel, raw)
		if err != nil {
			return ""
		}
		if n := dataName(data); n != "" || p.Source.ID != contribapi.SourceIDNameFile {
			return n
		}
		return path.Base(stem)
	}
	return ""
}
