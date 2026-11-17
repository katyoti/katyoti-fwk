// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE LINT POINT: declaring a linter is declaring a lint entry — here on a
// capability every `doc` carries — and the lint point runs it over the objects.

func lintFixture(t *testing.T, actions actionsapi.ActionService) (*Service, stdctx.Context) {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(),
		ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"},
		{Name: "rules-in-force", Hooks: api.Hooks{api.HookLint: {{URI: "x:lint/rules"}}}},
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "doc", Capabilities: []string{"container", "rules-in-force"},
			Hooks: api.Hooks{api.HookLint: {{URI: "x:lint/broken"}}}},
		{Name: "plain", Capabilities: []string{"container"}},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	for _, o := range []api.Object{{ID: "billing.a", Type: "doc"}, {ID: "billing.b", Type: "doc"}, {ID: "billing.p", Type: "plain"}} {
		o.Name = o.ID
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	app := application.New(application.Config())
	if actions != nil {
		application.PutService[actionsapi.ActionService](app, "default", actions)
	}
	return svc, application.WithApplication(stdctx.Background(), app)
}

func lintReplies() *hookActions {
	return &hookActions{replies: map[string]string{
		"x:lint/rules":  `{"status":"fail","findings":[{"rule":"no-title","message":"a doc needs a title","severity":"warn"}]}`,
		"x:lint/broken": `"not a lint result"`,
	}}
}

func runLint(svc *Service, ctx stdctx.Context, in api.ObjectRunHooksInput) (api.ObjectRunHooksOutput, error) {
	in.Point = api.HookLint
	return svc.ObjectRunHooks(ctx, in)
}

// TestTheLintPointOverAModuleRunsEveryLintHookInForce: each doc's two hooks
// run (tagged with the hook and its declarer), the broken linter's garbage
// comes back as it was answered, and the plain object has no lint hook.
func TestTheLintPointOverAModuleRunsEveryLintHookInForce(t *testing.T) {
	svc, ctx := lintFixture(t, lintReplies())
	out, err := runLint(svc, ctx, api.ObjectRunHooksInput{Module: "billing"})
	if err != nil {
		t.Fatalf("lint point: %v", err)
	}
	if out.Objects != 2 || len(out.Runs) != 4 {
		t.Fatalf("objects %d runs %d, want 2 objects with hooks and 4 runs", out.Objects, len(out.Runs))
	}
	seen := map[string]api.HookRun{}
	for _, r := range out.Runs {
		seen[r.ObjectID+"|"+r.URI] = r
		if r.ObjectID == "billing.p" {
			t.Errorf("the plain object carries no lint hook, yet ran %s", r.URI)
		}
	}
	for _, id := range []string{"billing.a", "billing.b"} {
		if r, ok := seen[id+"|x:lint/rules"]; !ok || r.Source != "capability:rules-in-force" {
			t.Errorf("%s: the rules hook = %+v (present %v)", id, r, ok)
		}
		if r, ok := seen[id+"|x:lint/broken"]; !ok || string(r.Answer) != `"not a lint result"` {
			t.Errorf("%s: the broken hook's answer must come back raw, got %+v", id, r)
		}
	}
}

// TestTheLintPointWithNoActionServiceSaysSoOnEachRun.
func TestTheLintPointWithNoActionServiceSaysSoOnEachRun(t *testing.T) {
	svc, ctx := lintFixture(t, nil)
	out, err := runLint(svc, ctx, api.ObjectRunHooksInput{ID: "billing.a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Runs) != 2 {
		t.Fatalf("runs = %+v, want one per declared hook", out.Runs)
	}
	for _, r := range out.Runs {
		if r.Error == "" {
			t.Errorf("run %+v, want an error", r)
		}
	}
}

// TestAPointScopeIsExactlyOne: an object or a module, never both, never neither.
func TestAPointScopeIsExactlyOne(t *testing.T) {
	svc, ctx := lintFixture(t, lintReplies())
	for _, in := range []api.ObjectRunHooksInput{{}, {ID: "billing.a", Module: "billing"}} {
		if _, err := runLint(svc, ctx, in); !errors.Is(err, api.ErrInvalidInput) {
			t.Errorf("%+v: err = %v, want ErrInvalidInput", in, err)
		}
	}
}

// TestADeclaredInputIsWhatTheLinterReceives: an entry's `input:` replaces the
// default {point, object}, each ${...} resolved over the object — so a linter
// that takes {path} serves the lint point with no adapter.
func TestADeclaredInputIsWhatTheLinterReceives(t *testing.T) {
	actions := &hookActions{replies: map[string]string{"x:lint/file": `{"status":"pass","findings":[]}`}}
	svc, ctx := lintFixture(t, actions)
	svc.Config.CatalogTypes.Capabilities = append(svc.Config.CatalogTypes.Capabilities, api.Capability{
		Name: "has-file", Attach: &api.ShapeFilter{Types: []string{"plain"}},
		Hooks: api.Hooks{api.HookLint: {{URI: "x:lint/file", Input: map[string]any{"path": "${current.file}", "about": "${id}"}}}},
	})
	if _, err := runLint(svc, ctx, api.ObjectRunHooksInput{ID: "billing.p"}); err != nil {
		t.Fatalf("lint point: %v", err)
	}
	if len(actions.calls) != 1 {
		t.Fatalf("calls = %+v, want the one input-bound hook", actions.calls)
	}
	var got map[string]any
	if err := json.Unmarshal(actions.calls[0].raw, &got); err != nil {
		t.Fatal(err)
	}
	file, _ := got["path"].(string)
	if got["about"] != "billing.p" || file == "" || strings.Contains(file, "${") {
		t.Fatalf("input = %v, want {about: billing.p, path: the object's file}", got)
	}
	if _, has := got["point"]; has {
		t.Errorf("the default call leaked into a declared input: %v", got)
	}
}
