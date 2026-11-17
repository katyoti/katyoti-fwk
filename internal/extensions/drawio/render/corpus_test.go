// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// corpusDir holds the reference corpus: <name>.json is a LogicalDiagram
// rendered with layout, <name>.drawio a diagram scored as it is (drawn by
// hand), ceilings.json the score each must not exceed.
var corpusDir = filepath.Join("testdata", "corpus")

// ceiling is the worst a corpus entry may score. Counts only: extent and
// density are reported, not ratcheted.
type ceiling struct {
	Overlaps          int `json:"overlaps"`
	Crossings         int `json:"crossings"`
	EdgesThroughNodes int `json:"edgesThroughNodes"`
	Bends             int `json:"bends"`
	ZoneEscapes       int `json:"zoneEscapes"`
	ZoneOverlaps      int `json:"zoneOverlaps"`
	ZoneIntrusions    int `json:"zoneIntrusions"`
	LabelCollisions   int `json:"labelCollisions"`
	TextOverlaps      int `json:"textOverlaps"`
	Duplicates        int `json:"duplicates"`
	TextOverflows     int `json:"textOverflows"`
	BoxCovers         int `json:"boxCovers"`
}

func ceilingOf(q *drawioapi.QualityReport) ceiling {
	return ceiling{q.Overlaps, q.Crossings, q.EdgesThroughNodes, q.Bends, q.ZoneEscapes, q.ZoneOverlaps, q.ZoneIntrusions, q.LabelCollisions,
		q.TextOverlaps, q.Duplicates, q.TextOverflows, q.BoxCovers}
}

// TestCorpus renders every corpus model with ELK layout, scores it with
// every hand-drawn entry, and refuses a score worse than its ceiling. A
// better score asks for the ceiling to be lowered (the ratchet only goes
// down). KTW_REGEN_CORPUS=1 rewrites ceilings.json. Needs bun (ELK).
func TestCorpus(t *testing.T) {
	long.Test(t)
	needBun(t)
	ctx := drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatal(err)
	}
	var ceilings map[string]ceiling
	raw, _ := os.ReadFile(filepath.Join(corpusDir, "ceilings.json"))
	_ = json.Unmarshal(raw, &ceilings)
	got := map[string]ceiling{}
	regen := os.Getenv("KTW_REGEN_CORPUS") == "1"

	for _, e := range entries {
		name, ext := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())), filepath.Ext(e.Name())
		if name == "ceilings" || (ext != ".json" && ext != ".drawio") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(corpusDir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var q *drawioapi.QualityReport
			if ext == ".json" {
				var ld drawioapi.LogicalDiagram
				if err := json.Unmarshal(data, &ld); err != nil {
					t.Fatal(err)
				}
				out, err := Emit(ctx, &ld, false)
				if err != nil {
					t.Fatal(err)
				}
				if len(out.Errors) > 0 {
					t.Fatalf("errors: %v", out.Errors)
				}
				q = out.Quality
			} else if q, err = Score(data); err != nil {
				t.Fatal(err)
			}
			if q == nil {
				t.Fatal("no quality report")
			}
			t.Logf("%s: %+v", name, *q)
			got[name] = ceilingOf(q)
			if regen {
				return
			}
			c, ok := ceilings[name]
			if !ok {
				t.Fatalf("no ceiling for %s: run with KTW_REGEN_CORPUS=1 and review", name)
			}
			now := ceilingOf(q)
			if now.Overlaps > c.Overlaps || now.Crossings > c.Crossings || now.EdgesThroughNodes > c.EdgesThroughNodes ||
				now.Bends > c.Bends || now.ZoneEscapes > c.ZoneEscapes || now.ZoneOverlaps > c.ZoneOverlaps ||
				now.ZoneIntrusions > c.ZoneIntrusions || now.LabelCollisions > c.LabelCollisions ||
				now.TextOverlaps > c.TextOverlaps || now.Duplicates > c.Duplicates || now.TextOverflows > c.TextOverflows || now.BoxCovers > c.BoxCovers {
				t.Errorf("%s got worse: %+v, ceiling %+v", name, now, c)
			} else if now != c {
				t.Errorf("%s got better (%+v, ceiling %+v): lower the ceiling with KTW_REGEN_CORPUS=1", name, now, c)
			}
		})
	}
	if regen {
		keys := make([]string, 0, len(got))
		for k := range got {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		ordered := make(map[string]ceiling, len(got))
		for _, k := range keys {
			ordered[k] = got[k]
		}
		out, _ := json.MarshalIndent(ordered, "", "  ")
		if err := os.WriteFile(filepath.Join(corpusDir, "ceilings.json"), append(out, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
