// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// : a repair applies only to an element still as the sweep
// read it - one that moved between the read and the repair is skipped, its
// repair never run, and counts nothing toward flapping.

func settleFinding(still bool, ran *int) finding {
	return finding{
		repair: api.Repair{Repair: "put-step", Work: "ops.RQ-900", Reason: "a step is missing"},
		key:    "put-step:ops.RQ-900",
		still:  func(context.Context) bool { return still },
		apply: func(context.Context) (int, error) {
			*ran++
			return 1, nil
		},
	}
}

func TestReconcileSkipsAnElementThatMovedSinceTheSweepReadIt(t *testing.T) {
	cfg := api.ReconcilerConfig{Repairs: []string{"put-step"}}
	in := api.ReconcileInput{Apply: true}

	st := &reconState{applied: map[string]int{}}
	ran := 0
	out := st.settle(context.Background(), in, cfg, []finding{settleFinding(false, &ran)}, nil)
	if len(out.Repairs) != 1 {
		t.Fatalf("repairs = %+v, want the one finding", out.Repairs)
	}
	r := out.Repairs[0]
	if r.Applied || r.Skipped != "it moved since the sweep read it" {
		t.Fatalf("moved element: applied=%v skipped=%q, want skipped as moved", r.Applied, r.Skipped)
	}
	if ran != 0 || st.applied["put-step:ops.RQ-900"] != 0 || st.puts != 0 {
		t.Fatalf("moved element: apply ran %d times, applied %d, puts %d - want none", ran, st.applied["put-step:ops.RQ-900"], st.puts)
	}

	// The same finding, unmoved, is applied: the skip is the observation's, not the repair's.
	st = &reconState{applied: map[string]int{}}
	out = st.settle(context.Background(), in, cfg, []finding{settleFinding(true, &ran)}, nil)
	if r := out.Repairs[0]; !r.Applied || r.Skipped != "" || ran != 1 || st.puts != 1 {
		t.Fatalf("unmoved element: applied=%v skipped=%q ran=%d puts=%d, want applied once", r.Applied, r.Skipped, ran, st.puts)
	}
}
