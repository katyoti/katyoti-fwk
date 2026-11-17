// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// A save never overwrites a change made elsewhere since the pane loaded: the
// revision the pane read travels with every save, so CWM refuses a write over
// a newer stored version (cwm.objectConflict, « reload »). The merge re-reads
// the object; without the revision it would silently overwrite that change.

func TestThePaneKeepsTheRevisionItRead(t *testing.T) {
	o := cwmapi.Object{ID: "billing.us-12", Type: "feature"}
	if got := objectDetailView(paneapi.DetailOutput{Object: o, Revision: "r1"}).Revision; got != "r1" {
		t.Fatalf("the pane's detail lost the revision read: %q", got)
	}
}

func TestASaveSendsTheRevisionThePaneRead(t *testing.T) {
	stored := cwmapi.Object{ID: "talks.d1", Type: "slide-deck", Body: "# old"}
	stored.Capabilities = map[string]map[string]any{"deck": {"theme": "katyoti"}}
	for name, save := range map[string]func(s *multiStub) (WorkspaceObjectMutationResult, error){
		"fields": func(s *multiStub) (WorkspaceObjectMutationResult, error) {
			return objectUpdateVia(context.Background(), s, "talks.d1", `{"deck":{"theme":"acme"}}`, "", "r1")
		},
		"body": func(s *multiStub) (WorkspaceObjectMutationResult, error) {
			body := "# new"
			return objectWriteVia(context.Background(), s, "talks.d1", "", "", &body, "r1")
		},
	} {
		s := &multiStub{t: t, replies: map[string]any{
			"ktw-agents:cwm/object-get":    cwmapi.ObjectGetOutput{Object: stored, Revision: "r2"},
			"ktw-agents:cwm/object-update": cwmapi.ObjectUpdateOutput{Object: stored, Revision: "r3"},
		}}
		out, err := save(s)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		in := s.inputFor("ktw-agents:cwm/object-update").(cwmapi.ObjectUpdateInput)
		if in.Revision != "r1" {
			t.Errorf("%s: the write is conditioned on %q, want the pane's r1 (not the merge's fresh read)", name, in.Revision)
		}
		if out.Revision != "r3" {
			t.Errorf("%s: the next save needs the version just written; got %q", name, out.Revision)
		}
	}
}

func TestASaveOverAChangeMadeElsewhereAsksToReload(t *testing.T) {
	stored := cwmapi.Object{ID: "billing.us-12", Type: "feature"}
	s := &multiStub{t: t,
		replies: map[string]any{"ktw-agents:cwm/object-get": cwmapi.ObjectGetOutput{Object: stored, Revision: "r2"}},
		errs:    map[string]error{"ktw-agents:cwm/object-update": cwmapi.ObjectConflict("billing.us-12", "r1")},
	}
	_, err := objectUpdateVia(context.Background(), s, "billing.us-12", `{"content":{"status":"done"}}`, "", "r1")
	if err == nil {
		t.Fatal("a save over a newer version was accepted")
	}
	if m := errorMsgOf(err); m == nil || m.Key != "cwm.objectConflict" {
		t.Fatalf("the refusal does not ask to reload: %v (%+v)", err, m)
	}
}
