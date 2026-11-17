// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A PERSON'S CONFIRM WAIVES THE GUI GATE, IT DOES NOT PASS IT
// : a realized screen feature, its unit criterion
// passed and no criterion running its GUI test, is refused at verify by
// gui-tested - the refusal files the request that writes and runs that test.
// The person confirms the feature done (R-006's waiver, « deliver then
// test »): the gate was not passed, so that request stays open - never
// dropped as « the gate it resolved was passed ».
func TestAPersonsConfirmKeepsTheGUITestTheGateAsked(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "The invoice pane shows its lines", cwmapi.ObjectEdge{Kind: "realizes", To: "app/ktw-gui/frontend/src/sections/InvoiceLines.tsx"})
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "built"}); err != nil {
		t.Fatal(err)
	}
	passVerdict(t, ctx, cwm, "billing", criterionFor(t, ctx, cwm, "billing", f, "go test ./internal/extensions/billing/..."))
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{f: "pass"}})

	guiTest := func() (api.Request, bool) {
		t.Helper()
		l, err := req.List(ctx, api.ListInput{Module: "billing"})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range l.Requests {
			if strings.Contains(r.Title, "Write and run the GUI test") {
				return r, true
			}
		}
		return api.Request{}, false
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "done", Transition: "verify", By: session, Note: "its criteria pass"}); err == nil {
		t.Fatalf("%s moved done by an agent with no GUI test", f)
	}
	var r api.Request
	ok := false
	for i := 0; i < 100 && !ok; i++ {
		if r, ok = guiTest(); !ok {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if !ok {
		t.Fatalf("the gui-tested refusal of %s filed no request writing its GUI test", f)
	}

	if err := move(ctx, cwm, f, api.StateDone, person); err != nil {
		t.Fatalf("the person's confirm of %s: %v", f, err)
	}
	// Its transitioned event ends the gate's wait, shortly after.
	time.Sleep(500 * time.Millisecond)
	if st := planState(t, ctx, r.ID); st == api.PlanDropped {
		t.Errorf("the person confirmed %s past its GUI gate: its GUI-test request %s was dropped; want it kept open", f, r.ID)
	}
}

// THE WAIVER HOLDS WHEN THE WAIT OUTLIVES THE MOVE:
// GateMoved did not end the wait - a remote gater, an error only logged - so
// the next GateProgress finds the feature already done, still waiting. It
// reads the move from the journal: a person's confirm, another transition
// than the refused verify, is a waiver there too - the GUI-test request stays
// open.
func TestAWaivedGateStaysWaivedWhenTheWaitOutlivesTheMove(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "The invoice pane shows its lines", cwmapi.ObjectEdge{Kind: "realizes", To: "app/ktw-gui/frontend/src/sections/InvoiceLines.tsx"})
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "built"}); err != nil {
		t.Fatal(err)
	}
	passVerdict(t, ctx, cwm, "billing", criterionFor(t, ctx, cwm, "billing", f, "go test ./internal/extensions/billing/..."))
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{f: "pass"}})
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "done", Transition: "verify", By: session, Note: "its criteria pass"}); err == nil {
		t.Fatalf("%s moved done by an agent with no GUI test", f)
	}
	var r api.Request
	var w api.GateWaitFields
	found := false
	for i := 0; i < 100 && !found; i++ {
		l, err := req.List(ctx, api.ListInput{Module: "billing"})
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range l.Requests {
			if strings.Contains(x.Title, "Write and run the GUI test") {
				r, found = x, true
			}
		}
		if !found {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if !found {
		t.Fatalf("the gui-tested refusal of %s filed no request writing its GUI test", f)
	}
	w, waiting := waitOf(t, ctx, cwm, f)
	if !waiting {
		t.Fatalf("%s refused at its gate carries no wait", f)
	}

	if err := move(ctx, cwm, f, api.StateDone, person); err != nil {
		t.Fatalf("the person's confirm of %s: %v", f, err)
	}
	time.Sleep(500 * time.Millisecond)
	// The wait as GateMoved would have left it, had it not run.
	if _, still := waitOf(t, ctx, cwm, f); !still {
		raw, _ := json.Marshal(w)
		var fields map[string]any
		_ = json.Unmarshal(raw, &fields)
		if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: f, Capability: api.CapGateWait, Fields: fields}); err != nil {
			t.Fatal(err)
		}
	}
	g, ok := req.(interface {
		GateProgress(ctx context.Context, id string) error
	})
	if !ok {
		t.Fatal("the in-process request service follows no gate")
	}
	if err := g.GateProgress(ctx, f); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st == api.PlanDropped {
		t.Errorf("GateProgress after the person's confirm of %s dropped its GUI-test request %s; want it kept open", f, r.ID)
	}
	if _, still := waitOf(t, ctx, cwm, f); still {
		t.Errorf("GateProgress left %s waiting after its move", f)
	}
}
