// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A DROPPED TARGET COUNTS IN NEITHER DONE NOR TOTAL - an increment holding a
// use case done and a fix that ended dropped is all done (1/1), never 1/2,
// and does not wait on the dropped one; once done, the requests still open
// that serve what it holds are follow-up work, never counted as its own
// (the IN-018 « done 37/38 »).
func TestADroppedTargetCountsInNeitherDoneNorTotal(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc, plans := increments(t, ctx), plansOf(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	uc := usecase(t, ctx, cwm, "Send invoices by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: uc, To: "done", By: person, Note: "mailed"}); err != nil {
		t.Fatal(err)
	}
	fix := file(t, ctx, req, api.FileInput{Title: "The mail lacks its attachment", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	if _, err := plans.Finish(ctx, api.FinishInput{ID: fix.ID, Outcome: "dropped", By: person, Note: "not needed"}); err != nil {
		t.Fatal(err)
	}

	sc, err := inc.Scope(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if sc.Done != 1 || sc.Total != 1 {
		t.Errorf("scope %d/%d; a dropped target counts in neither: want 1/1", sc.Done, sc.Total)
	}
	waits, err := inc.Waits(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(waits.Waits, "\n"); strings.Contains(got, "The mail lacks its attachment") {
		t.Errorf("the increment waits on its dropped fix: %q", got)
	}

	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatal(err)
	}
	later := file(t, ctx, req, api.FileInput{Title: "Mail the invoices in batches", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Assess(ctx, api.AssessInput{ID: later.ID, Serves: uc, By: intake}); err != nil {
		t.Fatal(err)
	}
	sum, err := inc.Summary(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Done != 1 || sum.Total != 1 || sum.Running+sum.ToDo+sum.Awaiting+sum.Blocked != 0 {
		t.Errorf("the done increment's summary: %+v; want 1/1 and no open request counted as its work", sum)
	}
}
