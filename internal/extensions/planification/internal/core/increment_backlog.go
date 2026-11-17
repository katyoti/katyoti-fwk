// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sort"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// incrementShipInURI ships an increment; with no release named, in its
// stream's next planned one.
const incrementShipInURI = "ktw-agents:increments/ship-in"

// Backlog is every open increment that ships in no release, across the
// streams, by priority (P0 first; then the ones a person ran before those only
// defined; then the title), each with its stream and the release « ship in
// the next release » would put it in — and, apart, the open increments
// already in a release, each with that release and where it stands.
func (i *Increments) Backlog(ctx context.Context, in api.IncrementBacklogInput) (api.IncrementBacklogOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.IncrementBacklogOutput{}, err
	}
	all, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: api.IncrementType})
	if err != nil {
		return api.IncrementBacklogOutput{}, err
	}
	out := api.IncrementBacklogOutput{Items: []api.IncrementBacklogItem{}, Affected: []api.IncrementBacklogItem{}}
	names := map[string]string{} // stream and release id → its name
	nameOf := func(id string) string {
		if id == "" {
			return ""
		}
		if n, ok := names[id]; ok {
			return n
		}
		n := id
		if o, _, err := loadAny(ctx, cwm, id); err == nil && o.Name != "" {
			n = o.Name
			if v := releaseFields(o).Version; v != "" && o.Type == api.ReleaseType {
				n = api.ReleaseLabel(v, o.Name)
			}
		}
		names[id] = n
		return n
	}
	next := map[string]string{} // stream → its next planned release
	for _, o := range all.Objects {
		if releaseState(o) != api.IncrementOpen {
			continue
		}
		inc := toIncrement(ctx, o)
		it := api.IncrementBacklogItem{Increment: inc, StreamName: nameOf(inc.Stream)}
		if rel := edgeTo(o, api.EdgeShipsIn); rel != "" {
			it.ReleaseName = nameOf(rel)
			if !in.Bare {
				if sum, err := i.Summary(ctx, api.GetInput{ID: o.ID}); err == nil {
					it.Summary = &sum
				}
			}
			out.Affected = append(out.Affected, it)
			continue
		}
		if inc.Stream != "" {
			n, ok := next[inc.Stream]
			if !ok {
				n, _ = nextPlannedRelease(ctx, cwm, inc.Stream)
				next[inc.Stream] = n
			}
			it.Next, it.NextName = n, nameOf(n)
		}
		out.Items = append(out.Items, it)
	}
	for _, items := range [][]api.IncrementBacklogItem{out.Items, out.Affected} {
		sort.SliceStable(items, func(a, b int) bool {
			x, y := items[a].Increment, items[b].Increment
			if x.Priority != y.Priority {
				return x.Priority < y.Priority
			}
			if x.Running != y.Running {
				return x.Running
			}
			return x.Title < y.Title
		})
	}
	return out, nil
}
