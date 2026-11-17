// SPDX-License-Identifier: Apache-2.0

package panel

import "testing"

// A track numbers a machine's panel events and follows the panel's state as
// plain data a checkpoint keeps.
func TestATrackNumbersThePanelsEventsAcrossSteps(t *testing.T) {
	tr := NewTrack("")
	if !ValidID(tr.ID) {
		t.Fatalf("id %q", tr.ID)
	}
	if _, err := tr.Next(Event{Kind: KindProgress, Progress: &Progress{Current: 1}}); err == nil {
		t.Fatal("an event before the open was accepted")
	}
	steps := []Event{
		{Kind: KindOpen, Open: &Open{Type: "x"}},
		{Kind: KindAsk, Ask: &Ask{Question: "go?"}},
	}
	for i, e := range steps {
		got, err := tr.Next(e)
		if err != nil || got.Seq != int64(i+1) || got.Panel != tr.ID {
			t.Fatalf("step %d: %+v %v", i, got, err)
		}
	}
	if tr.State != StateAsking {
		t.Fatalf("state %s after an ask", tr.State)
	}
	tr.Answered()
	// What a checkpoint keeps carries the numbering on.
	again := Track{ID: tr.ID, Seq: tr.Seq, State: tr.State}
	if e, err := again.Next(Event{Kind: KindComplete, Complete: &Complete{Status: "ok"}}); err != nil || e.Seq != 3 {
		t.Fatalf("after a restart: %+v %v", e, err)
	}
	if _, err := again.Next(Event{Kind: KindProgress, Progress: &Progress{}}); err == nil {
		t.Fatal("a progress after complete was accepted")
	}
}
