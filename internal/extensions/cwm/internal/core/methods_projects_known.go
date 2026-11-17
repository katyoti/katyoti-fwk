// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"

	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ProjectsKnown lists the projects this universe knows: the
// one it serves, then every folder of the installation's projects folder
// holding a project object, by name. A capability keeps those whose type
// carries it - « Mes dossiers » asks `dossier`, never a type's name. Read
// only: opening another is the hub's switch (orchestrator switch-project).
// Each says its data version and whether its data waits for this binary's
// conversion or is newer than it knows.
//
// A refused project does not refuse this read: it reads only each
// project's header, never the refused data, and it is the way out - « Mes
// dossiers » and the GUI's Projects panel switch to another project through
// it. The served entry then carries the refusal (Refused), the others are
// listed as before. The capability filter reads the catalog's object types,
// declared whatever the universe's solution, so it filters a refused project
// the same; a project whose type no catalog declares is dropped by a filter,
// refused or not.
func (s *Service) ProjectsKnown(_ stdctx.Context, in api.ProjectsKnownInput) (api.ProjectsKnownOutput, error) {
	served := ""
	if r := s.Config.ObjectsRoot(); r != "" {
		served = filepath.Clean(r)
	}
	dirs := []string{}
	if served != "" {
		dirs = append(dirs, served)
	}
	if folder := s.projectsFolder(); folder != "" {
		entries, _ := os.ReadDir(folder)
		for _, e := range entries {
			dir := filepath.Join(folder, e.Name())
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || samePath(dir, served) {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, projectfile.FileName)); err != nil {
				continue
			}
			dirs = append(dirs, dir)
		}
	}
	out := api.ProjectsKnownOutput{Projects: []api.KnownProject{}}
	for _, dir := range dirs {
		f, err := projectfile.Read(dir)
		if err != nil {
			continue
		}
		if c := strings.TrimSpace(in.Capability); c != "" {
			t, declared := s.lookupObjectType(f.Type)
			if !declared || !slices.Contains(t.Capabilities, c) {
				continue
			}
		}
		known := api.KnownProject{Dir: dir, ID: f.ID, Name: f.Name, Type: f.Type, Served: samePath(dir, served), DataVersion: f.DataVersion}
		if known.Served && s.refused != nil {
			known.Refused = s.refused.Error()
		}
		switch binary := dataVersionOf(s.steps); {
		case f.DataVersion < binary:
			known.Conversion = api.ConversionWaiting
		case f.DataVersion > binary:
			known.Conversion = api.ConversionNewer
		}
		out.Projects = append(out.Projects, known)
	}
	slices.SortStableFunc(out.Projects, func(a, b api.KnownProject) int {
		switch {
		case a.Served != b.Served:
			if a.Served {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}

// projectsFolder is the folder the configuration names, else the
// installation's projects folder; a program of no installation (a test, a
// development build) has none - it knows the project it serves only.
func (s *Service) projectsFolder() string {
	if s.Config.ProjectsFolder != "" {
		return s.Config.ProjectsFolder
	}
	inst := cenv.CurrentInstallation()
	if inst == nil {
		return ""
	}
	dir, _ := inst.ProjectsFolder()
	return dir
}

// samePath says whether two folders are the same, case aside on Windows.
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
