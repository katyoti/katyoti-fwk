// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person/relaytest"
)

// The person's acts in planification - verifying or testing by hand, holding,
// waiving - from an agent's session pass only relaying that person's act,
// confirmed, with relaying on, and each records the relay; relaying off they
// are refused and nothing changes.
func TestThePersonsActsRelayedAndConfirmed(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	as := func(act string) context.Context { return relaytest.Relaying(ctx, "claude-code", person.ID, act) }
	isRelay := func(what string, r *api.Relayed) {
		t.Helper()
		if r == nil || r.By != "claude-code" || r.Confirmed != "prompt" {
			t.Fatalf("%s: relayed %+v; want claude-code at the prompt", what, r)
		}
	}

	// Hold, then a hand verification.
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen on paper", ByHand: "a person checks it; no agent can"}}})
	toReview(t, ctx, r.ID)
	key := personQueue(t, ctx, r.ID).Items[0].Key
	relaytest.Set(t, false)
	if _, err := req.Hold(as("hold:"+key), api.HoldInput{Key: key, By: person}); err == nil || !strings.Contains(err.Error(), "relaying the person's acts is off") {
		t.Fatalf("hold relayed with relaying off: %v; want refused", err)
	}
	verify := api.VerifyInput{ID: r.ID, Deliverable: "seen on paper", Evidence: "printed and read", By: person}
	if _, err := req.Verify(as("verify:"+r.ID+"#seen on paper"), verify); err == nil {
		t.Fatal("verified relayed with relaying off")
	}
	relaytest.Set(t, true)
	// A confirmation covers its own act only.
	if _, err := req.Hold(as("verify:"+r.ID+"#seen on paper"), api.HoldInput{Key: key, By: person}); err == nil || !strings.Contains(err.Error(), "the confirmation is for") {
		t.Fatalf("hold with another act's confirmation: %v; want refused", err)
	}
	h, err := req.Hold(as("hold:"+key), api.HoldInput{Key: key, Reason: "later", By: person})
	if err != nil {
		t.Fatalf("hold relayed: %v", err)
	}
	isRelay("hold", h.Hold.Relayed)
	if _, err := req.Unhold(as("unhold:"+key), api.UnholdInput{Key: key, By: person}); err != nil {
		t.Fatalf("unhold relayed: %v", err)
	}
	if _, err := req.Verify(as("verify:"+r.ID+"#seen on paper"), verify); err != nil {
		t.Fatalf("verify relayed: %v", err)
	}
	d := deliverableOf(t, ctx, req, r.ID, "seen on paper")
	if !d.Verified {
		t.Fatalf("not verified: %+v", d)
	}
	isRelay("verify", d.Relayed)

	// A passing hand test verifies its deliverable as part of itself.
	r2 := file(t, ctx, req, api.FileInput{Title: "Mail invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "received", ByHand: "a person reads their mail; no agent can"}}})
	toReview(t, ctx, r2.ID)
	if _, err := req.Test(as("test:"+r2.ID+"#received"), api.TestInput{ID: r2.ID, Deliverable: "received", Result: api.TestPasses, Note: "in my inbox", By: person}); err != nil {
		t.Fatalf("a relayed passing hand test: %v", err)
	}
	if d := deliverableOf(t, ctx, req, r2.ID, "received"); !d.Verified || d.Relayed == nil {
		t.Fatalf("after a relayed passing hand test: %+v; want verified, relayed", d)
	}

	// A waiver.
	w := file(t, ctx, req, api.FileInput{Title: "Rename a label", Module: "billing", Kind: api.KindBuild})
	out, err := req.Waive(as("waive:"+w.ID), api.WaiveInput{ID: w.ID, Note: "a label renamed, nothing to prove", By: person})
	if err != nil || out.Request.Waiver == nil {
		t.Fatalf("waive relayed: %+v, %v", out.Request.Waiver, err)
	}
	isRelay("waive", out.Request.Waiver.Relayed)
}
