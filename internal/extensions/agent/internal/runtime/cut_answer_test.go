// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// : an answer cut at the output limit is continued at once with a
// higher limit (32 K, then 64 K); the count is of consecutive cut turns - a
// turn that is not cut resets it; and a run whose last answer stays cut ends
// failed, never completed.
func TestACutAnswerIsContinuedOrFails(t *testing.T) {
	hooks := NewOutputRecoveryPlugin().(*outputRecoveryPlugin).Hooks()
	lc := NewLoopContext()
	turn := func(stop string) (bool, int) {
		t.Helper()
		lc.SetMeta("_last_stop_reason", stop)
		d := &api.Decision{}
		if stop != "tool_use" { // a turn with tool calls reaches no decision
			if err := hooks.OnDecision(lc, d); err != nil {
				t.Fatal(err)
			}
		}
		_, maxTokens, _ := lc.ConsumeOverrides()
		if err := hooks.OnTurnEnd(lc, 0); err != nil {
			t.Fatal(err)
		}
		return d.Continue, maxTokens
	}
	if cont, limit := turn("max_tokens"); !cont || limit != 32768 {
		t.Fatalf("the first cut answer: continue %v at %d; want continued at 32768", cont, limit)
	}
	if cont, limit := turn("max_tokens"); !cont || limit != 65536 {
		t.Fatalf("the second cut answer in a row: continue %v at %d; want 65536", cont, limit)
	}
	turn("tool_use") // the work goes on: the count starts again
	for i, want := range []int{32768, 65536, 65536} {
		if cont, limit := turn("max_tokens"); !cont || limit != want {
			t.Fatalf("cut answer %d after a good turn: continue %v at %d; want %d", i+1, cont, limit, want)
		}
	}
	if cont, _ := turn("max_tokens"); cont {
		t.Fatal("a fourth cut answer in a row is continued; want the recovery spent")
	}

	// The run then ends failed, saying why; a text end that is not cut
	// completes as before.
	as := &agentState{Turn: 15, StartedAt: time.Now()}
	next, r := textTurnEnd(as, "max_tokens", "Je l'écrase maintenant avec le diagramme")
	if next != "failed" || r.Status != api.RunError || r.Error == nil || !strings.Contains(r.Error.Error(), "cut at its output limit") {
		t.Fatalf("a run ending on a cut answer: %s %+v", next, r)
	}
	if next, r := textTurnEnd(as, "end_turn", "Done."); next != "complete" || r.Status != api.RunCompleted || r.Summary != "Done." {
		t.Fatalf("a run ending on its text: %s %+v", next, r)
	}
}
