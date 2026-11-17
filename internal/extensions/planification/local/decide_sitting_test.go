// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"
)

// SEVERAL DECISIONS IN ONE SITTING - « À faire »'s Decide
// section draws every prepared decision of the person on the one page, each
// with its own choose form (decisions/resolve on that decision), so the person
// decides them one after the other without leaving it; an agent's is not drawn.
func TestThePersonDecidesSeveralDecisionsInOneSitting(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	a := openDecision(t, ctx, cwm, "Charge for exports?", true)
	b := openDecision(t, ctx, cwm, "Keep paper invoices?", true)
	page := queuePageOf(t, ctx, "decide")
	for _, id := range []string{a, b} {
		if !strings.Contains(page, `"id":"`+id+`"`) || !strings.Contains(page, `ktw-methods:decisions/resolve`) {
			t.Errorf("the Decide section does not let the person decide %s on the page", id)
		}
	}
	if n := strings.Count(page, `ktw-methods:decisions/resolve`); n < 4 {
		t.Errorf("%d choose forms on the Decide section; want one per option of each decision (4)", n)
	}
}
