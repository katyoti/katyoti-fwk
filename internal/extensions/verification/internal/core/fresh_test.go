// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	acceptanceapi "github.com/katyoti/katyoti-fwk/internal/extensions/acceptance/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// treeCWM is a feature, its criteria and their verdicts; it records which
// objects had their verify hooks run, each criterion's hook answering pass.
type treeCWM struct {
	cwmapi.CWMService
	objects []cwmapi.Object
	ran     []string
}

func (c *treeCWM) ObjectGet(_ context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	for _, o := range c.objects {
		if o.ID == in.ID {
			return cwmapi.ObjectGetOutput{Object: o}, nil
		}
	}
	return cwmapi.ObjectGetOutput{}, cwmapi.ObjectNotFound(in.ID)
}

func (c *treeCWM) ObjectList(_ context.Context, in cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error) {
	out := cwmapi.ObjectListOutput{}
	for _, o := range c.objects {
		if o.Parent == in.Parent {
			out.Objects = append(out.Objects, o)
		}
	}
	return out, nil
}

func (c *treeCWM) ObjectRunHooks(_ context.Context, in cwmapi.ObjectRunHooksInput) (cwmapi.ObjectRunHooksOutput, error) {
	if in.Subtree || in.Point != cwmapi.HookVerify {
		panic("verify() runs one object's verify hooks at a time")
	}
	c.ran = append(c.ran, in.ID)
	o, _ := c.ObjectGet(context.Background(), cwmapi.ObjectGetInput{ID: in.ID})
	if _, ok := o.Object.Capability(cwmapi.CapCheckable); !ok {
		return cwmapi.ObjectRunHooksOutput{}, nil
	}
	return cwmapi.ObjectRunHooksOutput{Objects: 1, Runs: []cwmapi.HookRun{{ObjectID: in.ID, URI: "probe", Answer: []byte(`{"status":"pass"}`)}}}, nil
}

func criterion(id, probe string) cwmapi.Object {
	o := cwmapi.Object{ID: id, Type: "use-case", Parent: "m.F-1"}
	_ = o.SetCapability(cwmapi.CapCheckable, map[string]any{"kind": "command", "probe": probe})
	return o
}

func verdict(of, probe, status string, at time.Time) cwmapi.Object {
	o := cwmapi.Object{ID: "m.verdict-" + of[2:], Type: acceptanceapi.VerdictType, Parent: of}
	_ = o.SetCapability(acceptanceapi.CapVerdict, map[string]any{"status": status, "probe": probe, "ran_at": at.UTC().Format(time.RFC3339), "evidence": "recorded"})
	return o
}

// : verify counts a criterion from its fresh verdict -
// recorded within the hour, for the probe it declares now - without running
// its check; a stale verdict, or one recorded for another probe, runs again.
func TestVerifyCountsAFreshVerdictWithoutRunningItsCheck(t *testing.T) {
	now := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	cwm := &treeCWM{objects: []cwmapi.Object{
		{ID: "m.F-1", Type: "feature"},
		criterion("m.AC-1", "bash live.sh"), verdict("m.AC-1", "bash live.sh", "pass", now.Add(-10*time.Minute)),
		criterion("m.AC-2", "go test ./x"), verdict("m.AC-2", "go test ./x", "pass", now.Add(-2*time.Hour)),
		criterion("m.AC-3", "go test ./y -run New"), verdict("m.AC-3", "go test ./y", "pass", now.Add(-time.Minute)),
		criterion("m.AC-4", "bash slow.sh"), verdict("m.AC-4", "bash slow.sh", "fail", now.Add(-5*time.Minute)),
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
	for _, id := range cwm.ran {
		if id == "m.AC-1" || id == "m.AC-4" {
			t.Errorf("%s has a fresh verdict, yet its check ran", id)
		}
	}
	ran := map[string]bool{}
	for _, id := range cwm.ran {
		ran[id] = true
	}
	if !ran["m.AC-2"] || !ran["m.AC-3"] || !ran["m.F-1"] {
		t.Errorf("a stale verdict or a changed probe runs again, the feature's own hooks too: ran %v", cwm.ran)
	}
	if sum.Runs != 4 || sum.Passed != 3 || sum.Failed != 1 || sum.Green {
		t.Errorf("summary: %+v (want 4 runs, 3 passed - one from its fresh verdict -, the fresh fail failing)", sum)
	}
}
