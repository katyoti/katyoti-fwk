// SPDX-License-Identifier: Apache-2.0

package core

import (
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE ENDED IDS. An archived object (ObjectArchive) left the index: its file
// sits, unchanged, in its module's `.ktw/_history/<segment>/`, which no scan
// reads. Its id has ENDED, not vanished: a link to it is valid (never a
// dangling link, accepted on write), the pane shows it as ended, ObjectGet
// says where it went, and the id is never minted again.
//
// The set is read lazily, one module at a time, from the history folders'
// file names; forgotten whole at ObjectScan (an outside edit of `_history` is
// seen then) and kept by ObjectArchive itself, which adds the id BEFORE it
// drops the object from the index — a reader never sees the id neither
// indexed nor ended.

// endedSet is the ended ids by module, each loaded on first use.
type endedSet struct {
	mu sync.RWMutex
	// loaded are the modules read; byID the ended id → its file,
	// project-relative.
	loaded map[string]bool
	byID   map[string]string
}

// endedPrefix is the id an archived file name carries: `RQ-047` of
// `RQ-047-a-slug.cwm.yaml`.
var endedPrefix = regexp.MustCompile(`^[A-Za-z]+-\d+`)

// ended is the service's set, made on first use.
func (s *Service) ended() *endedSet {
	s.endedOnce.Do(func() {
		s.endedIDs = &endedSet{loaded: map[string]bool{}, byID: map[string]string{}}
	})
	return s.endedIDs
}

// endedModuleOf is the module whose history an id would be in: the one its id
// names (a bare id is a module object's, its own module); "" for what does not
// read as an object id (a path, a URI).
func endedModuleOf(id string) string {
	if !readsAsObjectID(id) {
		return ""
	}
	m, err := api.ModuleOfObjectID(id)
	if err != nil {
		return ""
	}
	return m
}

// isEnded reports an archived id, and where its file is (project-relative).
func (s *Service) isEnded(id string) (string, bool) {
	module := endedModuleOf(id)
	if module == "" {
		return "", false
	}
	e := s.ended()
	s.loadEnded(module)
	e.mu.RLock()
	defer e.mu.RUnlock()
	p, ok := e.byID[id]
	return p, ok
}

// endedIDsOf is every ended id of a module (the mint counts them).
func (s *Service) endedIDsOf(module string) []string {
	e := s.ended()
	s.loadEnded(module)
	e.mu.RLock()
	defer e.mu.RUnlock()
	var out []string
	for id := range e.byID {
		if endedModuleOf(id) == module {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// noteEnded adds an id ObjectArchive just archived. The module is loaded
// first, so the lazy read never runs over it afterwards.
func (s *Service) noteEnded(id, rel string) {
	e := s.ended()
	s.loadEnded(endedModuleOf(id))
	e.mu.Lock()
	e.byID[id] = rel
	e.mu.Unlock()
}

// forgetEnded drops every module read: the next lookup reads them again
// (ObjectScan).
func (s *Service) forgetEnded() {
	e := s.ended()
	e.mu.Lock()
	e.loaded = map[string]bool{}
	e.byID = map[string]string{}
	e.mu.Unlock()
}

// loadEnded reads a module's history folders once: the authored module
// folder's and the runtime store's.
func (s *Service) loadEnded(module string) {
	if module == "" {
		return
	}
	e := s.ended()
	e.mu.RLock()
	done := e.loaded[module]
	e.mu.RUnlock()
	if done {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.loaded[module] {
		return
	}
	for _, dir := range s.moduleDirs(module) {
		s.readHistory(e, module, filepath.Join(dir, filepath.FromSlash(api.HistoryFolder)))
	}
	e.loaded[module] = true
}

// moduleDirs are the folders a module has: authored, then runtime.
func (s *Service) moduleDirs(module string) []string {
	var out []string
	if s.Config != nil && s.Config.ObjectsRoot() != "" {
		out = append(out, filepath.Join(s.Config.ObjectsRoot(), s.modulesDirName(), module))
	}
	if store := s.storeObjectsRoot(); store != "" {
		out = append(out, filepath.Join(store, module))
	}
	return out
}

// readHistory files every object file of `<history>/<segment>/` under its id.
// Caller holds e.mu. Two files of one id: the first (sorted) wins, logged.
func (s *Service) readHistory(e *endedSet, module, history string) {
	segs, err := os.ReadDir(history)
	if err != nil {
		return
	}
	for _, seg := range segs {
		if !seg.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(history, seg.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !api.IsObjectFile(f.Name()) {
				continue
			}
			path := filepath.Join(history, seg.Name(), f.Name())
			id := endedIDOf(module, path, f.Name())
			if id == "" {
				continue
			}
			rel := s.relPath(path)
			if prior, dup := e.byID[id]; dup {
				slog.Warn("cwm: two archived files name one id; the first is kept", "id", id, "kept", prior, "ignored", rel)
				continue
			}
			e.byID[id] = rel
		}
	}
}

// endedIDOf is the id of an archived file: the `RQ-047` prefix of its name
// qualified by its module, else (a name without one, or a prefix followed by
// more digits — a dated id) the `id:` the file says.
func endedIDOf(module, path, name string) string {
	base := strings.TrimSuffix(strings.TrimSuffix(name, api.ObjectFileSuffixMD), api.ObjectFileSuffix)
	if p := endedPrefix.FindString(base); p != "" {
		rest := base[len(p):]
		if rest == "" || (rest[0] == '-' && !(len(rest) > 1 && rest[1] >= '0' && rest[1] <= '9')) {
			return api.QualifyObjectID(module, p)
		}
	}
	obj, err := ReadObjectFile(path)
	if err != nil || obj.ID == "" {
		return ""
	}
	return obj.ID
}
