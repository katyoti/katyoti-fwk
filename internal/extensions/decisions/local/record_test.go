// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	decisionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/local"
	occontrib "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// RECORD: a decision taken before it was written here keeps its original
// dates, is answered, and says who recorded it.
func TestRecordKeepsTheOriginalDates(t *testing.T) {
	ctx, cwm, svc := setup(t)
	out, err := svc.Record(ctx, api.RecordInput{
		Module: "m", Question: "Which message bus?", Context: "Eleven messaging features are needed.",
		Options: []string{"nats", "hand-rolled"}, Selected: []string{"nats"}, Rationale: "Every feature is native.",
		Consequences: "Only the transport imports the client.", Raised: "2026-05-07", Resolved: "2026-05-09",
		Source: "docs/decisions/nats-as-message-bus.md", Actor: who,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	d := out.Decision
	if d.Status != api.StatusApplies || d.Raised != "2026-05-07" || d.Resolved != "2026-05-09" || d.Source != "docs/decisions/nats-as-message-bus.md" {
		t.Fatalf("recorded decision = %+v, want answered with its original dates and source", d)
	}
	obj, _ := stored(t, ctx, cwm, d.ID)
	for _, want := range []string{"## Question\n\nEleven", "## Decision\n\nnats. Every feature is native.", "## Consequences\n\nOnly the transport"} {
		if !strings.Contains(obj.Body, want) {
			t.Errorf("body lacks %q:\n%s", want, obj.Body)
		}
	}
	life, _ := obj.Lifecycle()
	if life.State != api.StatusApplies {
		t.Fatalf("lifecycle = %+v, want moved to %s (the move itself is the journal's)", life, api.StatusApplies)
	}
	// Resolved defaults to raised.
	out, err = svc.Record(ctx, api.RecordInput{Module: "m", Question: "Q2", Selected: []string{"x"}, Raised: "2026-01-02", Actor: who})
	if err != nil || out.Decision.Resolved != "2026-01-02" {
		t.Fatalf("Record without resolved: %v %+v", err, out.Decision)
	}
}

func TestRecordRefusesWhatOpenAndResolveRefuse(t *testing.T) {
	ctx, _, svc := setup(t)
	base := api.RecordInput{Module: "m", Question: "Q", Selected: []string{"a"}, Raised: "2026-05-07", Actor: who}
	for name, mut := range map[string]func(*api.RecordInput){
		"no question":         func(in *api.RecordInput) { in.Question = " " },
		"no choice":           func(in *api.RecordInput) { in.Selected = nil },
		"no actor":            func(in *api.RecordInput) { in.Actor = cwmapi.Actor{} },
		"a choice not listed": func(in *api.RecordInput) { in.Options = []string{"b"} },
		"no raised date":      func(in *api.RecordInput) { in.Raised = "" },
		"not a date":          func(in *api.RecordInput) { in.Raised = "May 7" },
		"answered before":     func(in *api.RecordInput) { in.Resolved = "2026-05-06" },
		"answered in future":  func(in *api.RecordInput) { in.Resolved = "2999-01-01" },
	} {
		in := base
		mut(&in)
		if _, err := svc.Record(ctx, in); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}

// JUSTIFIED-BY, the shipped edge: a rule cites the decision that says why it
// holds; a target without the decision capability is refused.
func TestARuleIsJustifiedByADecisionOnly(t *testing.T) {
	// Strict, as production: the edge kind's target capability is a refusal.
	ctx, cwm := cwmapp.NewStrict(t, cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":     {Data: []byte("name: module\n")},
		"object-types/decision.yaml":   {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/object-types/decision.yaml")},
		"capabilities/decision.yaml":   {Data: []byte("name: decision\n")},
		"capabilities/lifecycle.yaml":  {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/capabilities/lifecycle.yaml")},
		"workflows/decision.yaml":      {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/workflows/decision.yaml")},
		"object-types/rule.yaml":       {Data: cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/object-types/rule.yaml")},
		"edge-types/justified-by.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/edge-types/justified-by.yaml")},
	}), occontrib.Pack(""), decisionslocal.Extension())
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.DecisionsService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.Record(ctx, api.RecordInput{Module: "m", Question: "Which bus?", Selected: []string{"nats"}, Raised: "2026-05-09", Actor: who})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(id, title string) {
		o := cwmapi.Object{ID: id, Type: "rule", Parent: "m", Name: title, Fields: map[string]any{"statement": title}}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("m.R-001", "MessageBus only")
	mk("m.R-002", "Builders only")
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: "m.R-001", To: d.Decision.ID, Kind: "justified-by"}); err != nil {
		t.Fatalf("a rule justified by a decision: %v", err)
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: "m.R-002", To: "m.R-001", Kind: "justified-by"}); err == nil {
		t.Fatal("a rule justified by a rule was accepted; only a decision justifies")
	}
	in, _ := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: d.Decision.ID, Kind: "justified-by", Direction: cwmapi.DirectionIn})
	if len(in.Edges) != 1 || in.Edges[0].From != "m.R-001" {
		t.Fatalf("justified-by edges into the decision: %+v", in.Edges)
	}
}
