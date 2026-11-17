// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// edgesByScan is what edges answers, read the long way: every authored edge
// of every entry, kept when it is incident to id in dir.
func edgesByScan(ix *objectIndex, id string, dir api.Direction) []string {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []string
	for _, e := range ix.byID {
		for _, edge := range e.Object.Edges {
			if (e.Object.ID == id && dir != api.DirectionIn) || (edge.To == id && dir != api.DirectionOut) {
				out = append(out, e.Object.ID+" "+edge.Kind+" "+edge.To)
			}
		}
	}
	sort.Strings(out)
	return out
}

func edgeStrings(es []api.Edge) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.From+" "+string(e.Kind)+" "+e.To)
	}
	sort.Strings(out)
	return out
}

// THE INCOMING EDGES FOLLOW EVERY WRITE: edges answers from the
// incoming map once it is built, and the map is kept by put and drop. Over a
// random run of puts (edges added, removed, repeated, a self-edge) and drops,
// every object's in, out and both answers equal a walk of every entry.
func TestIncomingEdgesFollowEveryWrite(t *testing.T) {
	ix := newObjectIndex(t.TempDir(), "")
	ids := []string{"m.A", "m.B", "m.C", "m.D", "m.E", "m.F"}
	kinds := []string{"serves", "impacts"}
	r := rand.New(rand.NewSource(7))
	check := func(step int) {
		t.Helper()
		for _, id := range ids {
			for _, dir := range []api.Direction{api.DirectionIn, api.DirectionOut, ""} {
				got, want := edgeStrings(ix.edges(id, "", dir)), edgesByScan(ix, id, dir)
				if !slices.Equal(got, want) {
					t.Fatalf("step %d, %s %q: edges %v, a walk of every entry %v", step, id, dir, got, want)
				}
			}
		}
	}
	check(-1) // builds the map on an empty index
	for step := 0; step < 400; step++ {
		id := ids[r.Intn(len(ids))]
		if r.Intn(5) == 0 {
			ix.drop(id)
		} else {
			obj := api.Object{ID: id, Type: "goal"}
			for n := r.Intn(4); n > 0; n-- {
				obj.Edges = append(obj.Edges, api.ObjectEdge{Kind: kinds[r.Intn(len(kinds))], To: ids[r.Intn(len(ids))]})
			}
			ix.put("m", filepath.Join(ix.root, id+".cwm.yaml"), obj)
		}
		check(step)
	}
}

// A WALK OVER THE CORPUS IS LINEAR: asking each of 20,000
// objects what points at it reads only its referrers. A walk of every entry
// per question is 400 million reads, seconds of work; the map answers in
// milliseconds. The 2 s budget leaves room for a loaded machine.
func TestIncomingEdgesOfEveryObjectIsLinear(t *testing.T) {
	const n = 20000
	ix := newObjectIndex(t.TempDir(), "")
	ix.mu.Lock()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("m.G-%05d", i)
		obj := api.Object{ID: id, Type: "goal", Edges: []api.ObjectEdge{{Kind: "serves", To: fmt.Sprintf("m.G-%05d", (i+1)%n)}}}
		ix.byID[id] = indexEntry{Object: obj, Module: "m"}
	}
	ix.mu.Unlock()
	start := time.Now()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("m.G-%05d", i)
		if got := ix.edges(id, "", api.DirectionIn); len(got) != 1 {
			t.Fatalf("%s: %d incoming edges, want 1", id, len(got))
		}
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("the incoming edges of %d objects took %s", n, took)
	}
}
