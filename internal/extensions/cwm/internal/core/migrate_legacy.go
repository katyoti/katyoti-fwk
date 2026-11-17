// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A REPOSITORY NOT YET CONVERTED KEEPS WORKING: IN-036 typed every
// module (data version 17, and 18 for a module carrying an aspect) and every
// bare project (19), and retired the module and project types (
// ) - no pack ships them any more. A repository whose data is older
// still holds objects typed module, code-module or project: until
// `cwm migrate` converts it, this binary reads them in their legacy shape - a
// module, at the module level, with its lifecycle - so they are read, planned
// and shown as before; never offered by « + New » nor created anew (the type
// stays retired); and DeclarationFindings says the conversion is waiting. A
// repository at or past the conversion reads no legacy type: a module typed so
// there is an undeclared type, as any other.

// legacyType is a retired type's shape when the repository was written, the
// first data version where no object is of it any more, and what replaces it.
type legacyType struct {
	shape    api.ObjectType
	until    int
	replaced string
}

// legacyObjectTypes are the retired types an unconverted repository may hold.
var legacyObjectTypes = map[string]legacyType{
	"module":      {api.ObjectType{Name: "module", Label: "Module", Icon: "package", Tree: "root", Capabilities: []string{api.CapModule, "lifecycle"}}, 18, "documentation or software-module"},
	"code-module": {api.ObjectType{Name: "code-module", Label: "Code module", Icon: "code", Tree: "root", Capabilities: []string{api.CapModule, "lifecycle"}}, 18, "software-module"},
	"project":     {api.ObjectType{Name: "project", Label: "Project", Icon: "git-branch", Tree: "root", Capabilities: []string{api.CapProject, "lifecycle"}}, 19, "software-project or another project type a capability declares"},
	// A project group (retired at 41): read as a plain project
	// until cwm migrate joins it into one (project_group_merge.go).
	"project-group": {api.ObjectType{Name: "project-group", Label: "Project group", Icon: "folders", Tree: "root", Capabilities: []string{api.CapProject, "lifecycle"},
		AllowedChildren: []string{"software-module", "documentation", "system-module"}}, 41, "one project: cwm migrate joins the group (--group-renames for a module name two of its projects hold)"},
}

// projectDataVersion is the served project's data version, read once and
// cached until a migration; this binary's version when the project names none
// that can be read.
func (s *Service) projectDataVersion() int {
	if v := s.dataVersion.Load(); v > 0 {
		return int(v)
	}
	// No project object yet (a fresh folder): nothing old to read.
	v := dataVersionOf(s.steps)
	if s.Config != nil {
		if root := s.Config.ObjectsRoot(); root != "" {
			if _, err := os.Stat(filepath.Join(root, api.ProjectFileName)); err == nil {
				if f, err := projectfile.Read(root); err == nil && f.DataVersion > 0 {
					v = f.DataVersion
				}
			}
		}
	}
	s.dataVersion.Store(int64(v))
	return v
}

// legacyObjectType answers a retired type's legacy shape while the served
// project's data is older than the conversion.
func (s *Service) legacyObjectType(name string) (api.ObjectType, bool) {
	t, ok := legacyObjectTypes[name]
	if !ok || s.projectDataVersion() >= t.until {
		return api.ObjectType{}, false
	}
	return t.shape, true
}

// legacyCreate refuses a new object of a retired type: an unconverted
// repository reads its old objects, nothing new is made of that type.
func legacyCreate(obj api.Object, verb string) error {
	return api.Invalid(fmt.Errorf("%w: %s: object %q has the retired type %q (undeclared type since the conversion): its old objects are read until cwm migrate - make it a %s",
		api.ErrInvalidInput, verb, obj.ID, obj.Type, legacyObjectTypes[obj.Type].replaced), "cwm.retiredType", map[string]any{"id": obj.ID, "type": obj.Type, "replaced": legacyObjectTypes[obj.Type].replaced})
}

// conversionFindings says the served project's conversion is waiting: its
// data is older than this binary's.
func (s *Service) conversionFindings() []api.DeclarationFinding {
	have, want := s.projectDataVersion(), dataVersionOf(s.steps)
	if have >= want {
		return nil
	}
	return []api.DeclarationFinding{{
		Rule: "conversion-waiting", Severity: "warning", Kind: "project",
		Message: fmt.Sprintf("the project's data is at version %d and this binary's at %d: its conversion waits - run cwm migrate (until then its objects of a retired type are read as before)", have, want),
	}}
}

// legacyCapabilities are the retired capabilities an unconverted repository's
// objects may still carry, each with the first data version where none does:
// the project-group capability of an old group's root, read only by the join
// (project_group_legacy.go).
var legacyCapabilities = map[string]int{legacyCapProjectGroup: 41}

// legacyCapability says a group under a retired capability's name is read
// as written, not refused, while the served project's data is older than its
// retirement.
func (s *Service) legacyCapability(name string) bool {
	until, ok := legacyCapabilities[name]
	return ok && s.projectDataVersion() < until
}

// isLegacyType says a type resolves only to its legacy shape: retired, not
// declared by any pack.
func (s *Service) isLegacyType(name string) bool {
	if _, ok := legacyObjectTypes[name]; !ok {
		return false
	}
	s.objTypesMu.RLock()
	_, declared := s.objTypes[name]
	s.objTypesMu.RUnlock()
	return !declared
}
