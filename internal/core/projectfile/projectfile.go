// SPDX-License-Identifier: Apache-2.0

// Package projectfile reads the project object, `<project>/project.cwm.yaml`,
// for what the processes that run no CWM must know of it: where the modules
// are and where the project's own contributions live. CWM writes the file;
// this package only reads it, so the hub, every sidecar and CWM read the same
// facts the same way (decision).
package projectfile

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// FileName is the project object's file at the project root.
const FileName = "project.cwm.yaml"

// DefaultModulesDir is the modules folder when the project object names none.
const DefaultModulesDir = "modules"

// BaselineDataVersion is the data version of a project whose object names
// none: the shape its data had when data versions began.
const BaselineDataVersion = 1

// ContributionsFolder is the module holding the project's own contributions,
// under the modules folder, when the project object names no root.
const ContributionsFolder = "sys-contributions"

// File is what a reader outside CWM knows of the project object.
type File struct {
	// ID is the project object's id: the project's name.
	ID string
	// Type is the project object's type (software-project, a dossier
	// d'architecture...); "" when the file names none.
	Type string
	// Name is the project object's display name; its id when it names none.
	Name string
	// UUID is the project's one computer id; "" before `cwm project-init`.
	UUID string
	// ModulesDir is the folder under the project root holding the modules.
	ModulesDir string
	// Contributions is where the project's own contributions live.
	Contributions Contributions
	// ScanExclude is the project's own exclusions from the object scan:
	// doublestar globs, project-relative and slash-separated, each valid.
	ScanExclude []string
	// DataVersion is the version of the shape of the project's data;
	// BaselineDataVersion when the project object names none.
	DataVersion int
}

// Contributions is the project object's `contributions` capability: the one
// source of its field-group's data-type, `cap-contributions`, generated from
// it (projectcontributions/contrib).
type Contributions struct {
	// Provider is the token the project's contributions are published under.
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty" jsonschema:"title=Provider token,description=The token the project's contributions are published under (provider:/kind/name): a kebab token no member of the universe is named; empty means the project id made kebab"`
	// Root is their folder, relative to the project root, slash-separated.
	Root string `json:"root,omitempty" yaml:"root,omitempty" jsonschema:"title=Contributions folder,description=The folder holding the project's skills; tasks; kinds and packs - relative to the project root and slash-separated; empty means the sys-contributions module of the modules folder"`
}

// Project is the `project` capability's field-group: the one source of its
// data-type, `cap-project`, generated from it (cwm/contrib). No path: a path
// is machine-local, true for one clone and wrong for every other.
type Project struct {
	UUID        string `json:"uuid,omitempty" yaml:"uuid,omitempty" jsonschema:"title=Project id,description=The one computer id: a UUIDv7 minted once by cwm project-init and committed with the repository; a clone carries it and a fork mints a new one. The first segment of every object's full name"`
	ModulesDir  string `json:"modules_dir,omitempty" yaml:"modules_dir,omitempty" jsonschema:"title=Modules folder,description=The folder under the project root that holds the modules; empty means modules"`
	Scan        Scan   `json:"scan,omitempty" yaml:"scan,omitempty" jsonschema:"title=Object scan,description=What the object scan leaves out on top of its built-in exclusions"`
	DataVersion int    `json:"data_version,omitempty" yaml:"data_version,omitempty" jsonschema:"title=Data version,description=The version of the shape of the project's data: a binary refuses data newer than it knows and cwm migrate brings older data to its own. Written by CWM; empty means 1"`
}

// Scan is the project's settings for the object scan.
type Scan struct {
	Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty" jsonschema:"title=Excluded paths,description=Globs relative to the project root and slash-separated in doublestar syntax where ** crosses folders: a matching folder is not walked and a matching file is not read. The names starting with _ or . stay excluded"`
}

// raw is the file's shape, as CWM writes it.
type raw struct {
	ID           string `yaml:"id"`
	Type         string `yaml:"type"`
	Name         string `yaml:"name"`
	Capabilities struct {
		Project       Project       `yaml:"project"`
		Contributions Contributions `yaml:"contributions"`
	} `yaml:"capabilities"`
}

var kebab = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Read reads the project object at projectRoot. A project without the file
// answers the defaults and no error: a project that has not run
// `project-init` is not a fault. The file's folder name stands for the id
// then. A provider that is not a kebab token is refused, and so is a scan
// exclusion that is not a valid glob.
func Read(projectRoot string) (File, error) {
	var r raw
	data, err := os.ReadFile(filepath.Join(projectRoot, FileName))
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return File{}, fmt.Errorf("projectfile: %w", err)
	default:
		if err := config.DecodeBytes(data, marshal.YAML, &r); err != nil {
			return File{}, fmt.Errorf("projectfile: %s: %w", FileName, err)
		}
	}
	f := File{
		ID:          r.ID,
		Type:        r.Type,
		Name:        r.Name,
		UUID:        r.Capabilities.Project.UUID,
		ModulesDir:  r.Capabilities.Project.ModulesDir,
		ScanExclude: r.Capabilities.Project.Scan.Exclude,
		DataVersion: r.Capabilities.Project.DataVersion,
		Contributions: Contributions{
			Provider: r.Capabilities.Contributions.Provider,
			Root:     r.Capabilities.Contributions.Root,
		},
	}
	if f.ID == "" && projectRoot != "" {
		f.ID = filepath.Base(filepath.Clean(projectRoot))
	}
	if f.Name == "" {
		f.Name = f.ID
	}
	if f.ModulesDir == "" {
		f.ModulesDir = DefaultModulesDir
	}
	if f.DataVersion == 0 {
		f.DataVersion = BaselineDataVersion
	} else if f.DataVersion < BaselineDataVersion {
		return File{}, fmt.Errorf("projectfile: %s: data version %d is below %d, the first one", FileName, f.DataVersion, BaselineDataVersion)
	}
	for _, g := range f.ScanExclude {
		if g == "" || !doublestar.ValidatePattern(g) {
			return File{}, fmt.Errorf("projectfile: %s: scan exclude %q is not a valid glob (doublestar syntax, project-relative, slash-separated)", FileName, g)
		}
	}
	if f.Contributions.Provider == "" {
		f.Contributions.Provider = toKebab(f.ID)
	} else if !kebab.MatchString(f.Contributions.Provider) {
		return File{}, fmt.Errorf("projectfile: %s: contributions provider %q is not a kebab token ([a-z0-9] words joined by -)", FileName, f.Contributions.Provider)
	}
	if f.Contributions.Root == "" {
		f.Contributions.Root = path.Join(filepath.ToSlash(f.ModulesDir), ContributionsFolder)
	}
	return f, nil
}

// toKebab turns a project name into a provider token: lower case, every run
// of other characters one hyphen, none at either end.
func toKebab(s string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			hyphen = false
			continue
		}
		hyphen = true
	}
	return b.String()
}
