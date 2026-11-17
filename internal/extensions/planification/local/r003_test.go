// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// ONLY THE RELEASE BEING BUILT AND THE NEXT ONE ARE OPEN, NOTHING IS PLANNED
// OUTSIDE A RUN (rule, serves): each
// refusal below is checked by the verb, never remembered, and names the rule.

const ruleR003 = "define now - plan when run"

func manualStream(t *testing.T, ctx context.Context, name string) string {
	t.Helper()
	out, err := streams(t, ctx).Create(ctx, api.CreateStreamInput{Module: "billing", Name: name, StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver, ReleaseMode: api.ModeManual}})
	if err != nil {
		t.Fatal(err)
	}
	return out.Stream.ID
}

// withException records on a release the exception its by actor makes to the
// two open releases of a stream - the shape a person writes (fields.w5_exception).
func withException(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string, by cwmapi.Actor) {
	t.Helper()
	module, _ := cwmapi.ModuleOfObjectID(id)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	if o.Fields == nil {
		o.Fields = map[string]any{}
	}
	o.Fields["w5_exception"] = map[string]any{"by": map[string]any{"id": by.ID, "kind": by.Kind}, "at": "2026-11-14", "note": "a third open release, the person's exception"}
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o}); err != nil {
		t.Fatal(err)
	}
}

// excepted plans a third open release of a stream the way a person does it:
// planned in no stream, their exception recorded, then placed in the stream.
func excepted(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, rel api.ReleaseService, in api.PlanInput) api.Release {
	t.Helper()
	stream := in.Stream
	in.Stream = ""
	r := plan(t, ctx, rel, in)
	withException(t, ctx, cwm, r.ID, person)
	out, err := streams(t, ctx).ReleaseIn(ctx, api.StreamInInput{ID: r.ID, Stream: stream})
	if err != nil {
		t.Fatalf("a person's exception placed in %s: %v; want it in", stream, err)
	}
	return out.Release
}

// A stream holds two open releases: a third is refused (releases create),
// and so is placing one in it (streams release-in) - unless a person recorded
// their exception on it, an agent's is none; another stream and a release in
// none are not refused; once one of the two is dropped, the third is made; a
// cut over two makes nothing and says why, without an error.
func TestR003ThirdOpenReleaseIsRefused(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	product, docs := manualStream(t, ctx, "Product"), manualStream(t, ctx, "Docs")
	built := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: product})
	plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Date: "2027-02-01", Stream: product})

	_, err := rel.Plan(ctx, api.PlanInput{Module: "billing", Version: "1.3.0", Date: "2027-03-01", Stream: product})
	if err == nil || !strings.Contains(err.Error(), ruleR003) || !strings.Contains(err.Error(), built.ID) {
		t.Fatalf("a third open release of %s: %v; want it refused, naming the open ones and %s", product, err, ruleR003)
	}
	plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "0.9.0", Date: "2027-01-15", Stream: docs})
	loose := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "2.0.0"})
	sm := streams(t, ctx)
	if _, err := sm.ReleaseIn(ctx, api.StreamInInput{ID: loose.ID, Stream: product}); err == nil || !strings.Contains(err.Error(), ruleR003) {
		t.Errorf("placing %s in %s over two open releases: %v; want it refused, naming %s", loose.ID, product, err, ruleR003)
	}
	withException(t, ctx, cwm, loose.ID, session)
	if _, err := sm.ReleaseIn(ctx, api.StreamInInput{ID: loose.ID, Stream: product}); err == nil {
		t.Errorf("placing %s in %s on an agent's exception: accepted; want it refused - only a person's counts", loose.ID, product)
	}
	// The person's exceptions count in none of the two - as KaTyoti holds
	// 0.11.0 and 0.12.0 beside 0.9.0 and 0.10.0 - and free none of them.
	excepted(t, ctx, cwm, rel, api.PlanInput{Module: "billing", Version: "1.9.0", Date: "2027-06-01", Stream: product})
	excepted(t, ctx, cwm, rel, api.PlanInput{Module: "billing", Version: "1.10.0", Date: "2027-07-01", Stream: product})
	if _, err := rel.Plan(ctx, api.PlanInput{Module: "billing", Version: "1.3.0", Date: "2027-03-01", Stream: product}); err == nil {
		t.Error("a third counted release beside the person's exceptions: planned; want it refused")
	}

	if _, err := rel.Drop(ctx, api.MoveInput{ID: built.ID, By: person, Note: "not this one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := rel.Drop(ctx, api.MoveInput{ID: loose.ID, By: person, Note: "not this one"}); err != nil {
		t.Fatal(err)
	}
	// One of the two dropped: the next is made, the exceptions not counted.
	plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.3.0", Date: "2027-03-01", Stream: product})
	if _, err := rel.Plan(ctx, api.PlanInput{Module: "billing", Version: "1.4.0", Date: "2027-04-01", Stream: product}); err == nil {
		t.Error("a third counted release: planned; want it refused")
	}

	cut, err := streams(t, ctx).Cut(ctx, api.CutInput{ID: product, By: person})
	if err != nil {
		t.Fatalf("a cut over two open releases: %v; want a note, no error", err)
	}
	if cut.Release != nil {
		t.Errorf("a cut over two open releases made %s; want nothing", cut.Release.ID)
	}
}

// ship-in and plan-for refuse a release beyond its stream's first two open
// ones (a third placed in the stream by hand) - unless a person recorded their
// exception on it; an agent's is none.
func TestR003ShipInBeyondTwoIsRefused(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel, inc := releases(t, ctx), increments(t, ctx)
	product := manualStream(t, ctx, "Product")
	plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.1.0", Date: "2027-01-01", Stream: product})
	later := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.2.0", Date: "2027-02-01", Stream: product})
	third := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.3.0", Date: "2027-03-01"})
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: third.ID})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeInStream, To: product})
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	made, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: product, Title: "Invoices by mail", By: person})
	if err != nil {
		t.Fatal(err)
	}
	id := made.Increment.ID

	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: id, In: third.ID}); err == nil || !strings.Contains(err.Error(), ruleR003) {
		t.Errorf("plan-for the third open release %s: %v; want it refused, naming %s", third.ID, err, ruleR003)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: id, In: third.ID}); err == nil || !strings.Contains(err.Error(), ruleR003) {
		t.Errorf("ship-in the third open release %s: %v; want it refused, naming %s", third.ID, err, ruleR003)
	}
	withException(t, ctx, cwm, third.ID, session)
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: id, In: third.ID}); err == nil {
		t.Errorf("plan-for %s on an agent's exception: accepted; want it refused - only a person's counts", third.ID)
	}
	withException(t, ctx, cwm, third.ID, person)
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: id, In: third.ID}); err != nil {
		t.Errorf("plan-for %s on the person's exception: %v; want it planned", third.ID, err)
	}
	if _, err := inc.PlanFor(ctx, api.PlanForInput{ID: id, In: later.ID}); err != nil {
		t.Errorf("plan-for the next one %s: %v; want it planned", later.ID, err)
	}
}

// An increment defined by its delivery goals and not run gets no work
// planned (increments plan-work) and no request filed into it (requests file
// --increment); run by a person, its work is planned.
func TestR003NotRunIsNotPlanned(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	inc := increments(t, ctx)
	out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Invoices get paid",
		Goals: []string{"My customers pay me on time"}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	id := out.Increment.ID

	if _, err := inc.PlanWork(ctx, api.IncrementPlanWorkInput{ID: id, By: person}); err == nil || !strings.Contains(err.Error(), ruleR003) {
		t.Errorf("plan-work on %s, defined and not run: %v; want it refused, naming %s", id, err, ruleR003)
	}
	if _, err := req.File(ctx, api.FileInput{Title: "Mail the invoices", Module: "billing", Kind: api.KindBuild, Increment: id, By: person}); err == nil || !strings.Contains(err.Error(), ruleR003) {
		t.Errorf("a request filed into %s, defined and not run: %v; want it refused, naming %s", id, err, ruleR003)
	}
	if _, err := inc.Run(ctx, api.IncrementRunInput{ID: id, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.PlanWork(ctx, api.IncrementPlanWorkInput{ID: id, By: person}); err != nil && strings.Contains(err.Error(), ruleR003) {
		t.Errorf("plan-work on %s once run: %v; want it planned", id, err)
	}
}
