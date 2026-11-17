// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestTheShippedModuleLintAsksForEachModulesConcepts: ktw-methods'
// module-has-concepts hook contribution, as shipped, sends methods/file-exists the module's
// concepts path — and asks nothing of a module carrying system-module (told
// apart by the capability, here carried on the instance, never by its name).
func TestTheShippedModuleLintAsksForEachModulesConcepts(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "..", "app", "ktw-methods", "embed", "cwm", "hooks", "module-has-concepts.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var shipped api.Hook
	if err := config.DecodeBytes(raw, marshal.YAML, &shipped); err != nil {
		t.Fatal(err)
	}
	shipped.Name = "module-has-concepts" // the loader names it after its file
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "system-module"}, {Name: "module"}}
	cfg.CatalogTypes.Hooks = []api.Hook{shipped}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "module", Capabilities: []string{"container", "module"}}} // the level capability selects it
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	for _, m := range []string{"billing", "sys-inbox"} {
		o := api.Object{ID: m, Type: "module"}
		o.Name = m
		if m == "sys-inbox" {
			o.Capabilities = map[string]map[string]any{api.CapSystemModule: {}} // a tag: no fields
		}
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: m, Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	actions := &hookActions{replies: map[string]string{"ktw-methods:methods/file-exists": `{"status":"pass","findings":[]}`}}
	app := application.New(application.Config())
	application.PutService[actionsapi.ActionService](app, "default", actions)
	ctx := application.WithApplication(stdctx.Background(), app)

	for _, m := range []string{"billing", "sys-inbox"} {
		if _, err := svc.ObjectRunHooks(ctx, api.ObjectRunHooksInput{Point: api.HookLint, ID: m}); err != nil {
			t.Fatalf("lint point %s: %v", m, err)
		}
	}
	if len(actions.calls) != 1 {
		t.Fatalf("calls = %d, want one (billing; sys-inbox is not asked)", len(actions.calls))
	}
	var in map[string]string
	_ = json.Unmarshal(actions.calls[0].raw, &in)
	if actions.calls[0].uri != "ktw-methods:methods/file-exists" || in["path"] != "modules/billing/concepts.md" {
		t.Fatalf("call = %s %v", actions.calls[0].uri, in)
	}
}
