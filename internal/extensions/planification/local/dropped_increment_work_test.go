// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// DROPPING AN INCREMENT ENDS THE WORK OF WHAT IT TOOK:
// the features it took that no open scope holds have their plans ended
// dropped - no to-do of theirs is left for an agent to pull under the dropped
// increment's label -; a feature another open increment took keeps its work.
func TestDroppingAnIncrementEndsTheWorkOfWhatItTook(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	a := increment(t, ctx, "Invoices reach their customers by mail")
	b := increment(t, ctx, "Invoices reach their customers by post")
	taken := want(t, ctx, cwm, "feature", "Print the invoices")
	kept := want(t, ctx, cwm, "feature", "Post the invoices")
	for f, holder := range map[string]string{taken: a.ID, kept: b.ID} {
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: holder}); err != nil {
			t.Fatal(err)
		}
		file(t, ctx, req, api.FileInput{Title: "Build it", Kind: api.KindBuild, On: f})
		if len(openTodos(t, ctx, f)) == 0 {
			t.Fatalf("%s has no work planned on it", f)
		}
	}

	if _, err := inc.Drop(ctx, api.IncrementMoveInput{ID: a.ID, By: person, Note: "given up"}); err != nil {
		t.Fatal(err)
	}
	if open := openTodos(t, ctx, taken); len(open) > 0 {
		t.Fatalf("%s's increment is dropped but %d of its to-dos are still open (%s %s)", taken, len(open), open[0].ID, open[0].State)
	}
	if st := planState(t, ctx, taken); st != api.PlanDropped {
		t.Errorf("%s's increment is dropped but its plan is %s; want dropped", taken, st)
	}
	if len(openTodos(t, ctx, kept)) == 0 || planState(t, ctx, kept) == api.PlanDropped {
		t.Errorf("%s is held by the open %s but its work ended", kept, b.ID)
	}
}
