// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// DROPPING A GOAL OR A FEATURE ENDS ITS PLAN: a
// feature with work planned on it moved dropped leaves no open to-do - none
// for an agent to pull, none counted for its increment or release - and its
// plan ends dropped saying why. A feature kept keeps its work.
func TestDroppingAFeatureEndsItsPlan(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	dropped := want(t, ctx, cwm, "feature", "Print the invoices")
	kept := want(t, ctx, cwm, "feature", "Mail the invoices")
	for _, f := range []string{dropped, kept} {
		file(t, ctx, req, api.FileInput{Title: "Build it", Kind: api.KindBuild, On: f})
		if len(openTodos(t, ctx, f)) == 0 {
			t.Fatalf("%s has no work planned on it", f)
		}
	}

	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: dropped, To: api.StateDropped, By: person, Note: "superseded"}); err != nil {
		t.Fatal(err)
	}
	// Its transitioned event ends its plan, shortly after.
	for i := 0; i < 250 && len(openTodos(t, ctx, dropped)) > 0; i++ {
		time.Sleep(40 * time.Millisecond)
	}
	if open := openTodos(t, ctx, dropped); len(open) > 0 {
		t.Fatalf("%s is dropped but %d of its to-dos are still open (%s %s)", dropped, len(open), open[0].ID, open[0].State)
	}
	if st := planState(t, ctx, dropped); st != api.PlanDropped {
		t.Errorf("%s is dropped but its plan is %s; want dropped", dropped, st)
	}
	if len(openTodos(t, ctx, kept)) == 0 || planState(t, ctx, kept) == api.PlanDropped {
		t.Errorf("%s was not dropped but its work ended", kept)
	}
}
