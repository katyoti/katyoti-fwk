// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT IN NO STREAM (RQ-399, serves F-078): the
// views list increments through their stream, so one in none was hidden. An
// increment is created in its stream - named, or its release's - and with
// neither is refused; one in none (older data) stays a guard's concern. Planned
// for or shipped in a release it takes the release's stream (one of another
// stream is refused); created shipping in a release it takes its stream; one
// left in none is listed under « No stream » in Team priorities and is a lint
// finding.

func TestAnIncrementInNoStreamIsNeverHidden(t *testing.T) {
	t.Parallel()
	w := teamPrioritiesWorld(t)
	ctx, inc, rel := w.ctx, w.inc, w.rel
	cwm := w.cwm
	streamOf := func(id string) string {
		t.Helper()
		got, err := inc.Get(ctx, api.GetInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		return got.Increment.Stream
	}
	// An increment is always created in its stream:
	// one in none is older data, stored as such.
	loose := func(title string) string { t.Helper(); return strayIncrement(t, ctx, cwm, "billing", title) }

	// Left in no stream: listed last under « No stream », whatever streams are picked.
	lone := loose("A stray increment")
	for _, in := range []api.TeamPrioritiesInput{{}, {HiddenStreams: []string{w.product, w.docs}}} {
		d := w.data(t, in)
		last := d.Groups[len(d.Groups)-1]
		if last.Kind != api.TeamGroupNoStream || len(last.Lines) != 1 || last.Lines[0].ID != lone {
			t.Fatalf("hidden %v: the last group %+v; want no-stream holding %s", in.HiddenStreams, last, lone)
		}
	}
	page, err := inc.TeamPrioritiesPage(ctx, api.TeamPrioritiesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(page.DSLTree); !strings.Contains(string(b), `"content":"No stream"`) {
		t.Error("the page names no « No stream » group")
	}
	// A lint finding names it.
	lintOf := func(id string) []api.LintFinding {
		t.Helper()
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: id})
		if err != nil {
			t.Fatal(err)
		}
		out, err := rel.Lint(ctx, api.HookInput{Object: &got.Object})
		if err != nil {
			t.Fatal(err)
		}
		return out.Findings
	}
	noStream := func(f api.LintFinding) bool { return f.Rule == "increment-no-stream" }
	if !slices.ContainsFunc(lintOf(lone), noStream) {
		t.Errorf("an increment in no stream: findings %+v; want increment-no-stream", lintOf(lone))
	}
	if slices.ContainsFunc(lintOf(w.inNone), noStream) {
		t.Errorf("an increment in a stream: findings %+v; want no increment-no-stream", lintOf(w.inNone))
	}

	// Planned for a later release: it takes the release's stream.
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: lone, In: w.later, By: person}); err != nil {
		t.Fatalf("plan-for an increment in no stream: %v", err)
	}
	if got := streamOf(lone); got != w.product {
		t.Errorf("planned for %s: stream %q; want %q", w.later, got, w.product)
	}
	if slices.ContainsFunc(lintOf(lone), noStream) {
		t.Error("planned, it is still a no-stream finding")
	}
	for _, g := range w.data(t, api.TeamPrioritiesInput{}).Groups {
		if g.Kind == api.TeamGroupNoStream {
			t.Errorf("planned, a no-stream group remains: %+v", g)
		}
	}

	// Shipped in the next release: it takes the release's stream.
	other := loose("Another stray one")
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: other, In: w.current}); err != nil {
		t.Fatalf("ship-in an increment in no stream: %v", err)
	}
	if got := streamOf(other); got != w.product {
		t.Errorf("shipped in %s: stream %q; want %q", w.current, got, w.product)
	}

	// A release of another stream is refused.
	docsRel := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.0.0", Date: "2099-10-15", Stream: w.docs}).ID
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: w.inNone, In: docsRel}); err == nil {
		t.Error("ship-in a release of another stream: accepted; want refused")
	}
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: w.inNone, In: docsRel, By: person}); err == nil {
		t.Error("plan-for a release of another stream: accepted; want refused")
	}

	// Created with neither a stream nor a release: refused; it names the way out.
	if _, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Title: "Nowhere", By: person}); err == nil {
		t.Error("created with neither a stream nor a release: accepted; want refused")
	} else if !strings.Contains(err.Error(), "stream") {
		t.Errorf("created with neither a stream nor a release: %v; want the refusal naming the stream", err)
	}
	// Created in a stream: in it.
	if made, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Title: "Born in a stream", Stream: w.docs, By: person}); err != nil {
		t.Fatalf("created in a stream: %v", err)
	} else if made.Increment.Stream != w.docs {
		t.Errorf("created in %s: stream %q", w.docs, made.Increment.Stream)
	}

	// Created shipping in a release: it takes its stream; another stream named is refused.
	made, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Title: "Born in a release", ShipsIn: w.current, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if made.Increment.Stream != w.product {
		t.Errorf("created in %s: stream %q; want %q", w.current, made.Increment.Stream, w.product)
	}
	if _, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Title: "Crossed", ShipsIn: w.current, Stream: w.docs, By: person}); err == nil {
		t.Error("created in a release of another stream than the one named: accepted; want refused")
	}
}
