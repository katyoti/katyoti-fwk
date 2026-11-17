// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

type sent struct {
	subject string
	event   Event
}

func recorder(t *testing.T) (Publish, func() []sent) {
	var mu sync.Mutex
	var got []sent
	return func(_ context.Context, subject string, payload []byte) error {
			e, err := DecodeEvent(payload)
			if err != nil {
				t.Errorf("an invalid event went out: %v (%s)", err, payload)
			}
			mu.Lock()
			got = append(got, sent{subject, e})
			mu.Unlock()
			return nil
		}, func() []sent {
			mu.Lock()
			defer mu.Unlock()
			return append([]sent(nil), got...)
		}
}

var scope = Scope{Tenant: "t", Universe: "u"}

// A panel id is dp- and a time-ordered UUID: a valid subject token, fresh
// each time.
func TestAPanelIDIsFreshAndASubjectToken(t *testing.T) {
	a, b := NewID(), NewID()
	if a == b || !ValidID(a) || !strings.HasPrefix(a, "dp-") {
		t.Fatalf("ids %q %q", a, b)
	}
	_ = subjects.PanelOutSubject("t", "u", a) // panics on an invalid token
	for _, bad := range []string{"", "dp-", "x-0192", "dp-not-a-uuid", "DP-" + a[3:]} {
		if ValidID(bad) {
			t.Errorf("%q taken for a panel id", bad)
		}
	}
}

// A task opens a panel and drives it: each event goes out on the panel's
// out subject, numbered after the last, its ops carrying the event's
// sequence, its state following - asking after an ask, complete at the end -
// and nothing goes out once it is complete but its close.
func TestAHandleSendsThePanelsEventsInOrder(t *testing.T) {
	ctx := context.Background()
	pub, got := recorder(t)
	tree := render.InlineSpec(render.VBox(render.Text("…").AsSection("pages", render.Load{Method: "ktw-x:pub/pages"}, render.Placeholder{Kind: render.PlaceholderRows})))
	h, err := OpenPanel(ctx, scope, pub, Open{Type: "ktw-example:/panel-type/confluence-publish", Title: "Publication", Owner: "run-1", Answer: render.Answer{Tree: &tree}})
	if err != nil {
		t.Fatal(err)
	}
	steps := []func() error{
		func() error { return h.Progress(ctx, "Pages", 1, 3) },
		func() error { return h.Patch(ctx, render.Replace(0, "pages", render.Text("1/3"))) },
		func() error {
			return h.Ask(ctx, Ask{Question: "Overwrite?", Actions: []AskAction{{Name: "yes"}, {Name: "no"}}})
		},
	}
	for _, s := range steps {
		if err := s(); err != nil {
			t.Fatal(err)
		}
	}
	if h.State() != StateAsking {
		t.Fatalf("after an ask the panel is %s", h.State())
	}
	h.Answered()
	if err := h.Complete(ctx, "ok", map[string]any{"pages": 3}); err != nil {
		t.Fatal(err)
	}
	if err := h.Progress(ctx, "late", 3, 3); err == nil {
		t.Fatal("a progress went out after the panel completed")
	}
	if err := h.Close(ctx); err != nil {
		t.Fatalf("close after complete: %v", err)
	}
	if err := h.Close(ctx); err == nil {
		t.Fatal("a closed panel sent again")
	}
	evs := got()
	kinds := []Kind{KindOpen, KindProgress, KindOps, KindAsk, KindComplete, KindClose}
	if len(evs) != len(kinds) {
		t.Fatalf("%d events out, want %d", len(evs), len(kinds))
	}
	out := subjects.PanelOutSubject("t", "u", h.ID())
	for i, s := range evs {
		if s.subject != out || s.event.Seq != int64(i+1) || s.event.Kind != kinds[i] || s.event.Panel != h.ID() {
			t.Fatalf("event %d: %s seq %d %s, want %s seq %d %s", i, s.subject, s.event.Seq, s.event.Kind, out, i+1, kinds[i])
		}
	}
	if op := evs[2].event.Ops[0]; op.Seq != 3 || op.Target != "pages" {
		t.Fatalf("the op carries %+v, want the event's seq 3", op)
	}
	if h.InputSubject() != subjects.PanelInSubject("t", "u", h.ID()) {
		t.Fatal("the input subject is not the panel's in subject")
	}
	if h.State() != StateClosed || h.Seq() != 6 {
		t.Fatalf("state %s seq %d", h.State(), h.Seq())
	}
}

// What the handle refuses to send, and a publish that fails, leave the
// panel's sequence as it was: the next event fills no hole.
func TestARefusedOrFailedEventLeavesNoHole(t *testing.T) {
	ctx := context.Background()
	fail := true
	var seqs []int64
	pub := func(_ context.Context, _ string, payload []byte) error {
		if fail {
			return errors.New("bus down")
		}
		var e Event
		_ = json.Unmarshal(payload, &e)
		seqs = append(seqs, e.Seq)
		return nil
	}
	if _, err := OpenPanel(ctx, scope, pub, Open{Type: "x"}); err == nil {
		t.Fatal("an open that could not go out answered a handle")
	}
	fail = false
	h, err := OpenPanel(ctx, scope, pub, Open{Type: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Patch(ctx); err == nil {
		t.Fatal("ops without an op went out")
	}
	if err := h.Patch(ctx, render.Op{Op: "rfc6902", Target: "a"}); err == nil {
		t.Fatal("an op off the op set went out")
	}
	if err := h.Ask(ctx, Ask{}); err == nil {
		t.Fatal("an ask asking nothing went out")
	}
	fail = true
	if err := h.Progress(ctx, "x", 1, 2); err == nil {
		t.Fatal("a failed publish answered nil")
	}
	fail = false
	if err := h.Progress(ctx, "x", 1, 2); err != nil {
		t.Fatal(err)
	}
	if len(seqs) != 2 || seqs[0] != 1 || seqs[1] != 2 {
		t.Fatalf("sequences out %v, want [1 2]", seqs)
	}
}

func TestAnEventIsOneOfItsKind(t *testing.T) {
	id := NewID()
	bad := map[string]Event{
		"no panel id":          {Panel: "x", Seq: 1, Kind: KindOpen, Open: &Open{Type: "t"}},
		"seq 0":                {Panel: id, Seq: 0, Kind: KindClose},
		"open later":           {Panel: id, Seq: 2, Kind: KindOpen, Open: &Open{Type: "t"}},
		"first is not an open": {Panel: id, Seq: 1, Kind: KindProgress, Progress: &Progress{}},
		"open with nothing":    {Panel: id, Seq: 1, Kind: KindOpen, Open: &Open{}},
		"complete no status":   {Panel: id, Seq: 2, Kind: KindComplete, Complete: &Complete{}},
		"unknown":              {Panel: id, Seq: 2, Kind: "patch"},
	}
	for name, e := range bad {
		if e.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	tree := render.InlineSpec(render.Text("x"))
	if err := (Event{Panel: id, Seq: 1, Kind: KindOpen, Open: &Open{Answer: render.Answer{Tree: &tree}}}).Validate(); err != nil {
		t.Errorf("an open with an inline answer: %v", err)
	}
}

// The person's input comes back as one of four kinds.
func TestTheInputComesBackAsOneOfFourKinds(t *testing.T) {
	id := NewID()
	ok := []Input{
		{Panel: id, Kind: InputSubmit, Value: map[string]any{"overwrite": true}},
		{Panel: id, Kind: InputAction, Name: "yes"},
		{Panel: id, Kind: InputMessage, Text: "go on"},
		{Panel: id, Kind: InputCancel},
	}
	for _, in := range ok {
		raw, _ := json.Marshal(in)
		if _, err := DecodeInput(raw); err != nil {
			t.Errorf("%s: %v", in.Kind, err)
		}
	}
	for _, in := range []Input{{Panel: id, Kind: InputAction}, {Panel: id, Kind: InputMessage}, {Panel: id, Kind: "drop"}, {Panel: "x", Kind: InputCancel}} {
		if in.Validate() == nil {
			t.Errorf("%+v accepted", in)
		}
	}
}
