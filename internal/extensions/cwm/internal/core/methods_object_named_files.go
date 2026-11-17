// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Named files come from three sources, closer wins — a capability's `files:`,
// the object type's, the object's own — and are addressed by (capability,
// name). ONE resolver merges them; Read / Write / List / Declare all use it,
// then go through the ObjectFile* rules (module root, `..` refused, object
// files unreachable), which are that code, not a copy of it.

// namedFileEntry is one declaration in force on an object.
type namedFileEntry struct {
	capability, name, source string
	decl                     api.NamedFile
}

func namedFileKey(capability, name string) string { return capability + "\x00" + name }

// effectiveNamedFiles merges the three sources for obj, farthest first so the
// closer declaration of a key replaces it IN PLACE — the list keeps the order
// in which keys first appeared, and a replacement says where it came from.
func (s *Service) effectiveNamedFiles(obj api.Object, capIDs []string) []namedFileEntry {
	var out []namedFileEntry
	at := map[string]int{}
	put := func(e namedFileEntry) {
		k := namedFileKey(e.capability, e.name)
		if i, ok := at[k]; ok {
			out[i] = e
			return
		}
		at[k] = len(out)
		out = append(out, e)
	}
	sortedNames := func(m map[string]api.NamedFile) []string {
		names := make([]string, 0, len(m))
		for n := range m {
			names = append(names, n)
		}
		sort.Strings(names)
		return names
	}
	declared := s.capabilitiesByName()
	for _, c := range capIDs {
		files := declared[c].Files
		for _, n := range sortedNames(files) {
			put(namedFileEntry{capability: c, name: n, source: api.NamedFileFromCapability, decl: files[n]})
		}
	}
	if t, ok := s.lookupObjectType(obj.Type); ok {
		for _, n := range sortedNames(t.Files) {
			put(namedFileEntry{capability: t.Files[n].Capability, name: n, source: api.NamedFileFromType, decl: t.Files[n]})
		}
	}
	for _, n := range sortedNames(obj.Files) {
		put(namedFileEntry{capability: obj.Files[n].Capability, name: n, source: api.NamedFileFromObject, decl: obj.Files[n]})
	}
	return out
}

// namedFileContext loads what every named-file verb needs: the object, its
// expression environment and its effective capability ids.
func (s *Service) namedFileContext(ctx stdctx.Context, id string) (api.Object, map[string]any, []string, error) {
	if id == "" {
		return api.Object{}, nil, nil, fmt.Errorf("%w: ID required", api.ErrInvalidInput)
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.Object{}, nil, nil, err
	}
	obj, ok := ix.get(id)
	if !ok {
		return api.Object{}, nil, nil, api.ObjectNotFound(id)
	}
	eff, err := s.ObjectEffectiveCapabilities(ctx, api.ObjectEffectiveCapabilitiesInput{ID: id})
	if err != nil {
		return api.Object{}, nil, nil, err
	}
	ids := make([]string, 0, len(eff.Capabilities))
	for _, c := range eff.Capabilities {
		ids = append(ids, c.ID)
	}
	return obj, s.objectActionEnv(obj), ids, nil
}

// resolveNamedFilePath turns a declaration into the module-relative path it
// names for this object.
//
// STRICT, unlike an action's input: every `${...}` must resolve to a non-empty
// value. The lenient interpolation turns an unknown name into "" —
// `memory/${current.nope}.md` became `memory/.md`, a real file nobody meant
// (caught by TestNamedFileListReportsEachDeclaration).
func resolveNamedFilePath(decl api.NamedFile, env map[string]any) (string, error) {
	if strings.TrimSpace(decl.Path) == "" {
		return "", fmt.Errorf("%w: the declaration has no path", api.ErrInvalidInput)
	}
	var b strings.Builder
	rest := decl.Path
	for {
		i := strings.Index(rest, "${")
		if i < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:i])
		rest = rest[i+2:]
		j := strings.Index(rest, "}")
		if j < 0 {
			return "", fmt.Errorf("%w: path %q has an unclosed ${", api.ErrInvalidInput, decl.Path)
		}
		v, err := objectExprs.Eval(strings.TrimSpace(rest[:j]), env)
		if err != nil || v == nil || el.AsString(v) == "" {
			return "", fmt.Errorf("%w: path %q does not resolve for this object (${%s})", api.ErrInvalidInput, decl.Path, rest[:j])
		}
		b.WriteString(el.AsString(v))
		rest = rest[j+1:]
	}
	p := b.String()
	if strings.Contains(p, "//") || strings.HasSuffix(p, "/") || strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("%w: path %q resolved to %q, which names no file", api.ErrInvalidInput, decl.Path, p)
	}
	return p, nil
}

// findNamedFile returns the declaration in force for (capability, name),
// refusing a file owned by a capability that is not in force on the object:
// a file the object has no capability for is not its file.
func findNamedFile(entries []namedFileEntry, capIDs []string, capability, name string) (namedFileEntry, error) {
	if name == "" {
		return namedFileEntry{}, fmt.Errorf("%w: name required", api.ErrInvalidInput)
	}
	if capability != "" && !slices.Contains(capIDs, capability) {
		return namedFileEntry{}, fmt.Errorf("%w: capability %q is not in force on this object", api.ErrInvalidInput, capability)
	}
	for _, e := range entries {
		if e.capability == capability && e.name == name {
			return e, nil
		}
	}
	if capability == "" {
		return namedFileEntry{}, fmt.Errorf("%w: the object declares no file %q of its own", api.ErrNotFound, name)
	}
	return namedFileEntry{}, fmt.Errorf("%w: capability %q declares no file %q here", api.ErrNotFound, capability, name)
}

// findNamedFileToRead is findNamedFile for a read, which also forgives a
// missing capability: when the object has no file of that name of its own and
// exactly ONE capability in force declares it, that file is the one meant (an
// agent asking for `working` means the memory's). Two or more stay refused —
// the caller must say which.
func findNamedFileToRead(entries []namedFileEntry, capIDs []string, capability, name string) (namedFileEntry, error) {
	e, err := findNamedFile(entries, capIDs, capability, name)
	if err == nil || capability != "" || !errors.Is(err, api.ErrNotFound) {
		return e, err
	}
	var match []namedFileEntry
	for _, c := range entries {
		if c.name == name && c.capability != "" && slices.Contains(capIDs, c.capability) {
			match = append(match, c)
		}
	}
	switch len(match) {
	case 1:
		return match[0], nil
	case 0:
		return namedFileEntry{}, err
	}
	owners := make([]string, len(match))
	for i, c := range match {
		owners[i] = c.capability
	}
	return namedFileEntry{}, fmt.Errorf("%w: the object declares no file %q of its own, and capabilities %s each do — name one", api.ErrNotFound, name, strings.Join(owners, ", "))
}

func describeNamedFile(e namedFileEntry, path string) api.ObjectNamedFile {
	return api.ObjectNamedFile{Capability: e.capability, Name: e.name, Source: e.source, Path: path,
		Media: e.decl.Media, Description: e.decl.Description}
}

// ObjectNamedFileList resolves every named file in force on the object.
func (s *Service) ObjectNamedFileList(ctx stdctx.Context, in api.ObjectNamedFileListInput) (api.ObjectNamedFileListOutput, error) {
	obj, env, capIDs, err := s.namedFileContext(ctx, in.ID)
	if err != nil {
		return api.ObjectNamedFileListOutput{}, err
	}
	_, module, err := s.objectFileRoot(in.ID)
	if err != nil {
		return api.ObjectNamedFileListOutput{}, err
	}
	out := api.ObjectNamedFileListOutput{Module: module, Files: []api.ObjectNamedFile{}}
	for _, e := range s.effectiveNamedFiles(obj, capIDs) {
		p, perr := resolveNamedFilePath(e.decl, env)
		f := describeNamedFile(e, p)
		switch {
		case perr != nil:
			f.Error = perr.Error()
		case e.capability != "" && !slices.Contains(capIDs, e.capability):
			f.Error = fmt.Sprintf("owned by capability %q, which is not in force on this object", e.capability)
		default:
			if st, serr := s.ObjectFileStat(ctx, api.ObjectFileStatInput{ID: in.ID, Path: p}); serr == nil {
				f.Exists, f.SizeBytes = true, st.Entry.SizeBytes
			} else if !errors.Is(serr, api.ErrNotFound) {
				f.Error = serr.Error() // an object-file path, `..`: the declaration is wrong
			}
		}
		out.Files = append(out.Files, f)
	}
	return out, nil
}

// ObjectNamedFileRead reads one named file; one not written yet reads empty.
func (s *Service) ObjectNamedFileRead(ctx stdctx.Context, in api.ObjectNamedFileReadInput) (api.ObjectNamedFileReadOutput, error) {
	obj, env, capIDs, err := s.namedFileContext(ctx, in.ID)
	if err != nil {
		return api.ObjectNamedFileReadOutput{}, err
	}
	e, err := findNamedFileToRead(s.effectiveNamedFiles(obj, capIDs), capIDs, in.Capability, in.Name)
	if err != nil {
		return api.ObjectNamedFileReadOutput{}, err
	}
	p, err := resolveNamedFilePath(e.decl, env)
	if err != nil {
		return api.ObjectNamedFileReadOutput{}, err
	}
	f := describeNamedFile(e, p)
	got, err := s.ObjectFileRead(ctx, api.ObjectFileReadInput{ID: in.ID, Path: p, Offset: in.Offset, Limit: in.Limit})
	if errors.Is(err, api.ErrNotFound) {
		return api.ObjectNamedFileReadOutput{File: f, Content: []byte{}}, nil
	}
	if err != nil {
		return api.ObjectNamedFileReadOutput{}, err
	}
	f.Exists, f.SizeBytes = true, got.SizeBytes
	return api.ObjectNamedFileReadOutput{File: f, Content: got.Content, IsPartial: got.IsPartial}, nil
}

// ObjectNamedFileWrite writes or appends to one named file.
func (s *Service) ObjectNamedFileWrite(ctx stdctx.Context, in api.ObjectNamedFileWriteInput) (api.ObjectNamedFileWriteOutput, error) {
	if err := s.refuseInChangeSet(ctx, "ObjectNamedFileWrite"); err != nil {
		return api.ObjectNamedFileWriteOutput{}, err
	}
	obj, env, capIDs, err := s.namedFileContext(ctx, in.ID)
	if err != nil {
		return api.ObjectNamedFileWriteOutput{}, err
	}
	e, err := findNamedFile(s.effectiveNamedFiles(obj, capIDs), capIDs, in.Capability, in.Name)
	if err != nil {
		return api.ObjectNamedFileWriteOutput{}, err
	}
	p, err := resolveNamedFilePath(e.decl, env)
	if err != nil {
		return api.ObjectNamedFileWriteOutput{}, err
	}
	w, err := s.ObjectFileWrite(ctx, api.ObjectFileWriteInput{ID: in.ID, Path: p, Content: in.Content, Append: in.Append})
	if err != nil {
		return api.ObjectNamedFileWriteOutput{}, err
	}
	f := describeNamedFile(e, p)
	f.Exists, f.SizeBytes = true, w.SizeBytes
	return api.ObjectNamedFileWriteOutput{File: f, ProjectPath: w.ProjectPath}, nil
}

// ObjectNamedFileDeclare adds or removes a named file on ONE object — its own
// `files:` map, written through ObjectUpdate so the object's validation, index
// and events apply, and its authored comments survive.
//
// The declaration is checked BEFORE it is stored: its path must resolve for
// this object and land on a reachable file, and an owning capability must be
// in force. A declaration that could never be read would be stored noise.
func (s *Service) ObjectNamedFileDeclare(ctx stdctx.Context, in api.ObjectNamedFileDeclareInput) (api.ObjectNamedFileDeclareOutput, error) {
	if err := s.refuseInChangeSet(ctx, "ObjectNamedFileDeclare"); err != nil {
		return api.ObjectNamedFileDeclareOutput{}, err
	}
	obj, env, capIDs, err := s.namedFileContext(ctx, in.ID)
	if err != nil {
		return api.ObjectNamedFileDeclareOutput{}, err
	}
	if strings.TrimSpace(in.Name) == "" {
		return api.ObjectNamedFileDeclareOutput{}, fmt.Errorf("%w: name required", api.ErrInvalidInput)
	}
	module, ok := s.index().moduleOf(in.ID)
	if !ok {
		return api.ObjectNamedFileDeclareOutput{}, fmt.Errorf("%w: object %q has no module", api.ErrNotFound, in.ID)
	}
	full := s.hydrated(obj)

	if in.Remove {
		if _, has := full.Files[in.Name]; !has {
			return api.ObjectNamedFileDeclareOutput{}, fmt.Errorf("%w: the object declares no file %q of its own", api.ErrNotFound, in.Name)
		}
		owner := full.Files[in.Name].Capability
		delete(full.Files, in.Name)
		if len(full.Files) == 0 {
			full.Files = nil
		}
		if _, err := s.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: module, Object: full}); err != nil {
			return api.ObjectNamedFileDeclareOutput{}, err
		}
		// What is in force now, if a farther declaration of the key exists.
		if e, ferr := findNamedFile(s.effectiveNamedFiles(full, capIDs), capIDs, owner, in.Name); ferr == nil {
			p, _ := resolveNamedFilePath(e.decl, env)
			return api.ObjectNamedFileDeclareOutput{File: describeNamedFile(e, p)}, nil
		}
		return api.ObjectNamedFileDeclareOutput{File: api.ObjectNamedFile{Capability: owner, Name: in.Name}}, nil
	}

	if in.File.Capability != "" && !slices.Contains(capIDs, in.File.Capability) {
		return api.ObjectNamedFileDeclareOutput{}, fmt.Errorf("%w: capability %q is not in force on %q", api.ErrInvalidInput, in.File.Capability, in.ID)
	}
	p, err := resolveNamedFilePath(in.File, env)
	if err != nil {
		return api.ObjectNamedFileDeclareOutput{}, err
	}
	if _, _, _, err := s.objectFileAbs(in.ID, p); err != nil {
		return api.ObjectNamedFileDeclareOutput{}, err // `..`, an object file
	}
	if full.Files == nil {
		full.Files = map[string]api.NamedFile{}
	}
	full.Files[in.Name] = in.File
	if _, err := s.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: module, Object: full}); err != nil {
		return api.ObjectNamedFileDeclareOutput{}, err
	}
	e := namedFileEntry{capability: in.File.Capability, name: in.Name, source: api.NamedFileFromObject, decl: in.File}
	return api.ObjectNamedFileDeclareOutput{File: describeNamedFile(e, p)}, nil
}
