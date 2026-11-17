// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// HOOKS AT NAMED POINTS: one resolver layers the declarations — capabilities,
// then the type, then the object — and says which apply and why not.

func hooksFixture(t *testing.T, obj api.Object) *Service {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(),
		ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"},
		{Name: "rules-in-force", Hooks: api.Hooks{
			api.HookLint: {
				{URI: "ktw-methods:rules/check"},
				{URI: "ktw-code:lint/go"},
				{URI: "ktw-code:lint/docs", Filter: &api.ObjectFilter{Types: []string{"doc"}}},
			},
			api.HookOnCreate: {{URI: "ktw-agents:journal/append", When: "id == 'nope'"}},
		}},
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{{
		Name: "module", Capabilities: []string{"container", "rules-in-force"},
		Hooks: api.Hooks{api.HookLint: {
			// The type REPLACES the capability's go linter with its own when.
			{URI: "ktw-code:lint/go", When: "type == 'module'"},
			{URI: "ktw-methods:methods/lint-structure"},
		}},
	}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	obj.Name = obj.ID
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "billing", Object: obj}); err != nil {
		t.Fatalf("ObjectCreate: %v", err)
	}
	return svc
}

func hooksAt(t *testing.T, svc *Service, id string, p api.HookPoint) []api.ResolvedHook {
	t.Helper()
	out, err := svc.ObjectHooks(stdctx.Background(), api.ObjectHooksInput{ID: id, Point: p})
	if err != nil {
		t.Fatalf("ObjectHooks: %v", err)
	}
	return out.Hooks
}

// TestHooksLayerFarthestFirstAndCloserReplaces: the capability's entries come
// first; the type's own entry for the same uri replaces the capability's; the
// type's own new entry comes after the farther ones.
func TestHooksLayerFarthestFirstAndCloserReplaces(t *testing.T) {
	svc := hooksFixture(t, api.Object{ID: "billing", Type: "module"})
	got := hooksAt(t, svc, "billing", api.HookLint)
	want := []struct{ uri, source string }{
		{"ktw-methods:rules/check", "capability:rules-in-force"},
		{"ktw-code:lint/go", "type:module"}, // replaced by the closer type entry, same position
		{"ktw-code:lint/docs", "capability:rules-in-force"},
		{"ktw-methods:methods/lint-structure", "type:module"},
	}
	if len(got) != len(want) {
		t.Fatalf("lint hooks = %+v, want %d entries", got, len(want))
	}
	for i, w := range want {
		if got[i].URI != w.uri || got[i].Source != w.source {
			t.Errorf("entry %d = %s from %s, want %s from %s", i, got[i].URI, got[i].Source, w.uri, w.source)
		}
	}
}

// TestHooksReportWhyAnEntryDoesNotApply: a filter or a `when` that does not
// hold keeps the entry in the table, marked not applying, with the reason.
func TestHooksReportWhyAnEntryDoesNotApply(t *testing.T) {
	svc := hooksFixture(t, api.Object{ID: "billing", Type: "module"})
	seen := map[string]bool{}
	for _, h := range hooksAt(t, svc, "billing", "") {
		seen[h.URI] = true
		switch h.URI {
		case "ktw-code:lint/docs":
			if h.Applies || h.Reason == "" {
				t.Errorf("a doc-only linter applies to a module: %+v", h)
			}
		case "ktw-agents:journal/append":
			if h.Applies || h.Reason != "when is false" {
				t.Errorf("an entry whose when is false applies: %+v", h)
			}
		case "ktw-code:lint/go":
			if !h.Applies {
				t.Errorf("the type's own entry (when true) does not apply: %+v", h)
			}
		}
	}
	for _, u := range []string{"ktw-code:lint/docs", "ktw-agents:journal/append", "ktw-code:lint/go"} {
		if !seen[u] {
			t.Errorf("%s is missing from the table; a non-applying entry is listed, not dropped", u)
		}
	}
}

// TestObjectHookFalseRemovesAFartherEntry: an object's own `when: "false"`
// removes the entry its capability declares — it is not listed at all.
func TestObjectHookFalseRemovesAFartherEntry(t *testing.T) {
	obj := api.Object{ID: "billing", Type: "module",
		Hooks: api.Hooks{api.HookLint: {{URI: "ktw-methods:rules/check", When: "false"}}}}
	svc := hooksFixture(t, obj)
	for _, h := range hooksAt(t, svc, "billing", api.HookLint) {
		if h.URI == "ktw-methods:rules/check" {
			t.Fatalf("an entry the object removed is still listed: %+v", h)
		}
	}
}

// TestHooksRefuseAnUnknownPoint: a typo in a point name is an error, never an
// empty table that reads as "nothing declared".
func TestHooksRefuseAnUnknownPoint(t *testing.T) {
	svc := hooksFixture(t, api.Object{ID: "billing", Type: "module"})
	_, err := svc.ObjectHooks(stdctx.Background(), api.ObjectHooksInput{ID: "billing", Point: "on_crate"})
	if !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("unknown point: err = %v, want ErrInvalidInput", err)
	}
}
