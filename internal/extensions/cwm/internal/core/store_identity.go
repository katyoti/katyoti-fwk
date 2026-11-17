// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ONE STORE, ONE PROJECT. A Service serves one project folder: one
// corpus root (the folder holding project.cwm.yaml and its modules) and one
// store root (its runtime objects, under its workspace's .katyoti). Several
// projects - two folders of one repository, or two repositories - are several
// Services, joined above them by the store set (cwm/internal/storeset), which
// routes each read and write to the store holding the object. No second root
// is ever passed into the core (internal/test/arch: TestCWMCoreKeepsOneRootPerStore).
//
// These accessors are what the store set routes by; they read, never change,
// the store.

// ProjectKey is the key the project this store serves is addressed by: its
// project object's id, "" when the folder holds no
// project object or it does not read - never the folder's name, which is
// machine-local where a key is committed.
func (s *Service) ProjectKey() string {
	if _, err := os.Stat(filepath.Join(s.CorpusRoot(), api.ProjectFileName)); err != nil {
		return ""
	}
	f, err := projectfile.Read(s.CorpusRoot())
	if err != nil {
		return ""
	}
	return f.ID
}

// ProjectUUID is the uuid of the project this store serves (the first part of
// a full name), "" when it has no project object or no uuid.
func (s *Service) ProjectUUID() string {
	proj, ok := s.projectObject()
	if !ok {
		return ""
	}
	return proj.ProjectUUID()
}

// CorpusRoot is the one folder whose project object and modules this store
// serves (CWMServiceConfig.ObjectsRoot).
func (s *Service) CorpusRoot() string { return s.Config.ObjectsRoot() }

// StoreRoot is the one folder of this store's runtime objects; "" for a store
// without one.
func (s *Service) StoreRoot() string { return s.storeObjectsRoot() }

// Holds says whether this store's index holds the object id. It loads the
// index on first use; a refused project answers its refusal.
func (s *Service) Holds(id string) (bool, error) {
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return false, err
	}
	_, ok := ix.get(id)
	return ok, nil
}

// HoldsModule says whether this store's project has the module: its module
// object is indexed, or its folder exists under the project's modules folder
// (a module whose objects are not written yet).
func (s *Service) HoldsModule(module string) (bool, error) {
	if module == "" {
		return false, nil
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return false, err
	}
	if ix.hasModule(module) {
		return true, nil
	}
	fi, err := os.Stat(filepath.Join(s.CorpusRoot(), s.modulesDirName(), module))
	return err == nil && fi.IsDir(), nil
}

// hasModule says whether an indexed object belongs to the module.
func (ix *objectIndex) hasModule(module string) bool {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	for _, e := range ix.byID {
		if e.Module == module {
			return true
		}
	}
	return false
}
