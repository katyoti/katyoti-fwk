// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE VERIFY POINT: CWM walks the subtree, runs every applying verify hook and
// returns what they answered, raw; it runs no check itself, and counting the
// answers is the verification service's.

func verifyFixture(t *testing.T, actions actionsapi.ActionService) (*Service, stdctx.Context) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"},
		{Name: "checkable", Hooks: api.Hooks{api.HookVerify: {{URI: "x:acceptance/verify"}}}},
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "module", Capabilities: []string{"container"}},
		{Name: "use-case", Capabilities: []string{"container"},
			Hooks: api.Hooks{api.HookVerify: {{URI: "x:usecase/shape", Input: map[string]any{"about": "${id}"}}}}},
		{Name: "criterion", Capabilities: []string{"container", "checkable"}},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	for _, o := range []api.Object{
		{ID: "m", Type: "module"},
		{ID: "m.uc", Type: "use-case"},
		{ID: "m.ac-pass", Type: "criterion"},
		{ID: "m.ac-fail", Type: "criterion"},
		{ID: "m.ac-bad", Type: "criterion"},
	} {
		parent := map[string]string{"m.uc": "m", "m.ac-pass": "m.uc", "m.ac-fail": "m.uc", "m.ac-bad": "m.uc"}[o.ID]
		o.Name = o.ID
		o.Parent = parent
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	app := application.New(application.Config())
	if actions != nil {
		application.PutService[actionsapi.ActionService](app, "default", actions)
	}
	return svc, application.WithApplication(stdctx.Background(), app)
}

// verifyReplies answers per object: the acceptance hook reads the object id
// from the hook call; the use-case verifier gets its declared input.
type verifyReplies struct {
	hookActions
}

func (f *verifyReplies) Run(ctx stdctx.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	_, _ = f.hookActions.Run(ctx, in) // record the call
	switch in.URI {
	case "x:usecase/shape":
		return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{"status":"pass","desc":"shaped"}`)}, nil
	case "x:acceptance/verify":
		var c api.HookCall
		_ = json.Unmarshal(in.Input, &c)
		switch c.Object.ID {
		case "m.ac-pass":
			return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{"status":"pass","verdict":"m.verdict-ac-pass"}`)}, nil
		case "m.ac-fail":
			return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{"status":"fail","evidence":"1 != 2"}`)}, nil
		}
		// An object that is not a verify result: no status.
		return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{"verdict":"m.verdict-ac-bad"}`)}, nil
	}
	return actionsapi.RunOutput{}, nil
}

// runVerify is the verify point over id and everything under it.
func runVerify(t *testing.T, svc *Service, ctx stdctx.Context, id string) api.ObjectRunHooksOutput {
	t.Helper()
	out, err := svc.ObjectRunHooks(ctx, api.ObjectRunHooksInput{Point: api.HookVerify, ID: id, Subtree: true})
	if err != nil {
		t.Fatalf("ObjectRunHooks verify: %v", err)
	}
	return out
}

// TestTheVerifyPointRunsEveryApplyingHookUnderTheObject: the walk reaches the
// grandchildren from the module; a type's `hooks: verify` entry is a verify hook with
// its input; each answer comes back raw (reading it is the verification
// service's).
func TestTheVerifyPointRunsEveryApplyingHookUnderTheObject(t *testing.T) {
	actions := &verifyReplies{}
	svc, ctx := verifyFixture(t, actions)
	out := runVerify(t, svc, ctx, "m")
	if out.Objects != 4 || len(out.Runs) != 4 {
		t.Fatalf("objects %d runs %d, want 4 / 4 (the use-case verifier + three criteria)", out.Objects, len(out.Runs))
	}
	answers := map[string]string{}
	for _, r := range out.Runs {
		answers[r.ObjectID] = string(r.Answer)
	}
	if answers["m.ac-fail"] != `{"status":"fail","evidence":"1 != 2"}` || answers["m.ac-pass"] != `{"status":"pass","verdict":"m.verdict-ac-pass"}` {
		t.Errorf("the answers are not raw: %v", answers)
	}
	for _, c := range actions.calls {
		if c.uri == "x:usecase/shape" {
			var in map[string]any
			_ = json.Unmarshal(c.raw, &in)
			if in["about"] != "m.uc" {
				t.Errorf("the type verifier's input = %v, want about: m.uc", in)
			}
		}
	}
}

// TestTheVerifyPointWithoutAnActionServiceSaysSoOnEachRun: never a silent
// empty answer.
func TestTheVerifyPointWithoutAnActionServiceSaysSoOnEachRun(t *testing.T) {
	svc, ctx := verifyFixture(t, nil)
	out := runVerify(t, svc, ctx, "m.uc")
	if len(out.Runs) != 4 {
		t.Fatalf("runs = %d, want 4", len(out.Runs))
	}
	for _, r := range out.Runs {
		if r.Error == "" || len(r.Answer) != 0 {
			t.Errorf("run %+v, want an error and no answer", r)
		}
	}
}

// TestTheVerifyPointWithNothingToRunRunsNothing.
func TestTheVerifyPointWithNothingToRunRunsNothing(t *testing.T) {
	svc, ctx := verifyFixture(t, &verifyReplies{})
	solo := api.Object{ID: "m.solo", Type: "module"}
	solo.Name = "solo"
	solo.Parent = "m"
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "m", Object: solo}); err != nil {
		t.Fatal(err)
	}
	if out := runVerify(t, svc, ctx, "m.solo"); len(out.Runs) != 0 || out.Objects != 0 {
		t.Fatalf("out = %+v, want nothing run", out)
	}
}
