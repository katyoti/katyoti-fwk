// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Edge validation (phase 7e, identity-and-index.md §3): every edge an object
// carries names a DECLARED kind, satisfies the kind's `from_capabilities`
// against the source's EFFECTIVE set, and points at a target of the declared
// kind. The three modes are the type rule's — off with no declarations, warn
// logs, error refuses — with two refusals in EVERY mode, because they are
// broken rules rather than migration gaps: an edge that crosses a project
// (the grammar cannot say it; a URI can) and a kind declared as a FIELD found
// in an edges list (stored once means stored in one place).

// qualifyEdges resolves every bare edge target of obj (qualifyTarget), from the
// SOURCE object's own id — never from its file path (filing is not identity).
// Hand-written files may be short; what the index holds and the writers write
// is the target's id.
func qualifyEdges(obj api.Object, exists func(id string) bool) api.Object {
	if len(obj.Edges) == 0 {
		return obj
	}
	module, err := api.ModuleOfObjectID(obj.ID)
	if err != nil || module == "" {
		return obj
	}
	out := obj
	out.Edges = make([]api.ObjectEdge, len(obj.Edges))
	copy(out.Edges, obj.Edges)
	for i, e := range out.Edges {
		out.Edges[i].To = qualifyTarget(module, e.To, exists)
	}
	return out
}

// qualifyTarget is what a bare target (no `.`, `/` or `:`) of an edge from
// module names:
//
//   - `module.target` when that object exists — a short id inside one module;
//   - else `target` itself when an object has that id — the bare ids are the
//     module objects' and the project's, so an edge reaches another module;
//   - else `module.target`, which names nothing: the dangling link is refused
//     on write (error mode) and reported on the object.
//
// exists nil (nothing to look in) keeps the in-module reading. A qualified,
// path or URI target is returned as it is.
func qualifyTarget(module, to string, exists func(id string) bool) string {
	if to == "" || strings.ContainsAny(to, "./:") {
		return to
	}
	q := api.QualifyObjectID(module, to)
	if exists == nil || exists(q) {
		return q
	}
	if exists(to) {
		return to
	}
	return q
}

// checkObjectEdges validates obj.Edges. mode is the type-validation mode.
func (s *Service) checkObjectEdges(obj api.Object, mode, verb string) error {
	if len(obj.Edges) == 0 || s.Storage == nil {
		return nil
	}
	kinds := s.Storage.Types().EdgeTypes()
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s: object %q: "+format, append([]any{api.ErrInvalidInput, verb, obj.ID}, args...)...)
	}
	perKind := map[string]int{}
	for _, e := range obj.Edges {
		// ALWAYS: an edge never crosses a project.
		if api.IsFullName(e.To) {
			return refuse("edge %s -> %s crosses a project — an edge never leaves the project; refer to it by a URI", e.Kind, e.To)
		}
		decl, declared := kinds[api.EdgeType(e.Kind)]
		if !declared {
			if mode == api.TypeValidationError {
				return refuse("edge kind %q is declared by no pack — declare an edge-type for it or drop the edge", e.Kind)
			}
			if mode == api.TypeValidationWarn {
				slog.Warn("cwm: undeclared edge kind on object", "verb", verb, "id", obj.ID, "kind", e.Kind, "to", e.To,
					"hint", "declare an edge-type contribution; this becomes an error after the migration (D5)")
			}
			continue
		}
		// ALWAYS: a field-stored kind has no place in an edges list.
		if decl.StoredAsField() {
			return refuse("edge kind %q is stored as the field %s, never in the edges list — stored once means stored in one place", e.Kind, decl.Field)
		}
		if mode == api.TypeValidationOff {
			continue
		}
		perKind[e.Kind]++
		var problems []string
		for _, need := range decl.FromCapabilities {
			if !s.hasEffective(obj, need) {
				problems = append(problems, fmt.Sprintf("the source lacks capability %q that kind %q requires", need, e.Kind))
			}
		}
		switch decl.TargetKind() {
		case api.EdgeTargetPath:
			if _, err := os.Stat(filepath.Join(s.Config.ObjectsRoot(), filepath.FromSlash(e.To))); err != nil {
				problems = append(problems, fmt.Sprintf("kind %q targets a path and %q is not a file in the project", e.Kind, e.To))
			}
		case api.EdgeTargetURI:
			if !strings.Contains(e.To, ":") {
				problems = append(problems, fmt.Sprintf("kind %q targets a URI and %q is not one", e.Kind, e.To))
			}
		default: // object: a dangling id is a scan LINT, never a refusal (files land in any order)
			if target, ok := s.index().get(e.To); ok {
				for _, need := range decl.ToCapabilities {
					if !s.hasEffective(target, need) {
						problems = append(problems, fmt.Sprintf("the target %s lacks capability %q that kind %q requires", e.To, need, e.Kind))
					}
				}
			}
		}
		if (decl.Cardinality == "1:1" || decl.Cardinality == "N:1") && perKind[e.Kind] > 1 {
			problems = append(problems, fmt.Sprintf("kind %q is %s and the object carries it %d times", e.Kind, decl.Cardinality, perKind[e.Kind]))
		}
		for _, p := range problems {
			if mode == api.TypeValidationError {
				return refuse("%s", p)
			}
			slog.Warn("cwm: edge check", "verb", verb, "id", obj.ID, "problem", p)
		}
	}
	return nil
}

// edgeDecl returns the declaration of a kind, empty when undeclared.
func (s *Service) edgeDecl(kind api.EdgeType) api.EdgeTypeRegistration {
	if s.Storage == nil {
		return api.EdgeTypeRegistration{}
	}
	decl, _ := s.Storage.Types().EdgeType(kind)
	return decl
}
