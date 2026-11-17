// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/extensions/code/assistant"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WHAT'S NEXT ON A RELEASE — the scope narrowed to the work aimed at a release
// or an increment, as planification answers it (its scope: own targets, then
// the module releases' and the increments' shipping in it). Work aimed at no
// release is backlog: it falls out.

// ScopeReader answers the ids a release or an increment holds; ReleaseLister
// the releases and increments of a module (or of every module) a person can
// narrow What's Next to. The caller wires them - over the bus, in local/ - so
// this package speaks to no transport.
type (
	ScopeReader   func(ctx context.Context, id string) ([]string, error)
	ReleaseLister func(ctx context.Context, module string) ([]ReleaseChoice, error)
)

// ReleaseChoice is one release or increment What's Next can be narrowed to.
type ReleaseChoice struct {
	ID, Label string
}

// WithScope wires the readers What's Next narrows to a release with; without
// them a release is refused.
func (s *Service) WithScope(scope ScopeReader, list ReleaseLister) *Service {
	s.readScope, s.listReleases = scope, list
	return s
}

// scoped narrows a snapshot to a release when one is asked; kept is what
// stays (nil: everything).
func (s *Service) scoped(ctx context.Context, cwm cwmapi.CWMService, snap assistant.Snapshot, release string) (assistant.Snapshot, map[string]bool, error) {
	if release == "" {
		return snap, nil, nil
	}
	if s.readScope == nil {
		return snap, nil, fmt.Errorf("%w: What's Next cannot read a release's scope here", cwmapi.ErrInvalidInput)
	}
	ids, err := s.readScope(ctx, release)
	if err != nil {
		return snap, nil, fmt.Errorf("the scope of %s: %w", release, err)
	}
	return KeepScope(snap, ids, ParentReader(ctx, cwm))
}

// ParentReader answers an object's parent through the index ("" when none).
func ParentReader(ctx context.Context, cwm cwmapi.CWMService) func(string) string {
	return func(id string) string {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id})
		if err != nil {
			return ""
		}
		return got.Object.Parent
	}
}

// KeepScope keeps of a snapshot what a release's scope holds: the goals and
// features it names, the goals and features under one of them or under one
// kept (a sub-feature of a feature serving a kept goal), the features
// serving a kept goal, and the criteria of the kept features. A criterion
// under no feature and everything else falls out. kept answers what stayed.
func KeepScope(snap assistant.Snapshot, scope []string, parentOf func(string) string) (assistant.Snapshot, map[string]bool, error) {
	in := map[string]bool{}
	for _, id := range scope {
		in[id] = true
	}
	kept := map[string]bool{}
	under := func(id string) bool {
		for p, hops := parentOf(id), 0; p != "" && hops < 16; p, hops = parentOf(p), hops+1 {
			if in[p] || kept[p] {
				return true
			}
		}
		return false
	}
	var out assistant.Snapshot
	for _, g := range snap.Goals {
		if in[g.ID] || under(g.ID) {
			kept[g.ID] = true
		}
	}
	for _, f := range snap.Features {
		keep := in[f.ID] || under(f.ID)
		for _, g := range f.Serves {
			keep = keep || kept[g]
		}
		if keep {
			kept[f.ID] = true
			out.Features = append(out.Features, f)
		}
	}
	for _, g := range snap.Goals {
		if !kept[g.ID] {
			continue
		}
		var serving []string
		for _, f := range g.ServingUS {
			if kept[f] {
				serving = append(serving, f)
			}
		}
		g.ServingUS = serving
		out.Goals = append(out.Goals, g)
	}
	for _, c := range snap.Criteria {
		if c.FeatureID != "" && kept[c.FeatureID] {
			kept[c.ID] = true
			out.Criteria = append(out.Criteria, c)
		}
	}
	return out, kept, nil
}

// keptRequests keeps the requests to triage that the scope holds: filed on
// what stayed, or serving it.
func keptRequests(reqs []planapi.Request, kept map[string]bool) []planapi.Request {
	if kept == nil {
		return reqs
	}
	var out []planapi.Request
	for _, r := range reqs {
		keep := kept[r.ID]
		for _, w := range r.Serves {
			keep = keep || kept[w]
		}
		if keep {
			out = append(out, r)
		}
	}
	return out
}
