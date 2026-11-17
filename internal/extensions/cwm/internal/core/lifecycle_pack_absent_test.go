// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A TRANSITION ON AN OBJECT WHOSE PACK IS NOT LOADED SAYS SO:
// a command line with no universe hub opens the workspace on this binary's own
// types; another pack's object (a decision, ktw-methods') is then of a type no
// loaded pack declares, and the refusal names that — never "has no workflow",
// which blames the object.
func TestTransitionOnAnUnloadedPacksTypeNamesThePack(t *testing.T) {
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir(), ObjectTypeValidation: api.TypeValidationWarn}
	cfg.CatalogTypes.Capabilities = []api.Capability{{Name: "container"}, {Name: "lifecycle"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	d := api.Object{ID: "m.OD-9", Type: "decision"} // declared by no pack here
	d.Name = "a challenge"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: d}); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.OD-9", To: "applies", By: api.Actor{ID: "p", Kind: "person"}})
	if err == nil {
		t.Fatal("a transition on a type no pack declares passed")
	}
	msg := err.Error()
	if !strings.Contains(msg, "no loaded pack declares") || !strings.Contains(msg, "universe") {
		t.Fatalf("the refusal does not name the absent pack and the universe to start: %s", msg)
	}
	if strings.Contains(msg, "has no workflow") {
		t.Fatalf("the refusal blames the object: %s", msg)
	}
}
