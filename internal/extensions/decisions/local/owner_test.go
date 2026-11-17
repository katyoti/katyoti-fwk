// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
)

// WHOSE A DECISION IS - a decision is
// the person's unless opened as an agent's, saying why; an agent takes an
// agent's with its reasons, traced on the decision, and the person may reopen
// it; the person's is refused to an agent.
func TestAnAgentsDecisionIsTakenTracedAndReopenable(t *testing.T) {
	ctx, cwm, svc := setup(t)
	agent := cwmapi.Actor{ID: "claude-a", Kind: cwmapi.ActorAgent}

	if _, err := svc.Open(ctx, api.OpenInput{Module: "m", Question: "Which cache?", Owner: api.OwnerAgent}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("an agent's decision opened without saying why: %v; want refused", err)
	}
	if _, err := svc.Open(ctx, api.OpenInput{Module: "m", Question: "Which cache?", Owner: "team"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("a decision opened with owner team: %v; want refused", err)
	}
	mine, err := svc.Open(ctx, api.OpenInput{Module: "m", Question: "Which cache?", Options: []string{"memory", "disk"},
		Owner: api.OwnerAgent, OwnerReason: "technical and reversible, inside the approved cache study"})
	if err != nil {
		t.Fatal(err)
	}
	if mine.Decision.Owner != api.OwnerAgent {
		t.Errorf("the agent's decision reads owner %q; want agent", mine.Decision.Owner)
	}
	theirs := open(t, ctx, svc, "Charge for exports?", "yes", "no")
	if theirs.Owner != api.OwnerPerson {
		t.Errorf("a decision opened with no owner reads %q; want the person's", theirs.Owner)
	}

	// The person's is the person's.
	if _, err := svc.Resolve(ctx, api.ResolveInput{ID: theirs.ID, Actor: agent, Selected: []string{"no"}, Rationale: "simpler"}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Errorf("an agent deciding the person's decision: %v; want refused", err)
	}
	// An agent's is taken by an agent, giving its reasons.
	if _, err := svc.Resolve(ctx, api.ResolveInput{ID: mine.Decision.ID, Actor: agent, Selected: []string{"memory"}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("an agent deciding without its reasons: %v; want refused", err)
	}
	got, err := svc.Resolve(ctx, api.ResolveInput{ID: mine.Decision.ID, Actor: agent, Selected: []string{"memory"}, Rationale: "It is enough for one process."})
	if err != nil {
		t.Fatal(err)
	}
	if got.Decision.Status != api.StatusApplies {
		t.Fatalf("the agent's decision taken: %s; want applies", got.Decision.Status)
	}
	obj, _ := stored(t, ctx, cwm, mine.Decision.ID)
	if !strings.Contains(obj.Body, "It is enough for one process.") || !strings.Contains(obj.Body, "Taken by the agent claude-a") || got.Decision.OwnerReason != "technical and reversible, inside the approved cache study" {
		t.Errorf("the decision does not trace the agent, its reasons and why it was the agent's: %q", obj.Body)
	}
	page, err := svc.Panel(ctx, api.PanelInput{ID: mine.Decision.ID})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(page.DSLTree); !strings.Contains(string(raw), "An agent"+`'`+"s decision") && !strings.Contains(string(raw), "An agent's decision") {
		t.Error("the decision's page does not show it is an agent's")
	}

	// The person may reopen it; an agent may not.
	if _, err := svc.Reopen(ctx, api.ReopenInput{ID: mine.Decision.ID, Actor: agent, Reason: "second thoughts"}); !errors.Is(err, cwmapi.ErrTypeRuleViolation) {
		t.Errorf("an agent reopening a decision: %v; want refused", err)
	}
	if _, err := svc.Reopen(ctx, api.ReopenInput{ID: mine.Decision.ID, Actor: who}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("a reopen saying no why: %v; want refused", err)
	}
	re, err := svc.Reopen(ctx, api.ReopenInput{ID: mine.Decision.ID, Actor: who, Reason: "the cache must survive a restart"})
	if err != nil {
		t.Fatal(err)
	}
	if re.Decision.Status != api.StatusOpen || len(re.Decision.Selected) != 0 || re.Decision.Resolved != "" {
		t.Errorf("reopened: %+v; want open with no choice", re.Decision)
	}
	obj, _ = stored(t, ctx, cwm, mine.Decision.ID)
	if !strings.Contains(obj.Body, "## Reopened") || !strings.Contains(obj.Body, "the cache must survive a restart") || !strings.Contains(obj.Body, "Taken by the agent claude-a") {
		t.Errorf("the reopened decision does not keep its history and the reason: %q", obj.Body)
	}
}
