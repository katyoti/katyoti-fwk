// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"testing"

	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// actionSender answers ActionService.Run with the output it is given and
// keeps what it was sent.
type actionSender struct {
	out  actionsapi.RunOutput
	path string
	in   actionsapi.RunInput
}

func (s *actionSender) Call(_ context.Context, path string, params any, _ ...ktsapi.CallOption) (json.RawMessage, error) {
	s.path = path
	s.in, _ = params.(actionsapi.RunInput)
	return json.Marshal(s.out)
}

func (s *actionSender) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }

// TestAMethodPanelActionRunsThroughTheActionSurface -: a
// method-panel action runs through ActionService with its resolved args and
// the object it runs on as the run's target; a long method's answer carries
// the display panel its run reports into, drawn in place by the tab, and a
// short one its value.
func TestAMethodPanelActionRunsThroughTheActionSurface(t *testing.T) {
	const uri = "ktw-acme:confluence/status"
	args := map[string]any{"folder": "/p/modules/m/confluence", "dry_run": true}
	in, err := methodRunInput(uri, "m.dossier", args)
	if err != nil {
		t.Fatal(err)
	}
	s := &actionSender{out: actionsapi.RunOutput{URI: uri, TaskID: "run-1", Panel: "dp-1"}}
	got := methodRunResult(runActionCall(context.Background(), s, uri, in))

	if s.path != "ktw-agents:action/run" {
		t.Errorf("sent to %q, want ActionService.Run", s.path)
	}
	if s.in.URI != uri || s.in.Context["_target"] != "m.dossier" {
		t.Errorf("run input = %+v, want the method's URI and the object as _target", s.in)
	}
	var sent map[string]any
	if err := json.Unmarshal(s.in.Input, &sent); err != nil || sent["folder"] != "/p/modules/m/confluence" || sent["dry_run"] != true {
		t.Errorf("input = %s, want the resolved args, typed", s.in.Input)
	}
	if got.Panel != "dp-1" || got.TaskID != "run-1" || got.Error != "" {
		t.Errorf("a long method's run = %+v, want its run and its panel", got)
	}

	s.out = actionsapi.RunOutput{URI: uri, Value: json.RawMessage(`{"report":"3 pages"}`)}
	got = methodRunResult(runActionCall(context.Background(), s, uri, in))
	if got.Panel != "" || got.Result != `{"report":"3 pages"}` {
		t.Errorf("a short method's run = %+v, want its value and no panel", got)
	}

	if in, _ := methodRunInput(uri, "", args); in.Context != nil {
		t.Errorf("no object: the run must name no target, got %v", in.Context)
	}
}
