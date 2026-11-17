// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person/relaytest"
)

// TestAPersonsDecisionRelayedAndConfirmed: an agent's session deciding for the
// person passes only relaying that person's decision, confirmed, with relaying
// on - and the decision records who relayed it and how it was confirmed;
// with relaying off it is refused and nothing is decided.
func TestAPersonsDecisionRelayedAndConfirmed(t *testing.T) {
	ctx, _, svc := setup(t)
	d := open(t, ctx, svc, "Which store?", "files", "sqlite")
	relayed := relaytest.Relaying(ctx, "claude-code", who.ID, "decide:"+d.ID)

	relaytest.Set(t, false)
	if _, err := svc.Resolve(relayed, api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"files"}}); !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "relaying the person's acts is off") {
		t.Fatalf("relayed with relaying off: %v; want refused", err)
	}
	relaytest.Set(t, true)
	if _, err := svc.Resolve(relaytest.Relaying(ctx, "claude-code", "someone-else", "decide:"+d.ID), api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"files"}}); err == nil {
		t.Fatal("another person's relayed act decided it; want refused")
	}
	out, err := svc.Resolve(relayed, api.ResolveInput{ID: d.ID, Actor: who, Selected: []string{"files"}, Rationale: "simplest"})
	if err != nil {
		t.Fatalf("relayed, confirmed, relaying on: %v", err)
	}
	r := out.Decision.Relayed
	if r == nil || r.By != "claude-code" || r.Confirmed != "prompt" || out.Decision.Status != api.StatusApplies {
		t.Fatalf("decision: %+v relayed %+v; want applies, relayed by claude-code confirmed at the prompt", out.Decision, r)
	}
}
