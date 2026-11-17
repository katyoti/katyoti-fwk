// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// HOOKS AS CONTRIBUTIONS: a `hook-type` declares a point, a `hook` declares a
// hook at a point for the objects its filter admits — the farthest layer of the
// hook table, under the capabilities, the type and the object.

func hookContribFixture(t *testing.T) *Service {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"},
		// The capability REMOVES one contributed hook for the objects it is on.
		{Name: "quiet", Hooks: api.Hooks{api.HookLint: {{URI: "x:lint/noisy", When: "false"}}}},
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "doc", Capabilities: []string{"container", "quiet"},
			// The type REPLACES a contributed hook (same uri, closer).
			Hooks: api.Hooks{api.HookLint: {{URI: "x:lint/spelling", When: "type == 'doc'"}}}},
		{Name: "plain", Capabilities: []string{"container"}},
	}
	cfg.CatalogTypes.HookTypes = []api.HookType{{Name: "review", Timing: api.HookTimingOnDemand, Answer: api.HookAnswerFindings}}
	cfg.CatalogTypes.Hooks = []api.Hook{
		{Name: "spelling", Provider: "ktw-x", Type: api.HookLint, URI: "x:lint/spelling"},
		{Name: "noisy", Provider: "ktw-x", Type: api.HookLint, URI: "x:lint/noisy"},
		{Name: "docs-only", Provider: "ktw-x", Type: api.HookLint, URI: "x:lint/docs", Filter: api.ObjectFilter{Types: []string{"doc"}}},
		{Name: "broken", Provider: "ktw-x", Type: api.HookLint, URI: "x:lint/broken", Filter: api.ObjectFilter{When: "type =="}},
		{Name: "second-look", Provider: "ktw-y", Type: "review", URI: "y:review/look"},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	for _, o := range []api.Object{{ID: "m.d", Type: "doc"}, {ID: "m.p", Type: "plain"}} {
		o.Name = o.ID
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	return svc
}

func table(t *testing.T, svc *Service, id string, p api.HookPoint) map[string]api.ResolvedHook {
	t.Helper()
	out := map[string]api.ResolvedHook{}
	for _, h := range hooksAt(t, svc, id, p) {
		out[h.URI] = h
	}
	return out
}

// TestAHookContributionIsTheFarthestLayer: it applies by its filter, a closer
// declaration replaces it (same uri) or removes it (`when: false`), and a
// filter that cannot be read is listed not applying, with the reason.
func TestAHookContributionIsTheFarthestLayer(t *testing.T) {
	svc := hookContribFixture(t)

	doc := table(t, svc, "m.d", api.HookLint)
	if h, ok := doc["x:lint/spelling"]; !ok || h.Source != "type:doc" {
		t.Errorf("spelling on the doc = %+v, want the type's (closer) entry", h)
	}
	if _, ok := doc["x:lint/noisy"]; ok {
		t.Errorf("the capability's when:false did not remove the contributed noisy hook: %+v", doc)
	}
	if h, ok := doc["x:lint/docs"]; !ok || !h.Applies || h.Source != "hook:ktw-x/docs-only" {
		t.Errorf("docs-only on a doc = %+v, want it applying from its contribution", h)
	}
	if h, ok := doc["x:lint/broken"]; !ok || h.Applies || h.Reason == "" {
		t.Errorf("an unreadable filter must be listed not applying with the reason, got %+v", h)
	}

	plain := table(t, svc, "m.p", api.HookLint)
	if _, ok := plain["x:lint/docs"]; ok {
		t.Errorf("docs-only reached a plain object: %+v", plain)
	}
	for _, uri := range []string{"x:lint/spelling", "x:lint/noisy"} {
		if h, ok := plain[uri]; !ok || h.Source != "hook:ktw-x/"+map[string]string{"x:lint/spelling": "spelling", "x:lint/noisy": "noisy"}[uri] || !h.Applies {
			t.Errorf("%s on a plain object = %+v, want the contribution applying", uri, h)
		}
	}
}

// TestAnExtensionOpensAPoint: a declared hook-type is a point like CWM's own —
// its hooks resolve; a point nothing declares is refused.
func TestAnExtensionOpensAPoint(t *testing.T) {
	svc := hookContribFixture(t)
	review := table(t, svc, "m.p", "review")
	if h, ok := review["y:review/look"]; !ok || !h.Applies || h.Point != "review" {
		t.Fatalf("the review hook = %+v (table %+v)", h, review)
	}
	_, err := svc.ObjectHooks(stdctx.Background(), api.ObjectHooksInput{ID: "m.p", Point: "publish"})
	if !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("an undeclared point: err = %v, want invalid input", err)
	}
}

// reviewActions answers the review point per object.
type reviewActions struct{ hookActions }

func (f *reviewActions) Run(ctx stdctx.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	_, _ = f.hookActions.Run(ctx, in)
	return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{"looked":true}`)}, nil
}

// TestObjectRunHooksRunsAnExtensionPoint: the extension's own point runs over
// a module and comes back raw (the extension reads its answers); a write point
// and an undeclared point are refused.
func TestObjectRunHooksRunsAnExtensionPoint(t *testing.T) {
	svc := hookContribFixture(t)
	actions := &reviewActions{}
	app := application.New(application.Config())
	application.PutService[actionsapi.ActionService](app, "default", actions)
	ctx := application.WithApplication(stdctx.Background(), app)

	out, err := svc.ObjectRunHooks(ctx, api.ObjectRunHooksInput{Point: "review", Module: "m"})
	if err != nil {
		t.Fatalf("ObjectRunHooks: %v", err)
	}
	if out.Objects != 2 || len(out.Runs) != 2 || out.Answer != api.HookAnswerFindings || out.Timing != api.HookTimingOnDemand {
		t.Fatalf("out = %+v, want the review hook run on both objects, the point's timing and answer echoed", out)
	}
	for _, r := range out.Runs {
		if r.URI != "y:review/look" || string(r.Answer) != `{"looked":true}` || r.Source != "hook:ktw-y/second-look" {
			t.Errorf("run = %+v", r)
		}
	}
	for _, p := range []api.HookPoint{api.HookBeforeCreate, api.HookOnUpdate, "publish"} {
		if _, err := svc.ObjectRunHooks(ctx, api.ObjectRunHooksInput{Point: p, ID: "m.d"}); !errors.Is(err, api.ErrInvalidInput) {
			t.Errorf("point %s: err = %v, want refused (a write point runs on writes; publish is undeclared)", p, err)
		}
	}
}

// TestObjectHooksResolvesTheInputForTheObject: a caller that runs a hook itself
// gets the input already resolved, as the action would receive it.
func TestObjectHooksResolvesTheInputForTheObject(t *testing.T) {
	svc := hookContribFixture(t)
	svc.Config.CatalogTypes.Hooks = append(svc.Config.CatalogTypes.Hooks,
		api.Hook{Name: "path", Provider: "ktw-x", Type: api.HookLint, URI: "x:lint/path", Input: map[string]any{"about": "${id}", "fixed": "yes"}})
	h := table(t, svc, "m.p", api.HookLint)["x:lint/path"]
	if h.Input["about"] != "${id}" || h.ResolvedInput["about"] != "m.p" || h.ResolvedInput["fixed"] != "yes" {
		t.Fatalf("hook = %+v, want the authored input and its resolution for m.p", h)
	}
}

// TestTheShippedHookTypesAreCWMsPoints: ktw-agents ships one hook-type file per
// built-in point, saying what the code does (timing, answer).
func TestTheShippedHookTypesAreCWMsPoints(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "..", "app", "ktw-agents", "embed", "cwm", "hook-types")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	shipped := map[api.HookPoint]api.HookType{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".i18n.yaml") {
			continue // a hook type's translations, carried as its layer — not a declaration
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var ht api.HookType
		if err := config.DecodeBytes(raw, marshal.YAML, &ht); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		shipped[ht.Name] = ht
	}
	if len(shipped) != len(api.BuiltinHookTypes) {
		t.Errorf("shipped %d hook-types, the code has %d points", len(shipped), len(api.BuiltinHookTypes))
	}
	for _, b := range api.BuiltinHookTypes {
		s, ok := shipped[b.Name]
		if !ok || s.Timing != b.Timing || s.Answer != b.Answer {
			t.Errorf("%s: shipped %+v, code %+v", b.Name, s, b)
		}
	}
}
