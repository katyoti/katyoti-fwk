// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person/relaytest"
)

// TestAPersonsAnswerRelayedAndConfirmed: an agent's session answering for the
// person passes only relaying that person's answer, confirmed, with relaying
// on - resolved as the person, the entry recording the relay, the decision it
// is about decided too; relaying off it is refused and nothing moves.
func TestAPersonsAnswerRelayedAndConfirmed(t *testing.T) {
	ctx, cwm, svc, dec, _ := choiceWorld(t)
	decision, e := aChoice(t, ctx, svc, dec, "files", "sqlite")
	relayed := relaytest.Relaying(ctx, "claude-code", "jdoe", "answer:"+e.ID)

	relaytest.Set(t, false)
	if _, err := svc.Answer(relayed, api.AnswerInput{ID: e.ID, Choice: "files", By: "jdoe"}); !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "relaying the person's acts is off") {
		t.Fatalf("relayed with relaying off: %v; want refused", err)
	}
	if st := entryStatus(t, ctx, svc, e.ID); st != api.StatusPending {
		t.Fatalf("a refused relay moved the entry to %q", st)
	}
	relaytest.Set(t, true)
	out, err := svc.Answer(relayed, api.AnswerInput{ID: e.ID, Choice: "files", By: "jdoe"})
	if err != nil {
		t.Fatalf("relayed, confirmed, relaying on: %v", err)
	}
	r := out.Request.Relayed
	if r == nil || r.By != "claude-code" || r.Confirmed != "prompt" || out.Request.ResolvedBy != "jdoe" {
		t.Fatalf("entry: resolved by %q relayed %+v; want jdoe, relayed by claude-code at the prompt", out.Request.ResolvedBy, r)
	}
	if st, sel := decisionNow(t, ctx, cwm, decision); st != decisionsapi.StatusApplies || !slices.Equal(sel, []string{"files"}) {
		t.Fatalf("the decision is %s, selected %v; want it decided files", st, sel)
	}
}
