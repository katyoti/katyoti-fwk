// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// EVERY SHIP-IN REFUSES AN INCREMENT OF ANOTHER STREAM (
// serves F-078): the stream check lives in the one place every ship-in passes
// - a person's ship-in, a plan-for, a cut, a freeze - not in the person's path
// only. A freeze takes in what rides along in its stream; an increment whose
// own stream is another (data edited by hand: a second in-stream edge) is
// refused, the freeze naming it, and never shipped across streams.
func TestEveryShipInRefusesAnIncrementOfAnotherStream(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	sm, rel, inc := streams(t, ctx), releases(t, ctx), increments(t, ctx)
	mk := func(name string) string {
		t.Helper()
		out, err := sm.Create(ctx, api.CreateStreamInput{Module: "billing", Name: name, StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
		if err != nil {
			t.Fatal(err)
		}
		return out.Stream.ID
	}
	product, docs := mk("Product"), mk("Docs")
	next := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: product})

	made, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: docs, Title: "Docs site", By: person})
	if err != nil {
		t.Fatal(err)
	}
	crossed := made.Increment.ID
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: crossed, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatal(err)
	}
	// Edited by hand: listed in Product's stream too, its own stream still Docs.
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: crossed})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeInStream, To: product})
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	if g, _ := inc.Get(ctx, api.GetInput{ID: crossed}); g.Increment.Stream != docs {
		t.Fatalf("the hand-edited increment's stream: %q; want its own %q first", g.Increment.Stream, docs)
	}

	_, err = rel.Freeze(ctx, api.MoveInput{ID: next.ID, By: person})
	if err == nil || !strings.Contains(err.Error(), crossed) || !strings.Contains(err.Error(), "its own stream") {
		t.Errorf("freezing %s with %s of stream %s riding along: %v; want the take-in refused, naming it", next.ID, crossed, docs, err)
	}
	if g, _ := inc.Get(ctx, api.GetInput{ID: crossed}); g.Increment.ShipsIn != "" {
		t.Errorf("%s of stream %s ships in %s of stream %s; want it shipped nowhere", crossed, docs, g.Increment.ShipsIn, product)
	}
}
