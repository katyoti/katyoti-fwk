// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// JOINING A PROJECT GROUP (:
// a project never holds projects): a repository whose root is a project group
// becomes ONE project. Every module of each embedded child moves as it is
// into the root's modules folder; a system module the root holds too (sys-*)
// is joined file by file. The child project objects go - what they carried
// besides their project group (a dossier's identity) is reported for the
// person to keep where they choose - and a parent or an edge naming a child
// project names the joined project. Ids, files and every other edge stay as
// they were: an object id moves with its module. A module name two projects
// of the group hold is a collision, never overwritten: the join is refused,
// writing nothing, until the person renames one - before the join, or as it
// is made (GroupRenames: a child's module by its folder to its new name; the
// folder, its objects' ids and every parent and edge its own project named
// them by follow; text inside documents is left as written). It runs at the start of
// `cwm migrate`, the steps then running from the lowest data version of the
// root and its children.

// planGroupMerge reads the group whose root is root and answers what joining
// it does; nil when the root is no project group. Nothing is written.
func planGroupMerge(root string, in api.MigrateInput) (*groupMergePlan, error) {
	project, err := ReadObjectFile(filepath.Join(root, api.ProjectFileName))
	if err != nil {
		return nil, nil // the version check reports an unreadable project object
	}
	g, ok, err := legacyGroupOf(project)
	if err != nil || !ok {
		return nil, err
	}
	children, err := legacyEmbeddedChildren(root, g)
	if err != nil {
		return nil, err
	}
	modulesDir := project.ModulesDir()
	f, err := projectfile.Read(root)
	if err != nil {
		return nil, err
	}
	p := &groupMergePlan{root: root, modulesDir: modulesDir, project: project, childIDs: map[string]bool{},
		renames: map[string]childRenames{}, childOfID: map[string]string{}}
	pendingRenames := map[string]bool{}
	for from, to := range in.GroupRenames {
		if !moduleNamePattern.MatchString(to) {
			return nil, fmt.Errorf("%w: group rename %s: %q is no module name (kebab-case)", api.ErrInvalidInput, from, to)
		}
		pendingRenames[from] = true
	}
	rep := &p.report
	rep.Group, rep.DataVersion, rep.Links = project.ID, f.DataVersion, g.Links

	rootModules, err := moduleNames(filepath.Join(root, filepath.FromSlash(modulesDir)))
	if err != nil {
		return nil, err
	}
	rep.Before = len(rootModules)
	owner := map[string]string{} // module -> its folder, project-relative
	for _, m := range rootModules {
		owner[m] = path.Join(modulesDir, m)
	}
	types := map[string]bool{}
	for _, c := range children {
		childRoot := filepath.Join(root, filepath.FromSlash(c.Dir))
		child, err := ReadObjectFile(filepath.Join(childRoot, api.ProjectFileName))
		if err != nil {
			return nil, fmt.Errorf("project group: child %s: %w", c.Dir, err)
		}
		rep.Children = append(rep.Children, c.Dir)
		p.childIDs[child.ID] = true
		p.childOfID[child.ID] = c.Dir
		p.childDirs = append(p.childDirs, c.Dir)
		types[child.Type] = true
		if cf, err := projectfile.Read(childRoot); err == nil && cf.DataVersion < rep.DataVersion {
			rep.DataVersion = cf.DataVersion
		}
		drop := api.DroppedProject{ID: child.ID, Type: child.Type, Dir: c.Dir}
		for id, fields := range child.Capabilities {
			if id == api.CapProject {
				continue
			}
			if drop.Capabilities == nil {
				drop.Capabilities = map[string]map[string]any{}
			}
			drop.Capabilities[id] = fields
		}
		rep.Dropped = append(rep.Dropped, drop)

		childModulesDir := child.ModulesDir()
		names, err := moduleNames(filepath.Join(childRoot, filepath.FromSlash(childModulesDir)))
		if err != nil {
			return nil, err
		}
		rep.Before += len(names)
		for _, m := range names {
			from := path.Join(c.Dir, childModulesDir, m)
			mv := api.ModuleMove{Module: m}
			if n, ok := in.GroupRenames[from]; ok {
				delete(pendingRenames, from)
				if isSystemModule(m) || isSystemModule(n) {
					return nil, fmt.Errorf("%w: group rename %s: a system module is joined, never renamed", api.ErrInvalidInput, from)
				}
				if n != m {
					cr, seen := p.renames[c.Dir]
					if !seen {
						cr = childRenames{modulesDir: childModulesDir, names: map[string]string{}}
					}
					cr.names[m] = n
					p.renames[c.Dir] = cr
					mv.Module, mv.RenamedFrom = n, m
				}
			}
			m := mv.Module
			to := path.Join(modulesDir, m)
			files, err := moduleFiles(filepath.Join(root, filepath.FromSlash(from)))
			if err != nil {
				return nil, err
			}
			was, taken := owner[m]
			switch {
			case !taken:
				owner[m] = from
				mv.From, mv.To, mv.Files = from, to, len(files)
				rep.Moved = append(rep.Moved, mv)
			case isSystemModule(m) && was == to:
				join, clash := p.planJoin(m, from, to, files)
				if len(clash) > 0 {
					rep.Collisions = append(rep.Collisions, api.ModuleCollision{Module: m, Paths: clash,
						Why: "a file of the system module is held by the root and by " + c.Dir + " with different contents"})
					continue
				}
				rep.Joined = append(rep.Joined, join)
			default:
				rep.Collisions = append(rep.Collisions, api.ModuleCollision{Module: m, Paths: []string{was, from},
					Why: "two projects of the group hold a module of this name - rename one before the join"})
			}
		}
		// Anything a child holds besides its project object and its modules
		// would be lost with its folder: refused.
		entries, err := os.ReadDir(childRoot)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.Name() == api.ProjectFileName || e.Name() == path.Base(childModulesDir) {
				continue
			}
			if local := localStateDir(childRoot); e.Name() == local && e.IsDir() {
				if files, err := moduleFiles(filepath.Join(childRoot, local)); err == nil && len(files) > 0 {
					rep.KeptAside = append(rep.KeptAside, api.ModuleMove{From: path.Join(c.Dir, local),
						To: path.Join(localStateDir(root), "joined-projects", path.Base(c.Dir)), Files: len(files)})
				}
				continue
			}
			rep.Collisions = append(rep.Collisions, api.ModuleCollision{Paths: []string{path.Join(c.Dir, e.Name())},
				Why: "the child holds this beside its project object and its modules - move it into a module before the join"})
		}
	}
	if len(pendingRenames) > 0 {
		var left []string
		for from := range pendingRenames {
			left = append(left, from)
		}
		sort.Strings(left)
		return nil, fmt.Errorf("%w: group renames name no child module: %s", api.ErrInvalidInput, strings.Join(left, ", "))
	}
	rep.After = len(owner)
	switch {
	case in.GroupType != "":
		rep.Type = in.GroupType
	case len(types) == 1:
		for t := range types {
			rep.Type = t
		}
	default:
		rep.Type = api.DefaultProjectType
	}
	if err := p.planRetargets(); err != nil {
		return nil, err
	}
	sortMerge(rep)
	p.groupCaps = in.GroupCapabilities
	return p, nil
}

// groupMergePlan is a planned join and what applying it needs.
type groupMergePlan struct {
	report     api.GroupMerge
	root       string
	modulesDir string
	project    api.Object
	childIDs   map[string]bool
	childDirs  []string
	groupCaps  map[string]map[string]any
	// retargets are the object files, by path where they are read today,
	// whose parent or edge names a child project, or whose id or reference a
	// rename changes.
	retargets []string
	// renames are the modules renamed as they join, by child folder.
	renames map[string]childRenames
	// childOfID is each child project's folder by its id.
	childOfID map[string]string
}

// childRenames are a child's modules renamed as it joins: old name to new.
type childRenames struct {
	modulesDir string
	names      map[string]string
}

// ref renames an id - or a path into the module's folder (an attached
// document) - of a module the child renames.
func (r childRenames) ref(ref string) string {
	mod, local, dotted := strings.Cut(ref, ".")
	if n, ok := r.names[mod]; ok {
		if !dotted {
			return n
		}
		return n + "." + local
	}
	for old, n := range r.names {
		if prefix := path.Join(r.modulesDir, old) + "/"; strings.HasPrefix(ref, prefix) {
			return path.Join(r.modulesDir, n) + "/" + strings.TrimPrefix(ref, prefix)
		}
	}
	return ref
}

// moduleNamePattern is a module name: kebab-case.
var moduleNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// planJoin compares a system module a child holds with the root's: a file
// the root lacks moves, one it holds with the same bytes - or the module's
// own object, the root's kept - is dropped, one it holds with other bytes
// clashes.
func (p *groupMergePlan) planJoin(m, from, to string, files []string) (api.ModuleMove, []string) {
	join := api.ModuleMove{Module: m, From: from, To: to}
	var clash []string
	own := map[string]bool{"objects/" + m + api.ObjectFileSuffix: true, "objects/" + m + api.ObjectFileSuffixMD: true}
	for _, rel := range files {
		dst := filepath.Join(p.root, filepath.FromSlash(to), filepath.FromSlash(rel))
		theirs, err := os.ReadFile(dst)
		if err != nil {
			join.Files++
			continue
		}
		mine, _ := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(from), filepath.FromSlash(rel)))
		if bytes.Equal(mine, theirs) || own[rel] {
			join.Dropped = append(join.Dropped, rel)
			continue
		}
		clash = append(clash, path.Join(to, rel), path.Join(from, rel))
	}
	return join, clash
}

// planRetargets finds the object files - in the root's modules and every
// child's - whose parent or an edge names a child project.
func (p *groupMergePlan) planRetargets() error {
	dirs := []string{p.modulesDir}
	for _, mv := range append(append([]api.ModuleMove{}, p.report.Moved...), p.report.Joined...) {
		dirs = append(dirs, mv.From)
	}
	for _, d := range dirs {
		err := filepath.WalkDir(filepath.Join(p.root, filepath.FromSlash(d)), func(fp string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || !api.IsObjectFile(e.Name()) {
				return err
			}
			o, err := ReadObjectFile(fp)
			if err != nil {
				return nil // the index names an unreadable object
			}
			if p.retarget(&o, fp) {
				p.retargets = append(p.retargets, fp)
				if rel, err := filepath.Rel(p.root, fp); err == nil {
					p.report.Retargeted = append(p.report.Retargeted, filepath.ToSlash(rel))
				}
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// retarget points a parent or an edge naming a child project - or a
// cross-project reference into one - at the joined project, and renames the
// ids and references a renamed module of the object's own child changes; fp
// is where the object is read today. True when it changed o.
func (p *groupMergePlan) retarget(o *api.Object, fp string) bool {
	changed := false
	renamed := func(r childRenames, ref string) string {
		if r.names == nil {
			return ref
		}
		if n := r.ref(ref); n != ref {
			changed = true
			return n
		}
		return ref
	}
	own := p.renames[p.childOf(fp)]
	to := func(ref string) string {
		if p.childIDs[ref] {
			changed = true
			return p.report.Group
		}
		if key, local, ok := strings.Cut(ref, ":"); ok && p.childIDs[key] && !strings.HasPrefix(local, "/") {
			changed = true
			return renamed(p.renames[p.childOfID[key]], local)
		}
		return renamed(own, ref)
	}
	o.ID = renamed(own, o.ID)
	o.Parent = to(o.Parent)
	for i := range o.Edges {
		o.Edges[i].To = to(o.Edges[i].To)
	}
	return changed
}

// childOf is the child folder (project-relative) an object file read today
// sits in; "" for the root's own.
func (p *groupMergePlan) childOf(fp string) string {
	rel, err := filepath.Rel(p.root, fp)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	for _, d := range p.childDirs {
		if strings.HasPrefix(rel, d+"/") {
			return d
		}
	}
	return ""
}

// apply makes the join: nothing is written while a collision stands.
func (p *groupMergePlan) apply() error {
	rep := &p.report
	if len(rep.Collisions) > 0 {
		var lines []string
		for _, c := range rep.Collisions {
			lines = append(lines, fmt.Sprintf("%s (%s): %s", c.Module, strings.Join(c.Paths, ", "), c.Why))
		}
		return fmt.Errorf("%w: the project group %s cannot be joined into one project, nothing was written: %s",
			api.ErrConflict, rep.Group, strings.Join(lines, "; "))
	}
	// Retargets first, where the files are today: a later failure leaves a
	// group whose references already name the root - still readable.
	for _, fp := range p.retargets {
		o, err := ReadObjectFile(fp)
		if err != nil {
			return err
		}
		p.retarget(&o, fp)
		if _, err := writeObjectFileRev(fp, o); err != nil {
			return err
		}
	}
	abs := func(rel string) string { return filepath.Join(p.root, filepath.FromSlash(rel)) }
	for _, mv := range rep.Moved {
		if err := os.MkdirAll(filepath.Dir(abs(mv.To)), 0o755); err != nil { // a root holding no module yet
			return err
		}
		if err := os.Rename(abs(mv.From), abs(mv.To)); err != nil {
			return fmt.Errorf("project group join: move %s: %w", mv.From, err)
		}
		if mv.RenamedFrom == "" {
			continue
		}
		// The module's own object file is named after the module.
		for _, suffix := range []string{api.ObjectFileSuffix, api.ObjectFileSuffixMD} {
			was := filepath.Join(abs(mv.To), "objects", mv.RenamedFrom+suffix)
			if _, err := os.Stat(was); err == nil {
				if err := os.Rename(was, filepath.Join(abs(mv.To), "objects", mv.Module+suffix)); err != nil {
					return fmt.Errorf("project group join: rename %s: %w", was, err)
				}
			}
		}
	}
	for _, mv := range rep.Joined {
		dropped := map[string]bool{}
		for _, d := range mv.Dropped {
			dropped[d] = true
		}
		files, err := moduleFiles(abs(mv.From))
		if err != nil {
			return err
		}
		for _, rel := range files {
			if dropped[rel] {
				continue
			}
			dst := filepath.Join(abs(mv.To), filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.Rename(filepath.Join(abs(mv.From), filepath.FromSlash(rel)), dst); err != nil {
				return fmt.Errorf("project group join: move %s/%s: %w", mv.From, rel, err)
			}
		}
		if err := os.RemoveAll(abs(mv.From)); err != nil {
			return err
		}
	}
	for _, mv := range rep.KeptAside {
		if err := os.MkdirAll(filepath.Dir(abs(mv.To)), 0o755); err != nil {
			return err
		}
		if err := os.Rename(abs(mv.From), abs(mv.To)); err != nil {
			return fmt.Errorf("project group join: keep %s aside: %w", mv.From, err)
		}
	}
	for _, dir := range p.childDirs {
		if err := os.RemoveAll(abs(dir)); err != nil { // its project object and its emptied modules folder
			return err
		}
	}
	if g, ok, _ := legacyGroupOf(p.project); ok {
		if projects, err := legacyProjectsDir(g); err == nil {
			_ = os.Remove(abs(projects)) // only when empty
		}
	}
	// The root: its type, its group dropped, the lowest data version so every
	// step runs over what the children bring.
	o := p.project
	o.Type = rep.Type
	delete(o.Capabilities, legacyCapProjectGroup)
	for id, fields := range p.groupCaps {
		if err := o.SetCapability(id, fields); err != nil {
			return err
		}
	}
	group, _ := o.Capability(api.CapProject)
	next := map[string]any{}
	for k, v := range group {
		next[k] = v
	}
	next["data_version"] = rep.DataVersion
	if err := o.SetCapability(api.CapProject, next); err != nil {
		return err
	}
	_, err := writeObjectFileRev(abs(api.ProjectFileName), o)
	return err
}

// groupMergeTouched are the project-relative paths a join writes: each
// moved module's folders, each child's folder, the retargeted objects.
func groupMergeTouched(rep *api.GroupMerge) []string {
	if rep == nil {
		return nil
	}
	var out []string
	for _, mv := range append(append([]api.ModuleMove{}, rep.Moved...), rep.Joined...) {
		out = append(out, mv.From, mv.To)
	}
	out = append(out, rep.Children...)
	return append(out, rep.Retargeted...)
}

// localStateDir is a project's machine-local folder (gitignored): its index,
// its local queues - its home folder, as the environment names it. A join
// only carries a child's aside, unread.
func localStateDir(root string) string {
	return filepath.Base(application.EnvOf(root).HomeDir())
}

// isSystemModule says whether a module is one every project holds - its
// inbox, journal, planification space.
func isSystemModule(name string) bool { return strings.HasPrefix(name, "sys-") }

// moduleNames lists the module folders of a modules folder.
func moduleNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// moduleFiles lists every file under dir, slash-separated and relative to it.
func moduleFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(fp string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, fp)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out, err
}

func sortMerge(rep *api.GroupMerge) {
	sort.Slice(rep.Moved, func(i, j int) bool { return rep.Moved[i].From < rep.Moved[j].From })
	sort.Slice(rep.Joined, func(i, j int) bool { return rep.Joined[i].From < rep.Joined[j].From })
	sort.Strings(rep.Children)
	sort.Strings(rep.Retargeted)
}
