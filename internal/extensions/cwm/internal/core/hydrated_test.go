// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestAPredicateSeesTheObjectBeingWritten: a before-hook is handed the object
// as it is about to be — here in `ready` — with no body. Its environment must
// read that state (not the stored `draft`), and still get the stored body.
func TestAPredicateSeesTheObjectBeingWritten(t *testing.T) {
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "content"}, {Name: "lifecycle"}}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "request", Capabilities: []string{"container", "content", "lifecycle"}}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	stored := api.Object{ID: "m.r1", Type: "request"}
	stored.Name = "r1"
	stored.Name = "r1"
	stored.SetFieldsJSON(json.RawMessage(`{"priority":"high"}`))
	stored.SetLifecycle(api.Lifecycle{State: "draft"})
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "m", Object: stored}); err != nil {
		t.Fatalf("create: %v", err)
	}

	next := api.Object{ID: "m.r1", Type: "request"}
	next.Name = "r1"
	next.Name = "r1" // no body, as the index copy has none
	next.SetLifecycle(api.Lifecycle{State: "ready"})

	env := svc.objectActionEnv(next)
	for expr, want := range map[string]bool{
		"object.lifecycle.state == 'ready'": true,
		"object.lifecycle.state == 'draft'": false,
		"object.fields.priority == 'high'":  true,
	} {
		got, err := objectExprs.EvalPredicate(expr, env)
		if err != nil || got != want {
			t.Errorf("%s = %v (%v), want %v", expr, got, err, want)
		}
	}
}
