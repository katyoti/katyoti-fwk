// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ProjectsFolderSetting is the installation setting (katyoti.yml settings)
// naming the folder that holds the person's projects.
const ProjectsFolderSetting = "projects_folder"

// DefaultProjectsFolder is the projects folder proposed at first start, in the
// person's documents: <Documents>/Katyoti.
func DefaultProjectsFolder() string {
	return filepath.Join(UserDocumentsDir(), "Katyoti")
}

// ProjectsFolder is where the installation's projects live: the folder the
// person chose (chosen true), else the one proposed at first start. A program
// of no installation (a development build, a test) gets the proposed one.
func (i *Installation) ProjectsFolder() (dir string, chosen bool) {
	if i != nil {
		if s, ok := i.Settings[ProjectsFolderSetting].(string); ok && strings.TrimSpace(s) != "" {
			return filepath.Clean(s), true
		}
	}
	return DefaultProjectsFolder(), false
}

// CheckProjectsFolder says why dir cannot be the installation's projects
// folder: it is not an absolute path, or it lies in the installation's folder
// or one of its roots (config, data, cache) - an uninstall or a reinstall would
// take the projects with it. Nil: dir can be it.
func (i *Installation) CheckProjectsFolder(dir string) error {
	if strings.TrimSpace(dir) == "" || !filepath.IsAbs(dir) {
		return fmt.Errorf("the projects folder %q is not a full path", dir)
	}
	if what, root := i.inside(dir); what != "" {
		return fmt.Errorf("the projects folder %s is inside %s (%s): an uninstall or a reinstall would take the projects with it - choose a folder outside it", dir, what, root)
	}
	return nil
}

// ProjectDir is where a new project named name goes: <projects folder>/<name>.
func (i *Installation) ProjectDir(name string) string {
	dir, _ := i.ProjectsFolder()
	return filepath.Join(dir, name)
}

// CheckProjectDir says why dir cannot hold a project: it lies in the
// installation's folder or one of its roots, which an uninstall or a reinstall
// takes. A relative dir is judged where it resolves. Nil: dir can hold one (a
// program of no installation refuses none).
func (i *Installation) CheckProjectDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("the project folder %s: %w", dir, err)
	}
	if what, root := i.inside(abs); what != "" {
		projects, _ := i.ProjectsFolder()
		return fmt.Errorf("the project folder %s is inside %s (%s): an uninstall or a reinstall would take the project with it - put it in the projects folder (%s)", dir, what, root, projects)
	}
	return nil
}

// inside names the part of the installation dir lies in - its folder, or its
// config, data or cache root - and that part's path; empty when none.
func (i *Installation) inside(dir string) (what, root string) {
	if i == nil {
		return "", ""
	}
	for _, r := range []struct{ what, root string }{
		{"the installation's folder", i.Dir()},
		{"its config root", i.ConfigRoot()},
		{"its data root", i.DataRoot()},
		{"its cache root", i.CacheRoot()},
	} {
		if within(r.root, dir) {
			return r.what, r.root
		}
	}
	return "", ""
}

// SetProjectsFolder checks dir and records it as the installation's projects
// folder (katyoti.yml settings, written).
func (i *Installation) SetProjectsFolder(dir string) error {
	if i == nil {
		return fmt.Errorf("this program belongs to no installation (a development build): there is no projects folder to set")
	}
	if err := i.CheckProjectsFolder(dir); err != nil {
		return err
	}
	if i.Settings == nil {
		i.Settings = map[string]any{}
	}
	i.Settings[ProjectsFolderSetting] = filepath.Clean(dir)
	return SaveInstallation(i)
}
