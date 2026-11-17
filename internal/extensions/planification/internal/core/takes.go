// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"slices"
	"sort"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE REVERSED PLANNING EDGE (rule):
// a module object - a goal, a feature, a use case, a decision, a rule -
// carries no edge to an increment or a release: the increment or the release
// lists it, by its `takes` edge. A request keeps its own `targets` edge.
// Ending, dropping or moving an increment so leaves the modules untouched.
//
// Every reader goes through two helpers - workOf (what a holder holds) and
// takerOf / takersOf (what holds an object) -, every writer through retarget.
// The workspace is migrated (cwm step 36 -> 37): only a request carries a
// `targets` edge (the edge type's source), and the architecture guard
// TestNoModuleObjectPointsAtAnIncrement refuses a module object carrying one.

// ownsTarget is whether an object's link to its increment or release lives
// on the object itself - a request's `targets` - rather than on the holder.
func ownsTarget(o cwmapi.Object) bool { return o.Type == api.RequestType }

// workOf is what an increment or a release holds, sorted and once each: the
// module work it takes and the requests whose own `targets` edge names it.
func workOf(ctx context.Context, cwm cwmapi.CWMService, holder string) ([]string, error) {
	ids, err := incoming(ctx, cwm, holder, api.EdgeTargets)
	if err != nil {
		return nil, err
	}
	h, _, err := loadAny(ctx, cwm, holder)
	if err != nil {
		return ids, nil
	}
	for _, id := range edgesTo(h, api.EdgeTakes) {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// takersOf is every increment and release taking an object, sorted. A view
// carrying the read cache reads every `takes` edge once - one reverse index -,
// never an incoming query per object in a loop.
func takersOf(ctx context.Context, cwm cwmapi.CWMService, id string) []string {
	c, cached := ctx.Value(readCacheKey{}).(*readCache)
	if !cached {
		ids, _ := incoming(ctx, cwm, id, api.EdgeTakes)
		return ids
	}
	c.mu.Lock()
	idx := c.takers
	c.mu.Unlock()
	if idx == nil {
		idx = map[string][]string{}
		if out, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{Kind: cwmapi.EdgeType(api.EdgeTakes)}); err == nil {
			for _, e := range out.Edges {
				if string(e.Kind) == api.EdgeTakes && !slices.Contains(idx[e.To], e.From) {
					idx[e.To] = append(idx[e.To], e.From)
				}
			}
		}
		for k := range idx {
			sort.Strings(idx[k])
		}
		c.mu.Lock()
		if c.takers == nil {
			c.takers = idx
		}
		idx = c.takers
		c.mu.Unlock()
	}
	return slices.Clone(idx[id])
}

// liveTaker is whether a holder's state still moves what it takes: an
// increment neither done nor dropped, a release neither released nor
// dropped. A done or dropped one keeps its takes as its history.
func liveTaker(o cwmapi.Object) bool {
	switch releaseState(o) {
	case api.IncrementDone, api.ReleaseReleased, api.IncrementDropped:
		return false
	}
	return true
}

// takerOf is the increment or release an object is aimed at: a request's own
// `targets`; for module work its one live taker, else the latest that ended
// (done or released before dropped); "" with none.
func takerOf(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) string {
	if ownsTarget(o) {
		return edgeTo(o, api.EdgeTargets)
	}
	if releaseLike(o) {
		return ""
	}
	best, rank := "", -1
	for _, id := range takersOf(ctx, cwm, o.ID) {
		h, _, err := loadAny(ctx, cwm, id)
		if err != nil {
			continue
		}
		r := 0
		switch {
		case liveTaker(h):
			r = 2
		case releaseState(h) != api.IncrementDropped:
			r = 1
		}
		if r >= rank {
			best, rank = id, r
		}
	}
	return best
}

// liveTakers is the live increments and releases taking an object: more than
// one is written behind the service (planning check reports it).
func liveTakers(ctx context.Context, cwm cwmapi.CWMService, id string) []string {
	var out []string
	for _, h := range takersOf(ctx, cwm, id) {
		if ho, _, err := loadAny(ctx, cwm, h); err == nil && liveTaker(ho) {
			out = append(out, h)
		}
	}
	return out
}

// retake aims module work at an increment or a release ("" takes it out of
// its live one): the holder's `takes` is written first, then every other live
// taker's `takes` is removed - a cut leaves two takers, which planning check
// reports, never none. A done or dropped holder keeps its takes.
func retake(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, release string) (string, error) {
	was := takerOf(ctx, cwm, o)
	if release != "" {
		if err := addEdge(ctx, cwm, release, api.EdgeTakes, o.ID); err != nil {
			return was, err
		}
	}
	for _, h := range liveTakers(ctx, cwm, o.ID) {
		if h == release {
			continue
		}
		if err := dropEdge(ctx, cwm, h, api.EdgeTakes, o.ID); err != nil {
			return was, err
		}
	}
	return was, nil
}

// dropEdge removes an object's edge of a kind to a target, if it has one.
func dropEdge(ctx context.Context, cwm cwmapi.CWMService, id, kind, to string) error {
	o, module, err := loadAny(ctx, cwm, id)
	if err != nil {
		return err
	}
	edges := o.Edges[:0:0]
	for _, e := range o.Edges {
		if !(e.Kind == kind && e.To == to) {
			edges = append(edges, e)
		}
	}
	if len(edges) == len(o.Edges) {
		return nil
	}
	o.Edges = edges
	_, err = cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	return err
}
