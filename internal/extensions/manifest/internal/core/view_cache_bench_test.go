// SPDX-License-Identifier: Apache-2.0

package core_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
)

// view_cache_bench_test.go — the cost an agent spawn pays to read the same
// immutable content again.
//
// The shape is the real one: a body-free index plus a content store, and the
// skills plugin factory listing every skill body on every agent construction.
// The numbers this produces are the before/after c12 reports.

// benchSkillBody is the decode target — the picker fields plus the heavy L1
// instructions, which is what makes the body worth caching.
type benchSkillBody struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	Hashtags     []string `json:"hashtags"`
}

// benchCatalog builds a catalog of `n` skills with bodies of roughly `size`
// bytes, seeded into a content store on disk. The index itself is body-free,
// so every read goes through the content lane.
func benchCatalog(tb testing.TB, n, size int) *core.MemoryCatalog {
	tb.Helper()
	entries := make([]contribapi.Contribution, 0, n)
	store := core.NewContentStore(tb.TempDir())
	for i := 0; i < n; i++ {
		body, err := json.Marshal(benchSkillBody{
			Name:         fmt.Sprintf("skill-%03d", i),
			Description:  "a skill",
			Instructions: strings.Repeat("x", size),
			Hashtags:     []string{"a", "b"},
		})
		if err != nil {
			tb.Fatal(err)
		}
		hash := core.HashBody(body)
		if err := store.Put(hash, body); err != nil {
			tb.Fatal(err)
		}
		entries = append(entries, contribapi.Contribution{
			Provider: "ktw-docs", Type: "skill",
			ID:     fmt.Sprintf("skill-%03d", i),
			Layers: contribapi.DataDescriptor(hash, int64(len(body))),
		})
	}
	cat := core.NewMemoryCatalog([]*mbapi.Manifest{{
		Provider: "ktw-docs", Contributions: entries,
	}}, nil)
	cat.SetContentLane(store, func(context.Context, string, string) ([]byte, error) {
		tb.Fatal("unexpected bus fetch: everything is in the store")
		return nil, nil
	})
	return cat
}

// BenchmarkAgentSpawn_ListsEverySkill is the plugin-factory path: every agent
// construction lists every skill of the kind, with its body.
func BenchmarkAgentSpawn_ListsEverySkill(b *testing.B) {
	cat := benchCatalog(b, 182, 4096)
	ctx := context.Background()
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "skill", Catalog: cat}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := mbapi.QueryTyped[benchSkillBody](ctx, q)
		if err != nil {
			b.Fatal(err)
		}
		if len(got) != 182 {
			b.Fatalf("got %d skills", len(got))
		}
	}
}

// BenchmarkGetOneSkill_ViaWholeKind is the skills-get-instruction path: the
// agent asks for ONE skill's instructions and the whole kind is materialised
// to find it.
func BenchmarkGetOneSkill_ViaWholeKind(b *testing.B) {
	cat := benchCatalog(b, 182, 4096)
	ctx := context.Background()
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Kind: "skill", Catalog: cat}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := mbapi.QueryTyped[benchSkillBody](ctx, q)
		if err != nil {
			b.Fatal(err)
		}
		var found string
		for _, e := range got {
			if e.Body.Name == "skill-100" {
				found = e.Body.Instructions
			}
		}
		if found == "" {
			b.Fatal("not found")
		}
	}
}

// BenchmarkGetOneSkill_ViaGetOne is the same question asked of one entry.
func BenchmarkGetOneSkill_ViaGetOne(b *testing.B) {
	cat := benchCatalog(b, 182, 4096)
	ctx := context.Background()
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := mbapi.GetOne[benchSkillBody](ctx, q, "ktw-docs:/skill/skill-100")
		if err != nil {
			b.Fatal(err)
		}
		if got.Body.Instructions == "" {
			b.Fatal("empty instructions")
		}
	}
}
