// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ProjectInit mints the project id and writes the root project object
// (identity-and-index.md §7): `<project>/project.cwm.yaml`, its type the one
// asked or the default project type
// id = the project name, `project.uuid` = a UUIDv7 minted ONCE. It refuses to
// run twice — the id is identity, and re-minting it would silently turn every
// full name ever logged into a dangling reference.
func (s *Service) ProjectInit(ctx stdctx.Context, in api.ProjectInitInput) (api.ProjectInitOutput, error) {
	if s.refused != nil {
		return api.ProjectInitOutput{}, s.refused
	}
	root := s.Config.ObjectsRoot()
	if root == "" {
		return api.ProjectInitOutput{}, fmt.Errorf("%w: project root unset", api.ErrInvalidInput)
	}
	path := filepath.Join(root, api.ProjectFileName)
	if _, err := os.Stat(path); err == nil {
		return api.ProjectInitOutput{}, fmt.Errorf("%w: %s exists — a project id is minted once; edit the object, never re-init",
			api.ErrAlreadyExists, api.ProjectFileName)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = filepath.Base(filepath.Clean(root))
	}
	if err := safeSegment("name", name); err != nil {
		return api.ProjectInitOutput{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return api.ProjectInitOutput{}, fmt.Errorf("mint project id: %w", err)
	}
	title := in.Title
	if title == "" {
		title = name
	}
	// The default project type; never the retired bare project type
	//
	typ := strings.TrimSpace(in.Type)
	if typ == "" {
		typ = api.DefaultProjectType
	}
	obj := api.Object{ID: name, Type: typ, Name: title, Description: in.Description}
	for capID, group := range in.Capabilities {
		if capID != api.CapProject {
			_ = obj.SetCapability(capID, group)
		}
	}
	// A new project's data is at this binary's data version (migrate.go). A
	// project that already holds objects when its project object is minted
	// (a seed, a repository from before data versions) was written at the
	// baseline: it records none, so `cwm migrate` brings it up.
	project := map[string]any{"uuid": id.String()}
	if !s.holdsObjects() {
		project["data_version"] = dataVersionOf(s.steps)
	}
	if in.ModulesDir != "" && in.ModulesDir != api.DefaultModulesDir {
		project["modules_dir"] = in.ModulesDir
	}
	_ = obj.SetCapability(api.CapProject, project)
	// The same check every object write runs, once a pack declares the
	// `project` type. CWM writes every field of this object itself, so a
	// binary without that pack (a bare attach) still mints its project.
	if t, declared := s.lookupObjectType(typ); declared {
		if t.Level() != api.CapProject {
			return api.ProjectInitOutput{}, fmt.Errorf("%w: ProjectInit: %s is no project type (it carries no project capability)", api.ErrInvalidInput, typ)
		}
		if err := s.checkWrite(obj, nil, "ProjectInit"); err != nil {
			return api.ProjectInitOutput{}, err
		}
	}
	rev, err := writeObjectFileRev(path, obj)
	if err != nil {
		return api.ProjectInitOutput{}, err
	}
	s.dataVersion.Store(0) // the project's data version is now this file's (migrate_legacy.go)
	// The index keys on this file for the modules folder and the tree root:
	// a rescan picks it up rather than a put, because the modules folder may
	// have changed with it.
	if _, err := s.index().scan(); err != nil {
		return api.ProjectInitOutput{}, err
	}
	// The project has an id now: its events carry it from this one on - the
	// project object's own creation included - unless the service was given
	// its workspace slot.
	if s.Config.WorkspaceID == "" {
		s.Events.SetWorkspace(api.WorkspaceToken(id.String()))
	}
	s.publishObjectEvent(ctx, "", obj, false, s.eventMeta(ctx, rev))
	return api.ProjectInitOutput{Object: obj, Path: s.relPath(path)}, nil
}

// projectObject returns the root project object the index holds, if any.
func (s *Service) projectObject() (api.Object, bool) {
	ix := s.index()
	if ix.ensureLoaded() != nil {
		return api.Object{}, false
	}
	pid := ix.projectID()
	if pid == "" {
		return api.Object{}, false
	}
	return ix.get(pid)
}

// projectRootAbs is the project root, absolute ("" when unset).
func (s *Service) projectRootAbs() string {
	root := s.Config.ObjectsRoot()
	if root == "" {
		return ""
	}
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return root
}

// ObjectResolve is the "which file is this" tool (identity-and-index.md §1):
// a full name from a log is accepted only if its project id is THIS project's
// — a different project id is answered with that id, so the caller can ask the
// universe registry which mount has it (ktw-univers resolve-object) — a
// qualified `<key>:<module>.<local>` only if its key is this
// project's, and a bare `module.local` resolves in this project directly. One
// store resolves its own project; across projects the store set joins them
// (cwm/internal/storeset Set.Resolve).
func (s *Service) ObjectResolve(_ stdctx.Context, in api.ObjectResolveInput) (api.ObjectResolveOutput, error) {
	ref := strings.TrimSpace(in.Ref)
	if ref == "" {
		return api.ObjectResolveOutput{}, fmt.Errorf("%w: ref required", api.ErrInvalidInput)
	}
	proj, hasProject := s.projectObject()
	projectID := proj.ProjectUUID()
	id := ref
	if api.IsFullName(ref) {
		pid, bare, _ := api.ParseFullName(ref)
		if !hasProject || projectID == "" {
			return api.ObjectResolveOutput{}, fmt.Errorf("%w: this project has no id (no project.cwm.yaml with a uuid) — cannot match %s", api.ErrNotFound, ref)
		}
		if pid != projectID {
			return api.ObjectResolveOutput{}, fmt.Errorf("%w: %s belongs to project %s, not to this one (%s) — ask the universe registry: ktw-univers resolve-object %s",
				api.ErrNotFound, ref, pid, projectID, ref)
		}
		id = bare
	} else if strings.Contains(ref, ":") {
		// A qualified id: this project's own key resolves here; a
		// foreign key is another store's, joined by the store set.
		key, bare, err := api.ParseQualifiedID(ref)
		if err != nil {
			return api.ObjectResolveOutput{}, err
		}
		if !hasProject || key != proj.ID {
			return api.ObjectResolveOutput{}, fmt.Errorf("%w: %s belongs to project %q, not to this one (%q) - it resolves across the store set",
				api.ErrNotFound, ref, key, proj.ID)
		}
		id = bare
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectResolveOutput{}, err
	}
	obj, ok := ix.get(id)
	if !ok {
		return api.ObjectResolveOutput{}, fmt.Errorf("%w: no object %q in this project", api.ErrNotFound, id)
	}
	path, _ := ix.pathOf(id)
	out := api.ObjectResolveOutput{ID: id, ProjectID: projectID, Project: proj.ID, Path: path, Type: obj.Type,
		ProjectRoot: s.projectRootAbs()}
	if projectID != "" {
		out.FullName = api.FullName(projectID, id)
	}
	return out, nil
}

// EnsureProject gives the project its project object when it has none — the
// first time CWM opens a project attached to a universe (cwm/local, at boot):
// a new project shows in CWM as a project, and the `bootstrap` point, which
// fires on the project object, gives it its inbox and planification module. It
// answers whether it created one. An existing project object is never touched
// (its uuid is minted once).
func (s *Service) EnsureProject(ctx stdctx.Context) (bool, error) {
	root := s.Config.ObjectsRoot()
	if root == "" {
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(root, api.ProjectFileName)); err == nil {
		return false, nil
	}
	in, seed := s.projectSeed()
	if _, err := s.ProjectInit(ctx, in); err != nil {
		if errors.Is(err, api.ErrAlreadyExists) {
			return false, nil
		}
		if seed == "" {
			return false, err
		}
		// A seed CWM refuses (no project type, a group failing its schema) is
		// kept, its reason said, and the project opens as an unseeded one.
		slog.Warn("cwm: the project's seed is refused; it is kept and the project opens with the default object", "seed", seed, "error", err)
		if _, err := s.ProjectInit(ctx, api.ProjectInitInput{}); err != nil && !errors.Is(err, api.ErrAlreadyExists) {
			return false, err
		}
		return true, nil
	}
	if seed != "" {
		_ = os.Remove(seed)
	}
	return true, nil
}

// projectSeed is the project object's seed the hub left in the project's
// store, as ProjectInit's input, and its path; an empty input and
// "" when there is none or it cannot be read (the default object then).
func (s *Service) projectSeed() (api.ProjectInitInput, string) {
	if s.Config.WorkspaceRoot == "" {
		return api.ProjectInitInput{}, ""
	}
	path := filepath.Join(s.Config.WorkspaceRoot, api.ProjectSeedFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return api.ProjectInitInput{}, ""
	}
	var seed api.ProjectSeed
	if err := config.DecodeBytes(data, marshal.YAML, &seed); err != nil {
		slog.Warn("cwm: the project's seed cannot be read; the project opens with the default object", "seed", path, "error", err)
		return api.ProjectInitInput{}, ""
	}
	return api.ProjectInitInput{Name: seed.Name, Title: seed.Title, Type: seed.Type, Capabilities: seed.Capabilities}, path
}
