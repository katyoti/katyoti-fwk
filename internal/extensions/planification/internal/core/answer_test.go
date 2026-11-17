// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A person's validation or challenge rejected says what to change: a result
// that does not — however it reaches the collector — is refused; approved, it
// needs nothing more than its verdict.
func TestARejectionSaysWhatToChange(t *testing.T) {
	for _, interaction := range []string{api.InteractionValidate, api.InteractionChallenge} {
		p := api.WorkPacket{Doer: api.DoerPerson, Interaction: interaction}
		if err := checkAnswer(p, schedapi.Result{Status: schedapi.StatusSuccess, Verdict: api.VerdictRejected, Summary: " \n "}); err == nil {
			t.Errorf("a %s rejected with an empty summary: accepted", interaction)
		}
		if err := checkAnswer(p, schedapi.Result{Status: schedapi.StatusSuccess, Verdict: api.VerdictRejected, Summary: "the totals are wrong"}); err != nil {
			t.Errorf("a %s rejected saying what to change: %v", interaction, err)
		}
		if err := checkAnswer(p, schedapi.Result{Status: schedapi.StatusSuccess, Verdict: api.VerdictApproved}); err != nil {
			t.Errorf("a %s approved: %v", interaction, err)
		}
	}
}

// A review planned before an agent's review carried its interaction
// is read as the validate its recipe step asks, as
// every step is: it closes the round before it, so a
// later rejection sends back only the work since, never the old review nor
// the round it already judged.
func TestAnOldReviewClosesItsRound(t *testing.T) {
	todo := func(id string, pos int, p api.WorkPacket) api.WorkTodo {
		w := api.WorkTodo{Packet: p}
		w.ID, w.Position = id, pos
		return w
	}
	done := []api.WorkTodo{
		todo("implement", 1, api.WorkPacket{Template: "build", Step: "implement", Phase: api.PhaseBuild, Doer: api.DoerPersona}),
		todo("old-review", 2, api.WorkPacket{Template: "build", Step: "review", Phase: api.PhaseVerify, Doer: api.DoerPersona}),
		todo("rework", 3, api.WorkPacket{Template: "build", Step: "implement (rework)", Phase: api.PhaseBuild, Doer: api.DoerPersona}),
	}
	v := todo("review", 4, api.WorkPacket{Template: "build", Step: "review (rework)", Phase: api.PhaseVerify, Doer: api.DoerPersona, Interaction: api.InteractionValidate})
	var got []string
	for _, d := range toRedo(done, v) {
		got = append(got, d.ID)
	}
	if len(got) != 1 || got[0] != "rework" {
		t.Fatalf("a rejection after a review planned before RQ-1080 sends back %v; want only [rework]", got)
	}
}

// A refusal names the interaction the step asks, however it was planned: a
// review planned before RQ-1080 says « a validate takes a verdict ».
func TestARefusalNamesTheAskedInteraction(t *testing.T) {
	old := api.WorkPacket{Template: "build", Step: "review", Doer: api.DoerPersona}
	err := checkAnswer(old, schedapi.Result{Status: schedapi.StatusSuccess, Summary: "ok"})
	if err == nil || !strings.Contains(err.Error(), "a validate takes a verdict") {
		t.Fatalf("an old review with no verdict: %v; want « a validate takes a verdict »", err)
	}
	err = checkAnswer(old, schedapi.Result{Status: schedapi.StatusSuccess, Verdict: api.VerdictRejected})
	if err == nil || !strings.Contains(err.Error(), "a validate rejected says what to change") {
		t.Fatalf("an old review rejected with no summary: %v; want « a validate rejected says what to change »", err)
	}
}
