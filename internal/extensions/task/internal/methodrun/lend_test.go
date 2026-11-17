// SPDX-License-Identifier: Apache-2.0

package methodrun

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// assertNumberedThrough: the panel's events are numbered 1..n with no gap
// and no duplicate, the open first and the complete last, and the kinds in
// between are want.
func assertNumberedThrough(t *testing.T, evs []panel.Event, want ...panel.Kind) {
	t.Helper()
	sort.Slice(evs, func(i, j int) bool { return evs[i].Seq < evs[j].Seq })
	kinds := []panel.Kind{panel.KindOpen}
	kinds = append(kinds, want...)
	kinds = append(kinds, panel.KindComplete)
	if len(evs) != len(kinds) {
		t.Fatalf("the panel got %d events, want %d (%v): %+v", len(evs), len(kinds), kinds, evs)
	}
	for i, e := range evs {
		if e.Seq != int64(i+1) {
			t.Fatalf("event %d is numbered %d: a gap or a duplicate (%+v)", i+1, e.Seq, evs)
		}
		if e.Kind != kinds[i] {
			t.Fatalf("event %d is a %s, want a %s (%+v)", i+1, e.Kind, kinds[i], evs)
		}
	}
}

// A method called in process reports its progress into its run's panel with
// its own progress.Emit; the run's complete follows its last report.
func TestAnInProcessMethodReportsIntoItsRunsPanel(t *testing.T) {
	acts := &fakeActions{run: func(ctx context.Context) (json.RawMessage, error) {
		progress.Emit(ctx, panel.Progress{Label: "items", Current: 1, Total: 2})
		progress.Emit(ctx, panel.Progress{Label: "items", Current: 2, Total: 2})
		return json.RawMessage(`{"ok":true}`), nil
	}}
	_, events := rig(t, acts)
	if _, err := runIt(t, context.Background(), taskapi.MethodRunDef("ktw-univers:marketplace/install", panel.NewID(), "Install", "")["config"].(map[string]any), nil); err != nil {
		t.Fatal(err)
	}
	assertNumberedThrough(t, waitEvents(t, events, 4), panel.KindProgress, panel.KindProgress)
}

// sidecarEndpoint serves marketplace/install as a sidecar would: it reports
// with progress.Emit and knows nothing of panels; fail makes it fail after
// one report. complete is the error a lent handle gave when the method tried
// to complete the panel itself.
func sidecarEndpoint(fail bool, complete *error) *ktsp.Endpoint {
	ep := ktsp.NewEndpoint("ktw-univers", "", "")
	slot := ktsp.NewSlot("marketplace", ktsp.DefaultInstance, "", nil)
	slot.Add(ktsp.RawMethod("install", ktsp.MethodOpts{},
		func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			progress.Emit(ctx, panel.Progress{Label: "download", Current: 1, Total: 2})
			if fail {
				return nil, errors.New("sha256 mismatch")
			}
			progress.Emit(ctx, panel.Progress{Label: "download", Current: 2, Total: 2})
			if h, ok := panel.HandleFrom(ctx); ok {
				*complete = h.Complete(ctx, "ok", nil)
			}
			return json.RawMessage(`{"installed":"ktw-example"}`), nil
		}))
	ep.AddSlot(slot)
	return ep
}

// A method in a sidecar reports into its run's panel: the KTSP call carries
// the run's handle, the sidecar's dispatcher resumes it under the method's
// context, and the answer gives the last sequence back - the run's complete
// follows the method's last event with no gap and no duplicate, answered or
// failed. The method cannot complete the panel it was lent.
func TestASidecarMethodReportsIntoItsRunsPanel(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := map[bool]string{false: "answered", true: "failed"}[fail]
		t.Run(name, func(t *testing.T) {
			var completeErr error
			acts := &fakeActions{}
			bus, events := rig(t, acts)
			group, err := messagebus.Subscribe(context.Background(), bus, "t", "u", "ktw-univers", sidecarEndpoint(fail, &completeErr))
			if err != nil {
				t.Fatal(err)
			}
			defer group.Unsubscribe()
			sender := messagebus.NewSender(bus, "t", "u")
			acts.run = func(ctx context.Context) (json.RawMessage, error) {
				return sender.Call(ctx, "ktw-univers:/marketplace/install", nil)
			}
			_, runErr := runIt(t, context.Background(), taskapi.MethodRunDef("ktw-univers:marketplace/install", panel.NewID(), "Install", "")["config"].(map[string]any), nil)
			if fail {
				if runErr == nil {
					t.Fatal("a failed method gave its run no error")
				}
				assertNumberedThrough(t, waitEvents(t, events, 3), panel.KindProgress)
				return
			}
			if runErr != nil {
				t.Fatal(runErr)
			}
			assertNumberedThrough(t, waitEvents(t, events, 4), panel.KindProgress, panel.KindProgress)
			if completeErr == nil {
				t.Fatal("the sidecar completed the panel it was lent")
			}
		})
	}
}
