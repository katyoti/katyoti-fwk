// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ERRORS ARE OBJECT DATA (api/object_errors.go). Write refuses, read reports:
// a write that would ADD a problem is refused by the write path; what is READ
// — files on disk, however they got there — is loaded anyway and carries its
// problems, computed here.
//
// Two parts per object, kept apart because they change for different reasons:
//
//   - validity: the object against its declarations (type, capability groups,
//     edges, containment, its workflow states) — changes when the object is
//     written or the declarations change;
//   - links: its parent and its object edges point at objects that exist —
//     also changes when ANOTHER object appears or disappears.
//
// The index marks what a write touched (put / drop mark the object, and the
// objects pointing at it when an id appears or goes); the next read recomputes
// only those. A catalog refresh marks everything, and so does a scan that
// found an object added, modified or removed; a scan that found nothing
// marks nothing.

type problemStore struct {
	mu       sync.Mutex
	loaded   bool
	validity map[string][]api.ObjectDiagnostic
	links    map[string][]api.ObjectDiagnostic
}

// problemsOf returns an object's errors, current.
func (s *Service) problemsOf(id string) []api.ObjectDiagnostic {
	s.ensureProblems()
	s.problems.mu.Lock()
	defer s.problems.mu.Unlock()
	return joinDiagnostics(joinDiagnostics(s.problems.validity[id], s.problems.links[id]), s.hooks().failuresOf(id))
}

// inError reports which of ids carry errors, in the order given.
func (s *Service) inError(ids []string) []string {
	s.ensureProblems()
	s.problems.mu.Lock()
	defer s.problems.mu.Unlock()
	var out []string
	for _, id := range ids {
		if len(s.problems.validity[id]) > 0 || len(s.problems.links[id]) > 0 || len(s.hooks().failuresOf(id)) > 0 {
			out = append(out, id)
		}
	}
	return out
}

func joinDiagnostics(a, b []api.ObjectDiagnostic) []api.ObjectDiagnostic {
	if len(a)+len(b) == 0 {
		return nil
	}
	out := make([]api.ObjectDiagnostic, 0, len(a)+len(b))
	return append(append(out, a...), b...)
}

// ensureProblems brings the store current: everything after a scan or a
// declaration change, else only the objects the index marked.
func (s *Service) ensureProblems() {
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return
	}
	all, dirty := ix.takeDirty()
	s.problems.mu.Lock()
	defer s.problems.mu.Unlock()
	if all || !s.problems.loaded {
		s.problems.validity = map[string][]api.ObjectDiagnostic{}
		s.problems.links = map[string][]api.ObjectDiagnostic{}
		dirty = nil
		for _, o := range ix.list(api.ObjectListInput{}) {
			dirty = append(dirty, o.ID)
		}
		s.problems.loaded = true
	}
	for _, id := range dirty {
		resident, ok := ix.get(id)
		if !ok {
			delete(s.problems.validity, id)
			delete(s.problems.links, id)
			continue
		}
		path, _ := ix.pathOf(id)
		full, err := ReadObjectFile(path)
		if err != nil {
			s.problems.validity[id] = []api.ObjectDiagnostic{{Code: api.DiagnosticUnreadable, Label: "the object file cannot be read", Description: err.Error()}}
		} else {
			setOrDelete(s.problems.validity, id, append(s.validityProblems(full), s.sizeProblem(path, full)...))
		}
		setOrDelete(s.problems.links, id, s.linkProblems(resident))
	}
}

func setOrDelete(m map[string][]api.ObjectDiagnostic, id string, d []api.ObjectDiagnostic) {
	if len(d) == 0 {
		delete(m, id)
		return
	}
	m[id] = d
}

// validityProblems checks the object as a write would, in error mode — its
// declarations and the sys- reservation — plus its workflow states.
func (s *Service) validityProblems(full api.Object) []api.ObjectDiagnostic {
	var out []api.ObjectDiagnostic
	for _, err := range s.validationErrors(full, "read", api.TypeValidationError) {
		out = append(out, api.ObjectDiagnostic{Code: api.DiagnosticInvalid, Label: shortReason(err), Description: err.Error()})
	}
	// The sys- reservation, which every write refuses: a module file written
	// by hand that breaks it is read, and carries the problem.
	if err := s.checkSystemModuleName(full, "read"); err != nil {
		out = append(out, api.ObjectDiagnostic{Code: api.DiagnosticInvalid, Label: shortReason(err), Description: err.Error()})
	}
	if p := s.lifecycleProblem(full); p != "" {
		out = append(out, api.ObjectDiagnostic{Code: api.DiagnosticLifecycle, Label: p, Description: p})
	}
	return out
}

// shortReason drops what the object's own pane already says — the error's
// "invalid input: read: object "<id>"" prefix — and keeps the reason.
func shortReason(err error) string {
	msg := err.Error()
	if errors.Is(err, api.ErrInvalidInput) {
		if i := strings.Index(msg, api.ErrInvalidInput.Error()+": "); i >= 0 {
			msg = msg[i+len(api.ErrInvalidInput.Error())+2:]
		}
	}
	msg = strings.TrimPrefix(msg, "read: ")
	return objectPrefix.ReplaceAllString(msg, "")
}

// objectPrefix is `object "<id>": ` or `object "<id>" ` at the start of a reason.
var objectPrefix = regexp.MustCompile(`^object "[^"]*":? `)

// linkProblems reports a parent or an object edge pointing at nothing. A path
// or URI edge is the validity check's. An edge of an undeclared kind (itself a
// validity error when kinds are declared) targets an object, the default, when
// its target reads as an object id.
func (s *Service) linkProblems(obj api.Object) []api.ObjectDiagnostic {
	return s.linkProblemsWith(obj, nil)
}

// linkProblemsWith is linkProblems with known answering whether an id is an
// object (nil: the index); an ended (archived) id is always known.
func (s *Service) linkProblemsWith(obj api.Object, known func(string) bool) []api.ObjectDiagnostic {
	ix := s.index()
	if known == nil {
		known = func(id string) bool { _, ok := ix.get(id); return ok }
	}
	// An archived id has ENDED, not vanished: a link to it is valid
	// (ended.go). A write's check (writeLinkProblems) comes through here too.
	indexed := known
	known = func(id string) bool {
		if indexed(id) {
			return true
		}
		_, ended := s.isEnded(id)
		return ended
	}
	var out []api.ObjectDiagnostic
	if p := obj.ParentID(); p != "" {
		if !known(p) && p != obj.ID {
			out = append(out, api.ObjectDiagnostic{Code: api.DiagnosticDanglingParent,
				Label:       "parent " + p + " does not exist",
				Description: "the parent " + p + " is not an object of the project"})
		}
	}
	if len(obj.Edges) == 0 {
		return out
	}
	var kinds map[api.EdgeType]api.EdgeTypeRegistration
	if s.Storage != nil {
		kinds = s.Storage.Types().EdgeTypes()
	}
	for _, e := range obj.Edges {
		decl, declared := kinds[api.EdgeType(e.Kind)]
		switch {
		case declared && decl.StoredAsField():
			continue
		case declared && (decl.TargetKind() == api.EdgeTargetPath || decl.TargetKind() == api.EdgeTargetURI):
			continue
		case !declared && !readsAsObjectID(e.To):
			continue
		}
		if api.IsFullName(e.To) {
			continue // an edge never leaves the project: the validity check says so
		}
		if !known(e.To) && e.To != obj.ID {
			out = append(out, api.ObjectDiagnostic{Code: api.DiagnosticDanglingLink,
				Label:       e.Kind + " → " + e.To + " does not exist",
				Description: "the " + e.Kind + " edge targets " + e.To + ", which is not an object of the project"})
		}
	}
	return out
}

// ObjectErrorList lists every object carrying errors, and the files CWM could
// not read.
func (s *Service) ObjectErrorList(_ stdctx.Context, in api.ObjectErrorListInput) (api.ObjectErrorListOutput, error) {
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectErrorListOutput{}, err
	}
	var within map[string]bool
	if in.Within != "" {
		if _, ok := ix.get(in.Within); !ok {
			return api.ObjectErrorListOutput{}, fmt.Errorf("%w: within: object %q", api.ErrNotFound, in.Within)
		}
		within = ix.subtree(in.Within)
	}
	s.ensureProblems()
	out := api.ObjectErrorListOutput{Objects: []api.ObjectErrors{}}
	for _, o := range ix.list(api.ObjectListInput{Module: in.Module}) {
		if within != nil && !within[o.ID] {
			continue
		}
		errs := s.problemsOf(o.ID)
		if len(errs) == 0 {
			continue
		}
		module, _ := ix.moduleOf(o.ID)
		path, _ := ix.pathOf(o.ID)
		out.Objects = append(out.Objects, api.ObjectErrors{ID: o.ID, Module: module, Type: o.Type, Name: o.Name, Path: ix.rel(path), Errors: errs})
	}
	if in.Module == "" && in.Within == "" {
		out.Unreadable = ix.unreadableFiles()
	}
	sort.Slice(out.Objects, func(i, j int) bool { return out.Objects[i].ID < out.Objects[j].ID })
	out.Total = len(out.Objects) + len(out.Unreadable)
	return out, nil
}

// readsAsObjectID: no path separator, no URI scheme, and a module segment.
func readsAsObjectID(s string) bool {
	if strings.ContainsAny(s, `/\: `) {
		return false
	}
	_, err := api.ModuleOfObjectID(s)
	return err == nil
}
