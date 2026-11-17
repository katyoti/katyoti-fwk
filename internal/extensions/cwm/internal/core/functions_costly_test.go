// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A PREVIEW NEVER RUNS A COSTLY FUNCTION. heavy() is declared costly (and its
// action writes, which a costly function may): an evaluation with NoCostly,
// and a dry run with NoCostly, do not run it — the call answers
// ErrDecidedAtMove, marked AtMove — while the other functions run, told
// NoCostly. The move itself, and an evaluation that is not a preview, run it.

func TestAPreviewEvaluationDoesNotRunACostlyFunction(t *testing.T) {
	acts := newFnActions()
	svc, ctx := fnFixture(t, "true", acts)
	out, err := svc.ObjectEvaluate(ctx, api.ObjectEvaluateInput{ID: "m.t", Expressions: []string{"heavy()", "score() == 3"}, NoCostly: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := acts.count("x:heavy/get"); n != 0 {
		t.Errorf("a preview ran the costly heavy() %d times", n)
	}
	r := out.Results[0]
	if r.Value != nil || !strings.Contains(r.Error, api.ErrDecidedAtMove.Error()) || len(r.Calls) != 1 || !r.Calls[0].AtMove {
		t.Errorf("heavy() in a preview: %+v — an error, decided at the move", r)
	}
	if out.Results[1].Value != true || acts.count("x:score/get") != 1 {
		t.Errorf("the cheap score() still runs in a preview: %+v", out.Results[1])
	}
	if len(acts.seen) != 1 || !acts.seen[0].NoCostly {
		t.Errorf("a function called by a preview is told so: %+v", acts.seen)
	}

	out, err = svc.ObjectEvaluate(ctx, api.ObjectEvaluateInput{ID: "m.t", Expressions: []string{"heavy()"}})
	if err != nil || out.Results[0].Value != true || acts.count("x:heavy/get") != 1 {
		t.Errorf("outside a preview heavy() runs, its writing action allowed: %+v, %v", out, err)
	}
}

func TestAPreviewDryRunLeavesACostlyGuardToTheMove(t *testing.T) {
	acts := newFnActions()
	svc, ctx := fnFixture(t, "heavy()", acts)
	ana := api.Actor{ID: "ana", Kind: "person"}
	_, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.t", To: "accepted", By: ana, DryRun: true, NoCostly: true})
	var refused *api.TransitionRefusedError
	if !errors.As(err, &refused) || !api.AnyAtMove(refused.Calls) {
		t.Fatalf("a previewed costly guard: %v — undecided, its call AtMove", err)
	}
	if n := acts.count("x:heavy/get"); n != 0 {
		t.Errorf("the preview ran heavy() %d times", n)
	}
	if _, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.t", To: "accepted", By: ana, NoCostly: true}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("no_costly on a move: %v — it goes with dry_run", err)
	}
	if err := fnMove(svc, ctx); err != nil {
		t.Fatalf("the move runs heavy(), which holds: %v", err)
	}
	if n := acts.count("x:heavy/get"); n != 1 {
		t.Errorf("the move ran heavy() %d times, want once", n)
	}
}
