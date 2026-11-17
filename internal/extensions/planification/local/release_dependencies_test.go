// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A RELEASE'S DEPENDENCIES: each increment of the
// release a frame holding its work, every depends-on an arrow, the work
// outside the release that something waits on a dotted box saying the
// increment and release it is scheduled in and how it stands; each increment
// ready to run, waiting (on what) or running (who holds its work).
func TestAReleaseShowsWhatWaitsForWhat(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel, inc := releases(t, ctx), increments(t, ctx)
	a := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	b := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.5.0"})
	invoices, reminders, dunning := increment(t, ctx, "Invoices are mailed"), increment(t, ctx, "Reminders are mailed"), increment(t, ctx, "Dunning letters")
	for _, s := range []struct{ id, in string }{{invoices.ID, a.ID}, {reminders.ID, a.ID}, {dunning.ID, b.ID}} {
		if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: s.id, In: s.in}); err != nil {
			t.Fatal(err)
		}
	}
	edge := func(from, to string) {
		t.Helper()
		if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: from, To: to, Kind: cwmapi.EdgeType(api.EdgeDependsOn)}); err != nil {
			t.Fatal(err)
		}
	}
	work := func(in, title string) string {
		t.Helper()
		f := want(t, ctx, cwm, "feature", title)
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in}); err != nil {
			t.Fatal(err)
		}
		out, err := req.File(ctx, api.FileInput{Module: "billing", Title: title + " - build", Kind: api.KindBuild, By: person})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: out.Request.ID, To: f, Kind: cwmapi.EdgeType(api.EdgeServes)}); err != nil {
			t.Fatal(err)
		}
		return out.Request.ID
	}
	mail := work(invoices.ID, "Send by mail")
	remind := work(reminders.ID, "Remind by mail")
	letter := work(dunning.ID, "Print the letter")
	edge(reminders.ID, invoices.ID) // Reminders waits on Invoices
	edge(remind, letter)            // its request waits on Dunning's, outside the release

	page := func() (map[string]map[string]any, [][2]string, string) {
		t.Helper()
		def, err := rel.DependenciesPage(ctx, api.GetInput{ID: a.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("the Dependencies panel must display: %v", err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		var tree struct {
			Children []struct {
				Kind    string `json:"kind"`
				Content string `json:"content"`
				Data    struct {
					Nodes []map[string]any            `json:"nodes"`
					Edges []struct{ From, To string } `json:"edges"`
				} `json:"data"`
			} `json:"children"`
		}
		_ = json.Unmarshal(raw, &tree)
		boxes, summary := map[string]map[string]any{}, ""
		var edges [][2]string
		for _, c := range tree.Children {
			if c.Kind == "text" {
				summary = c.Content
			}
			for _, n := range c.Data.Nodes {
				boxes[n["id"].(string)] = n
			}
			for _, e := range c.Data.Edges {
				edges = append(edges, [2]string{e.From, e.To})
			}
		}
		return boxes, edges, summary
	}
	badges := func(b map[string]any) string {
		raw, _ := json.Marshal(b["badges"])
		return string(raw)
	}

	// The network: frames, work inside, arrows from what is waited on.
	boxes, edges, summary := page()
	if boxes[invoices.ID] == nil || boxes[reminders.ID] == nil || boxes[dunning.ID] != nil {
		t.Fatalf("the frames: %v; want the release's two increments, not the other release's", boxIDs(boxes))
	}
	if boxes[mail]["group"] != invoices.ID || boxes[remind]["group"] != reminders.ID {
		t.Errorf("the work inside its increment's frame: %v / %v", boxes[mail], boxes[remind])
	}
	for _, want := range [][2]string{{invoices.ID, reminders.ID}, {letter, remind}} {
		if !slices.Contains(edges, want) {
			t.Errorf("no arrow %s -> %s in %v", want[0], want[1], edges)
		}
	}
	// The work outside: dotted, where it is scheduled, how it stands.
	ext := boxes[letter]
	if ext == nil || ext["frame"] != "dotted" || ext["group"] != nil || !strings.Contains(badges(ext), "in Dunning letters") || !strings.Contains(badges(ext), "not started") {
		t.Errorf("the external box: %v; want dotted, in Dunning letters and its release, not started", ext)
	}
	// Readiness: Invoices ready, Reminders waiting on it.
	if boxes[invoices.ID]["readiness"] != api.ReadinessReady || !strings.Contains(badges(boxes[invoices.ID]), "ready to run") {
		t.Errorf("Invoices: %v; want ready to run", boxes[invoices.ID])
	}
	if boxes[reminders.ID]["readiness"] != api.ReadinessWaiting || !strings.Contains(badges(boxes[reminders.ID]), "waits on Invoices are mailed") {
		t.Errorf("Reminders: %v; want waiting on Invoices", boxes[reminders.ID])
	}
	if !strings.Contains(summary, "Ready to run: Invoices are mailed") {
		t.Errorf("the summary: %q; want the increments ready to run named", summary)
	}

	// Someone takes Invoices' work: it reads running, with who.
	intake := stepOf(t, ctx, mail, api.StepIntake)
	if _, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: intake.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	r, err := inc.Readiness(ctx, api.GetInput{ID: invoices.ID})
	if err != nil || r.State != api.ReadinessRunning || !slices.Equal(r.HeldBy, []string{session.ID}) {
		t.Fatalf("Invoices after its intake is pulled: %+v %v; want running, held by %s", r, err, session.ID)
	}
	if boxes, _, _ := page(); boxes[mail]["status"] != "running" || !strings.Contains(badges(boxes[invoices.ID]), "running: "+session.ID) {
		t.Errorf("the running work and frame: %v / %v", boxes[mail], boxes[invoices.ID])
	}
}

func boxIDs(m map[string]map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
