// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	contentlint "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/lint"
	pcapi "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
)

// The project as a write target: its contributions folder for what it
// authors as files (skills, tasks), and override objects in its
// sys-contributions module for what it changes of a sidecar's entries
// (decisions, OD-1354, OD-1359). CWM is reached through
// the CWM service - over the bus in ktw-compose (cwm/remote).

// projectCWM is what the project verbs ask of CWM.
type projectCWM interface {
	ObjectGet(ctx context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error)
	ObjectCreate(ctx context.Context, in cwmapi.ObjectCreateInput) (cwmapi.ObjectCreateOutput, error)
	ObjectUpdate(ctx context.Context, in cwmapi.ObjectUpdateInput) (cwmapi.ObjectUpdateOutput, error)
	ObjectDelete(ctx context.Context, in cwmapi.ObjectDeleteInput) (cwmapi.ObjectDeleteOutput, error)
	ObjectResolve(ctx context.Context, in cwmapi.ObjectResolveInput) (cwmapi.ObjectResolveOutput, error)
	ObjectFileRead(ctx context.Context, in cwmapi.ObjectFileReadInput) (cwmapi.ObjectFileReadOutput, error)
	ObjectFileWrite(ctx context.Context, in cwmapi.ObjectFileWriteInput) (cwmapi.ObjectFileWriteOutput, error)
	ObjectFileDelete(ctx context.Context, in cwmapi.ObjectFileDeleteInput) (cwmapi.ObjectFileDeleteOutput, error)
}

// entryReader reads an entry of the universe and one of its layers.
type entryReader interface {
	Entry(ctx context.Context, uri string) (contribapi.Contribution, error)
	Layer(ctx context.Context, uri, layer string) (content []byte, mediaType string, err error)
}

func (s *Service) cwm(ctx context.Context) (projectCWM, error) {
	if s.cwmOverride != nil {
		return s.cwmOverride, nil
	}
	svc, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return nil, fmt.Errorf("the project verbs need the CWM service: %w", err)
	}
	return svc, nil
}

func (s *Service) entries(ctx context.Context) (entryReader, error) {
	if s.entriesOverride != nil {
		return s.entriesOverride, nil
	}
	cat := s.universeOverride
	if cat == nil {
		c, err := mbapiCatalog(ctx)
		if err != nil {
			return nil, err
		}
		cat = c
	}
	return universeEntries{q: mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}}, nil
}

// universeEntries reads entries through the universe catalog.
type universeEntries struct{ q mbapi.Q }

func (u universeEntries) Entry(ctx context.Context, uri string) (contribapi.Contribution, error) {
	return mbapi.Get(ctx, u.q, uri)
}

func (u universeEntries) Layer(ctx context.Context, uri, layer string) ([]byte, string, error) {
	l, err := mbapi.Open(ctx, u.q, uri, layer)
	if err != nil {
		return nil, "", err
	}
	b, err := l.Bytes()
	return b, l.Ref.MediaType, err
}

// kinds are the kinds a sidecar's pack is read against: the universe's
// registry, else the kinds compiled into this binary.
func (s *Service) kinds(ctx context.Context) (contribapi.Kinds, error) {
	if s.kindsOverride != nil {
		return *s.kindsOverride, nil
	}
	if cat, err := mbapiCatalog(ctx); err == nil {
		if k, err := mbapi.KindsAt(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}); err == nil {
			return k, nil
		}
	}
	return builtin.Kinds()
}

// EnsureContributions gives the project its sys-contributions module when it
// has none - a system module that operates contributions - and says where its
// contributions go. A module that exists without saying what it operates is
// given it. Nothing else is written: the folder's content is the author's.
func (s *Service) EnsureContributions(ctx context.Context, _ api.EnsureContributionsInput) (api.EnsureContributionsOutput, error) {
	cwm, err := s.cwm(ctx)
	if err != nil {
		return api.EnsureContributionsOutput{}, err
	}
	out := api.EnsureContributionsOutput{Module: pcapi.ModuleID}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: pcapi.ModuleID, ID: pcapi.ModuleID})
	switch {
	case errors.Is(err, cwmapi.ErrNotFound):
		mod := cwmapi.Object{ID: pcapi.ModuleID, Type: cwmapi.SystemModuleType, Name: pcapi.ModuleID,
			Description:  "The project's own contributions: its skills, tasks, kinds and packs, published to its universe under its provider token.",
			Capabilities: map[string]map[string]any{cwmapi.CapSystemModule: {"operates": "contributions"}}}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: pcapi.ModuleID, Object: mod}); err != nil && !errors.Is(err, cwmapi.ErrAlreadyExists) {
			return api.EnsureContributionsOutput{}, fmt.Errorf("create %s: %w", pcapi.ModuleID, err)
		}
		out.Created = true
	case err != nil:
		return api.EnsureContributionsOutput{}, err
	case cwmapi.SystemModuleOperates(got.Object) != "contributions":
		o := got.Object
		if o.Capabilities == nil {
			o.Capabilities = map[string]map[string]any{}
		}
		grp := o.Capabilities[cwmapi.CapSystemModule]
		if grp == nil {
			grp = map[string]any{}
		}
		grp["operates"] = "contributions"
		o.Capabilities[cwmapi.CapSystemModule] = grp
		if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: pcapi.ModuleID, Object: o}); err != nil {
			return api.EnsureContributionsOutput{}, fmt.Errorf("update %s: %w", pcapi.ModuleID, err)
		}
	}
	f, _, err := s.projectFile(ctx, cwm)
	if err != nil {
		return api.EnsureContributionsOutput{}, err
	}
	out.Provider, out.Folder = f.Contributions.Provider, f.Contributions.Root
	return out, nil
}

// projectFile reads the settings of the project holding sys-contributions.
func (s *Service) projectFile(ctx context.Context, cwm projectCWM) (projectfile.File, string, error) {
	r, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: pcapi.ModuleID})
	if err != nil {
		return projectfile.File{}, "", err
	}
	f, err := projectfile.Read(r.ProjectRoot)
	return f, r.ProjectRoot, err
}

// projectFolder ensures sys-contributions and returns the folder projectfile
// names for the project's contributions, with the follow-up note.
func (s *Service) projectFolder(ctx context.Context) (string, string, error) {
	if _, err := s.EnsureContributions(ctx, api.EnsureContributionsInput{}); err != nil {
		return "", "", err
	}
	cwm, err := s.cwm(ctx)
	if err != nil {
		return "", "", err
	}
	f, root, err := s.projectFile(ctx, cwm)
	if err != nil {
		return "", "", err
	}
	note := fmt.Sprintf("written to the project's contributions folder (%s) - served to the universe under the token %s once the hub reads it again", f.Contributions.Root, f.Contributions.Provider)
	return filepath.Join(root, filepath.FromSlash(f.Contributions.Root)), note, nil
}

// CreateOverride makes the override object of an entry of a sidecar in the
// project's sys-contributions: only the layers named (all the entry has by
// default), each a file seeded from the entry's current layer.
func (s *Service) CreateOverride(ctx context.Context, in api.CreateOverrideInput) (api.CreateOverrideOutput, error) {
	if in.Target != "" && in.Target != targetProject {
		return api.CreateOverrideOutput{}, fmt.Errorf("create-override: invalid target %q (want %q)", in.Target, targetProject)
	}
	uri, err := contribapi.ParseResourceURI(strings.TrimSpace(in.URI))
	if err != nil {
		return api.CreateOverrideOutput{}, fmt.Errorf("create-override: %w", err)
	}
	src, err := s.entries(ctx)
	if err != nil {
		return api.CreateOverrideOutput{}, err
	}
	e, err := src.Entry(ctx, uri.String())
	if err != nil {
		return api.CreateOverrideOutput{}, fmt.Errorf("create-override: %w", err)
	}
	names := in.Layers
	if len(names) == 0 {
		for name := range e.Layers {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	content := map[string]layerContent{}
	for _, name := range names {
		if _, ok := e.Layers[name]; !ok {
			return api.CreateOverrideOutput{}, fmt.Errorf("create-override: %s has no layer %q (it has %s)", uri, name, strings.Join(layerKeys(e.Layers), ", "))
		}
		b, media, err := src.Layer(ctx, uri.String(), name)
		if err != nil {
			return api.CreateOverrideOutput{}, fmt.Errorf("create-override: %s layer %s: %w", uri, name, err)
		}
		if media == "" {
			media = e.Layers[name].MediaType
		}
		content[name] = layerContent{bytes: b, mediaType: media}
	}
	return s.writeOverride(ctx, uri, content)
}

type layerContent struct {
	bytes     []byte
	mediaType string
}

// writeOverride creates the override object of uri in sys-contributions with
// the layers given, each written as a file of the module - the shape the
// cwm project migration writes (pcapi.OverrideObject).
func (s *Service) writeOverride(ctx context.Context, uri contribapi.ResourceURI, layers map[string]layerContent) (api.CreateOverrideOutput, error) {
	if len(layers) == 0 {
		return api.CreateOverrideOutput{}, fmt.Errorf("%s: no layer to override", uri)
	}
	ens, err := s.EnsureContributions(ctx, api.EnsureContributionsInput{})
	if err != nil {
		return api.CreateOverrideOutput{}, err
	}
	if uri.Provider == ens.Provider {
		return api.CreateOverrideOutput{}, fmt.Errorf("%s is an entry of the project itself: edit its file in %s, an override is of a sidecar's entry", uri, ens.Folder)
	}
	cwm, err := s.cwm(ctx)
	if err != nil {
		return api.CreateOverrideOutput{}, err
	}
	conv := pcapi.ConvertedOverride{Overrides: uri.String(), Kind: uri.Type, ID: uri.ID, Layers: map[string]string{}, Files: map[string][]byte{}}
	for name, l := range layers {
		file := pcapi.OverrideLayerFile("overrides/"+uri.Provider, uri.Type, uri.ID, name, l.mediaType)
		conv.Layers[name] = pcapi.SourceFiles + file
		conv.Files[file] = l.bytes
	}
	obj := pcapi.OverrideObject(uri.Provider, conv)
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: pcapi.ModuleID, Object: obj})
	if errors.Is(err, cwmapi.ErrAlreadyExists) {
		return api.CreateOverrideOutput{}, fmt.Errorf("%s is already overridden by %s.%s: edit its files, or promote it", uri, pcapi.ModuleID, obj.ID)
	}
	if err != nil {
		return api.CreateOverrideOutput{}, fmt.Errorf("override object of %s: %w", uri, err)
	}
	id := created.Object.ID
	if id == "" {
		id = pcapi.ModuleID + "." + obj.ID
	}
	files := make([]string, 0, len(conv.Files))
	for f := range conv.Files {
		files = append(files, f)
	}
	sort.Strings(files)
	// A failed file write takes back what was written and the object: no
	// override object is left naming a missing file.
	for i, f := range files {
		if _, err := cwm.ObjectFileWrite(ctx, cwmapi.ObjectFileWriteInput{ID: id, Path: f, Content: conv.Files[f]}); err != nil {
			werr := fmt.Errorf("override object of %s: file %s: %w", uri, f, err)
			for _, done := range files[:i] {
				if _, derr := cwm.ObjectFileDelete(ctx, cwmapi.ObjectFileDeleteInput{ID: id, Path: done}); derr != nil {
					return api.CreateOverrideOutput{}, fmt.Errorf("%w; removing its file %s failed too (remove %s by hand): %v", werr, done, id, derr)
				}
			}
			if _, derr := cwm.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{Module: pcapi.ModuleID, ID: strings.TrimPrefix(id, pcapi.ModuleID+".")}); derr != nil {
				return api.CreateOverrideOutput{}, fmt.Errorf("%w; removing %s failed too (remove it by hand): %v", werr, id, derr)
			}
			return api.CreateOverrideOutput{}, werr
		}
	}
	return api.CreateOverrideOutput{Object: id, Overrides: uri.String(), Layers: conv.Layers,
		Note: fmt.Sprintf("edit the layer files in %s; promote when ready: ktw-compose promote --object %s", pcapi.ModuleID, id)}, nil
}

// promoteObject writes the layers of a project override object into the
// files of app/<sidecar>/embed/ they are read from, then removes the object
// and its files. A layer read from part of a file (a frontmatter kind's data
// or instructions) is refused, naming it: nothing is written then.
func (s *Service) promoteObject(ctx context.Context, ref string) (api.PromoteOutput, error) {
	cwm, err := s.cwm(ctx)
	if err != nil {
		return api.PromoteOutput{}, err
	}
	module, local, ok := strings.Cut(ref, ".")
	if !ok {
		module, local = pcapi.ModuleID, ref
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: local})
	if err != nil {
		return api.PromoteOutput{}, fmt.Errorf("promote: %s: %w", ref, err)
	}
	c, has, err := pcapi.ContributesOf(got.Object)
	if err != nil {
		return api.PromoteOutput{}, fmt.Errorf("promote: %w", err)
	}
	if !has || c.Overrides == "" {
		return api.PromoteOutput{}, fmt.Errorf("promote: %s is no override object (no %s capability with overrides)", ref, pcapi.CapContributes)
	}
	uri, err := contribapi.ParseResourceURI(c.Overrides)
	if err != nil {
		return api.PromoteOutput{}, fmt.Errorf("promote: %s overrides: %w", ref, err)
	}
	dir, err := s.sidecarDir(uri.Provider)
	if err != nil {
		return api.PromoteOutput{}, err
	}
	kinds, err := s.kinds(ctx)
	if err != nil {
		return api.PromoteOutput{}, err
	}
	packs, err := pcapi.PackLayerFiles(ctx, pcapi.NewTreeLoader(dir), uri.Provider, kinds)
	if err != nil {
		return api.PromoteOutput{}, fmt.Errorf("promote: app/%s: %w", uri.Provider, err)
	}
	var read map[string]pcapi.LayerFiles
	for _, k := range packs {
		if l, ok := k.Layers[uri.String()]; ok {
			read = l
			break
		}
	}
	if read == nil {
		return api.PromoteOutput{}, fmt.Errorf("promote: app/%s/embed holds no entry %s - nothing to write the override into", uri.Provider, uri)
	}
	objID := got.Object.ID
	if !strings.Contains(objID, ".") {
		objID = module + "." + objID
	}
	// Every layer checked and read before anything is written.
	type write struct{ file, objFile string }
	var writes []write
	content := map[string][]byte{}
	for _, name := range layerKeysOf(c.Layers) {
		srcFile, ok := strings.CutPrefix(c.Layers[name], pcapi.SourceFiles)
		if !ok {
			return api.PromoteOutput{}, fmt.Errorf("promote: %s layer %s comes from %s, not a file of its module - promote only moves layer files", ref, name, c.Layers[name])
		}
		_, withSource := c.Layers[contribapi.LayerSource]
		target, err := layerTarget(read, name, withSource)
		if err != nil {
			return api.PromoteOutput{}, fmt.Errorf("promote: %s: %w", uri, err)
		}
		if target == "" { // derived from the source the object also carries
			writes = append(writes, write{objFile: srcFile})
			continue
		}
		b, err := cwm.ObjectFileRead(ctx, cwmapi.ObjectFileReadInput{ID: objID, Path: srcFile})
		if err != nil {
			return api.PromoteOutput{}, fmt.Errorf("promote: %s layer %s: %w", ref, name, err)
		}
		content[target] = b.Content
		writes = append(writes, write{file: target, objFile: srcFile})
	}
	out := api.PromoteOutput{Removed: objID}
	// Each file's previous content is kept: a failed write restores every
	// file written so far and leaves the object in place - the source is
	// never left partly promoted.
	type previous struct {
		path   string
		before []byte
		had    bool
	}
	var done []previous
	restore := func(cause error) error {
		var failed []string
		for i := len(done) - 1; i >= 0; i-- {
			p := done[i]
			var err error
			if p.had {
				err = os.WriteFile(p.path, p.before, 0o644)
			} else if rerr := os.Remove(p.path); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
				err = rerr
			}
			if err != nil {
				failed = append(failed, p.path)
			}
		}
		if len(failed) > 0 {
			return fmt.Errorf("%w; restoring %s failed (restore them from git)", cause, strings.Join(failed, ", "))
		}
		return fmt.Errorf("%w (the files written before it are restored, %s is kept)", cause, objID)
	}
	for _, w := range writes {
		if w.file == "" {
			continue
		}
		dst := filepath.Join(dir, filepath.FromSlash(w.file))
		before, rerr := os.ReadFile(dst)
		if rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			return api.PromoteOutput{}, restore(fmt.Errorf("promote: read %s: %w", dst, rerr))
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return api.PromoteOutput{}, restore(fmt.Errorf("promote: %w", err))
		}
		done = append(done, previous{path: dst, before: before, had: rerr == nil})
		if err := promoteWriteFile(dst, content[w.file], 0o644); err != nil {
			return api.PromoteOutput{}, restore(fmt.Errorf("promote: write %s: %w", dst, err))
		}
		out.Written = append(out.Written, dst)
	}
	if len(out.Written) > 0 {
		out.Source = out.Written[0]
	}
	for _, w := range writes {
		if _, err := cwm.ObjectFileDelete(ctx, cwmapi.ObjectFileDeleteInput{ID: objID, Path: w.objFile}); err != nil {
			return out, fmt.Errorf("written to source, but removing %s's file %s failed (remove the object by hand - it would shadow the source): %w", objID, w.objFile, err)
		}
	}
	if _, err := cwm.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{Module: module, ID: local}); err != nil {
		return out, fmt.Errorf("written to source, but removing %s failed (remove it by hand - it would shadow the source): %w", objID, err)
	}
	out.Note = joinNotes(uri.Provider, "override object removed - the embedded copy is authoritative again; open a PR for app/"+uri.Provider+"/embed/")
	return out, nil
}

// promoteWriteFile writes one file of a sidecar's embed/ (a seam a test
// replaces to make a write fail).
var promoteWriteFile = os.WriteFile

// layerTarget is the file of the pack one layer is written into: the file
// it is read from whole; for a translations layer the pack does not carry
// yet, the sibling <name>.i18n.yaml of the entry's file. A layer derived from
// the source file is skipped ("") when the object carries the source too -
// the source written whole holds it; else it is refused, naming the file.
func layerTarget(read map[string]pcapi.LayerFiles, layer string, withSource bool) (string, error) {
	if lf, ok := read[layer]; ok {
		if !lf.Whole && withSource && len(lf.Files) == 1 {
			if src, ok := read[contribapi.LayerSource]; ok && len(src.Files) == 1 && src.Files[0] == lf.Files[0] {
				return "", nil
			}
		}
		if !lf.Whole || len(lf.Files) != 1 {
			return "", fmt.Errorf("layer %s is read from part of %s - edit that file in the source by hand, promote moves whole files only", layer, strings.Join(lf.Files, ", "))
		}
		return lf.Files[0], nil
	}
	if layer == contribapi.LayerTranslations {
		for _, base := range []string{contribapi.LayerSource, contribapi.LayerData} {
			if lf, ok := read[base]; ok && len(lf.Files) == 1 {
				f := lf.Files[0]
				return strings.TrimSuffix(f, path.Ext(f)) + manifestcontrib.TranslationsSuffix, nil
			}
		}
	}
	return "", fmt.Errorf("the source reads no file for layer %s", layer)
}

func layerKeys(m map[string]contribapi.LayerRef) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func layerKeysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// i18nIntoProject scaffolds the missing texts of a sidecar's entries into
// the project: per entry missing texts, an override object carrying only its
// translations layer - the entry's existing languages kept, a ~ stub per
// missing text of lang added.
func (s *Service) i18nIntoProject(ctx context.Context, m *mbapi.Manifest, entries []contentlint.EntryTranslation, lang string, out *api.I18nOutput) error {
	byURI := map[string]contribapi.Contribution{}
	for _, c := range m.Contributions {
		byURI[c.ResolvedURI()] = c
	}
	for _, e := range entries {
		if len(e.Missing) == 0 {
			continue
		}
		uri, err := contribapi.ParseResourceURI(e.Entry)
		if err != nil {
			return fmt.Errorf("i18n: %s: %w", e.Entry, err)
		}
		existing, err := s.existingTranslations(ctx, byURI[e.Entry], e.Entry)
		if err != nil {
			return err
		}
		b, err := scaffoldTranslationsLayer(e, existing, lang)
		if err != nil {
			return fmt.Errorf("i18n: %s: %w", e.Entry, err)
		}
		o, err := s.writeOverride(ctx, uri, map[string]layerContent{contribapi.LayerTranslations: {bytes: b, mediaType: "application/yaml"}})
		if err != nil {
			return err
		}
		out.Scaffolded = append(out.Scaffolded, e.Entry)
		out.Objects = append(out.Objects, o.Object)
	}
	return nil
}

// existingTranslations is the entry's translations layer as the sidecar
// serves it: carried inline, else - above the inline band - read through the
// universe. A layer that cannot be read refuses the scaffold: an override
// written without its languages would drop them.
func (s *Service) existingTranslations(ctx context.Context, c contribapi.Contribution, entry string) ([]byte, error) {
	ref, ok := c.Layers[contribapi.LayerTranslations]
	if !ok || len(ref.Inline) > 0 || ref.Size == 0 {
		return ref.Inline, nil
	}
	er, err := s.entries(ctx)
	if err == nil {
		var b []byte
		if b, _, err = er.Layer(ctx, entry, contribapi.LayerTranslations); err == nil {
			return b, nil
		}
	}
	return nil, fmt.Errorf("i18n: %s: its translations layer is not carried inline and could not be read from the universe (%v) - nothing written, its languages would be lost", entry, err)
}

// scaffoldTranslationsLayer is the translations layer existing (YAML or
// JSON, every language) with a ~ stub per missing text of lang: the one
// scaffold (contentlint.ScaffoldTranslations) run on a scratch copy.
func scaffoldTranslationsLayer(e contentlint.EntryTranslation, existing []byte, lang string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "compose-i18n-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	entry := filepath.Join(dir, "entry.yaml")
	if err := os.WriteFile(entry, nil, 0o644); err != nil {
		return nil, err
	}
	tr := strings.TrimSuffix(entry, ".yaml") + manifestcontrib.TranslationsSuffix
	if t := bytes.TrimSpace(existing); len(t) > 0 && string(t) != "null" {
		if err := os.WriteFile(tr, existing, 0o644); err != nil {
			return nil, err
		}
	}
	e.Entry = entry
	if _, err := contentlint.ScaffoldTranslations(e, lang); err != nil {
		return nil, err
	}
	return os.ReadFile(tr)
}
