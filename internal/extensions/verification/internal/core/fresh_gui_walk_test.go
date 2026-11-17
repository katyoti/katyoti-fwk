// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// : verify never runs a live GUI walk - a gate's
// question runs in no GUI environment and would write an environment fail
// over the batch's pass. A GUI-walk criterion is counted from its latest
// verdict for its probe whatever its age, and with none reads unproven; its
// hook never runs. A stale verdict of any other criterion still runs it, and
// an explicit Verify still runs the walk.
func TestVerifyCountsAGUIWalkFromItsVerdictNeverRunsIt(t *testing.T) {
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	walk := "bash app/ktw-gui/frontend/tests/live/source-model-switch.sh"
	cli := "bash tests/live/hub-lifetime.sh"
	cwm := &treeCWM{objects: []cwmapi.Object{
		{ID: "m.F-1", Type: "feature"},
		criterion("m.AC-1", walk), verdict("m.AC-1", walk, "pass", now.Add(-72*time.Hour)),
		criterion("m.AC-2", "bash app/ktw-gui/frontend/tests/live/unrun.sh"),
		criterion("m.AC-3", "go test ./x"), verdict("m.AC-3", "go test ./x", "pass", now.Add(-72*time.Hour)),
		criterion("m.AC-4", cli), verdict("m.AC-4", cli, "pass", now.Add(-72*time.Hour)),
	}}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		application.DeclareSimpleService[cwmapi.CWMService](func(context.Context) (cwmapi.CWMService, error) { return cwm, nil }))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	ctx := application.WithApplication(context.Background(), app)
	svc := &Service{now: func() time.Time { return now }}

	sum, err := svc.VerifyFunction(ctx, cwmapi.FunctionCall{Object: "m.F-1"})
	if err != nil {
		t.Fatal(err)
	}
	ran := map[string]bool{}
	for _, id := range cwm.ran {
		ran[id] = true
	}
	if ran["m.AC-1"] || ran["m.AC-2"] {
		t.Errorf("verify() ran a live GUI walk: ran %v", cwm.ran)
	}
	if !ran["m.AC-3"] || !ran["m.AC-4"] {
		t.Errorf("a stale verdict of a criterion that is no GUI walk (a CLI tests/live probe too) runs again: ran %v", cwm.ran)
	}
	if sum.Passed != 3 || sum.Failed != 0 || sum.Unproven != 1 {
		t.Errorf("summary: %+v (want 3 passed - the walk from its 3-day-old verdict -, the walk with no verdict unproven)", sum)
	}

	cwm.ran = nil
	if _, err := svc.Verify(ctx, api.VerifyInput{ID: "m.AC-1"}); err != nil {
		t.Fatal(err)
	}
	if len(cwm.ran) == 0 || cwm.ran[0] != "m.AC-1" {
		t.Errorf("an explicit Verify of the criterion runs its walk: ran %v", cwm.ran)
	}
}

// GUIWalk is the one owner of « is this probe a GUI walk »: a tests/live
// script of the GUI; any other tests/live script is a CLI probe.
func TestGUIWalkIsTheGUIsLiveScripts(t *testing.T) {
	for probe, want := range map[string]bool{
		"bash app/ktw-gui/frontend/tests/live/x.sh": true,
		`bash app\ktw-gui\frontend\tests\live\x.sh`: true,
		"bash tests/live/hub-lifetime.sh":           false,
		"go test ./x":                               false,
	} {
		if got := api.GUIWalk(probe); got != want {
			t.Errorf("GUIWalk(%q) = %v, want %v", probe, got, want)
		}
	}
}
