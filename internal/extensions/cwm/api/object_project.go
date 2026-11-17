// SPDX-License-Identifier: Apache-2.0

package api

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
)

// The project object (identity-and-index.md §7).
//
// A project is an object like any other — `type: project` — but it is the ONE
// object filed outside the modules folder: it lives at the project root, beside
// `modules/`, because the scanner must read it BEFORE it knows where the modules
// are (`modules_dir` is one of its fields). Its id is the bare project name, the
// way a module object's id is the bare module name.

// ProjectFileName is the project object's file at the project root. Readers
// outside CWM read it through internal/core/projectfile.
const ProjectFileName = projectfile.FileName

// ProjectFilesFolder is where the project object's FILES live — its named files
// (comments, memory) and every ObjectFile* path: `<project>/.ktw/`, beside
// project.cwm.yaml. A module's files live in its module folder; the project
// belongs to no module, so its folder is the project root, and its own
// committed space there is the same `.ktw` a module's planning space uses. It
// is never scanned for objects: the scanner reads only project.cwm.yaml at the
// root, then the modules folder. (Not `<modules_dir>/.ktw/`: every folder of
// the modules folder is read as a module.)
const ProjectFilesFolder = PlanningFolder

// ObjectFileSuffix is the object file suffix for a data-only object.
const ObjectFileSuffix = ".cwm.yaml"

// ObjectFileSuffixMD is the suffix of an object with PROSE: YAML frontmatter
// carries the object, the markdown body carries the prose (`body`),
// nothing lives in both, and the body is never parsed for data (concepts.md
// §5 "The physical shape", amended 2026-09-24; phase 7c).
const ObjectFileSuffixMD = ".cwm.md"

// IsObjectFile reports whether a file name is an object file of either form.
func IsObjectFile(name string) bool {
	return strings.HasSuffix(name, ObjectFileSuffix) || strings.HasSuffix(name, ObjectFileSuffixMD)
}

// CapProject is the project capability id.
const CapProject = "project"

// DefaultModulesDir is the modules folder when the project object names none.
const DefaultModulesDir = projectfile.DefaultModulesDir

// ProjectInitInput mints a project id and writes the root project object.
type ProjectInitInput struct {
	Name        string `json:"name,omitempty"        jsonschema:"The project's name — its object id; defaults to the project folder's name"`
	Title       string `json:"title,omitempty"       jsonschema:"Human title; defaults to the name"`
	Description string `json:"description,omitempty" jsonschema:"One-line description"`
	ModulesDir  string `json:"modules_dir,omitempty" jsonschema:"Folder holding the modules; empty = modules"`
	// Type is the project's type - a type carrying the project level
	// capability; the default project type when empty.
	Type string `json:"type,omitempty" jsonschema:"Its project type; empty = software-project, the default"`
	// Capabilities are groups the new project object carries beside its
	// project capability (a dossier's identity) - checked against their
	// schemas like any write.
	Capabilities map[string]map[string]any `json:"capabilities,omitempty" jsonschema:"Groups the project object carries beside its project capability"`
}

// ProjectSeedFile is the seed of a project object, in the project's store
// (its workspace folder): what the hub that created the project wants its
// object to be. The hub never writes a project
// object; CWM mints it from the seed at the first opening (EnsureProject),
// at the code's data version, then removes the seed.
const ProjectSeedFile = "project-seed.yaml"

// ProjectSeed is a project object's seed: its name, title, type and groups.
type ProjectSeed struct {
	Name         string                    `json:"name,omitempty" yaml:"name,omitempty"`
	Title        string                    `json:"title,omitempty" yaml:"title,omitempty"`
	Type         string                    `json:"type,omitempty" yaml:"type,omitempty"`
	Capabilities map[string]map[string]any `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}

// ProjectKeyOf is the key the project at dir is addressed by - the id of its
// project object - read without opening it: its project object's id; else,
// before its first opening, the name of the seed the hub left in its store
// (ProjectSeedFile), which CWM mints it from; else its folder's name, which
// ProjectInit names a project with no seed after. A store set joins a
// member's store under that key; the hub reads a member's key
// the same way before it attaches one beside others. A seed CWM refuses at the
// opening is the one case it misses: the default object, named after the
// folder, is minted instead.
func ProjectKeyOf(dir string) string {
	if dir == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, ProjectFileName)); err == nil {
		if f, err := projectfile.Read(dir); err == nil && f.ID != "" {
			return f.ID
		}
	}
	if data, err := os.ReadFile(filepath.Join(application.EnvOf(dir).HomeDir(), ProjectSeedFile)); err == nil {
		var seed ProjectSeed
		if config.DecodeBytes(data, marshal.YAML, &seed) == nil {
			if name := strings.TrimSpace(seed.Name); name != "" {
				return name
			}
		}
	}
	return filepath.Base(filepath.Clean(dir))
}

// DefaultProjectType is the type a project is created with when none is
// named.
const DefaultProjectType = "software-project"

// ProjectInitOutput returns the written project object and its path.
type ProjectInitOutput struct {
	Object Object `json:"object"`
	Path   string `json:"path" jsonschema:"Path of the project object, relative to the project root"`
}

// ModulesDir returns the modules folder a project object declares, or the default.
func (o Object) ModulesDir() string {
	if g, ok := o.Capability(CapProject); ok {
		if d, _ := g["modules_dir"].(string); d != "" {
			return d
		}
	}
	return DefaultModulesDir
}

// ProjectUUID returns the project id a project object carries, or "".
func (o Object) ProjectUUID() string {
	if g, ok := o.Capability(CapProject); ok {
		u, _ := g["uuid"].(string)
		return u
	}
	return ""
}

// ObjectResolveInput answers "which file is this" for a reference — a full
// name from a log, or a bare `module.local` inside the current project.
type ObjectResolveInput struct {
	Ref string `json:"ref" cli:"arg" jsonschema:"A full name (<project-uuid>.<module>.<local>) or a qualified id (<key>:<module>.<local>) or a module-qualified id; required"`
}

// ObjectResolveOutput is the resolution: the object's ids and its file.
type ObjectResolveOutput struct {
	ID        string `json:"id"         jsonschema:"The authored, module-qualified id"`
	FullName  string `json:"full_name"  jsonschema:"<project-uuid>.<id> — the name that is unique everywhere"`
	ProjectID string `json:"project_id" jsonschema:"The project's uuid"`
	Project   string `json:"project"    jsonschema:"The project object's id (its name)"`
	Path      string `json:"path"       jsonschema:"The object's file, absolute"`
	Type      string `json:"type"`
	// ProjectRoot is the folder holding project.cwm.yaml and the modules
	// folder — every project-relative path is relative to it.
	ProjectRoot string `json:"project_root" jsonschema:"The project root (the folder holding project.cwm.yaml and the modules folder), absolute"`
}

// ProjectsKnownInput asks the projects a universe knows, optionally only
// those whose type carries a capability.
type ProjectsKnownInput struct {
	Capability string `json:"capability,omitempty" jsonschema:"Keep only the projects whose type carries this capability (dossier for the dossiers d'architecture); empty keeps every one"`
}

// KnownProject is a project a universe knows: its folder, its id, name and
// type, whether the universe serves it now, and why it refuses it if it does.
type KnownProject struct {
	Dir    string `json:"dir"    jsonschema:"The project's folder"`
	ID     string `json:"id"     jsonschema:"The project object's id - its name"`
	Name   string `json:"name"   jsonschema:"The project's display name"`
	Type   string `json:"type"   jsonschema:"The project's type"`
	Served bool   `json:"served" jsonschema:"The universe serves it now"`
	// DataVersion is the version of the shape of the project's data (1 when
	// its project object names none).
	DataVersion int `json:"data_version" jsonschema:"The version of the shape of the project's data - 1 when its project object names none"`
	// Conversion says whether the project's data waits for this binary's
	// conversion (ConversionWaiting), is newer than it knows
	// (ConversionNewer), or neither ("").
	Conversion string `json:"conversion,omitempty" jsonschema:"waiting: its data is older than this binary's and waits for its conversion (cwm migrate); newer: its data is newer than this binary knows; empty: at this binary's version"`
	// Refused is why this universe refuses to open the project
	// set on the served entry only: the error's text with its
	// `project refused: ` lead. Empty when it opens.
	Refused string `json:"refused,omitempty" jsonschema:"Why this universe refuses to open the served project - its 'project refused: ' reason; empty when it opens"`
}

// The conversion a known project's data waits for, against this binary's
// data version.
const (
	ConversionWaiting = "waiting"
	ConversionNewer   = "newer"
)

// ProjectsKnownOutput is the projects a universe knows, the one it serves
// first, then by name.
type ProjectsKnownOutput struct {
	Projects []KnownProject `json:"projects" jsonschema:"The projects - the one served first, then by name"`
}
