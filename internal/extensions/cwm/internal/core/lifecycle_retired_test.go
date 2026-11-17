// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// An object follows its TYPE's workflow: a `workflow` its lifecycle group
// names is not read (a laxer one would walk around the type's gates under the
// same state), and a raw write cannot set it — nor forge a `history`, which
// is the journal's.

// laxWorkflow has approval's states, with accept ungated.
func laxWorkflow() api.Workflow {
	wf := approvalWorkflow()
	wf.Name = "lax"
	wf.Transitions = []api.WorkflowTransition{{Name: "accept", From: []string{"*"}, To: "accepted"}}
	return wf
}

func TestAnObjectCannotNameALaxerWorkflow(t *testing.T) {
	svc, ctx, _ := transitionFixture(t, nil)
	svc.Config.CatalogTypes.Workflows = append(svc.Config.CatalogTypes.Workflows, laxWorkflow())
	for _, to := range []string{"proposed", "in-review"} {
		if _, err := move(t, svc, ctx, to, agent); err != nil {
			t.Fatalf("to %s: %v", to, err)
		}
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.g1"})

	// A raw write naming the laxer workflow, the state unchanged: refused.
	cheat := got.Object
	cheat.Capabilities = cloneCaps(got.Object.Capabilities)
	cheat.Capabilities[api.CapLifecycle]["workflow"] = "lax"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: cheat}); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "workflow") {
		t.Fatalf("a raw write naming a workflow: %v, want refused", err)
	}
	// Even an object stored with one is judged by its type's workflow.
	slot, err := svc.lifecycleSlotOf(cheat, "")
	if err != nil || slot.workflow != "approval" {
		t.Fatalf("the slot of an object naming lax: %+v (%v), want approval", slot, err)
	}
	if _, err := move(t, svc, ctx, "accepted", agent); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("an agent accepted: %v, want refused by approval's gate", err)
	}
}

func TestARawWriteCannotForgeTheHistory(t *testing.T) {
	svc, ctx, _ := transitionFixture(t, nil)
	if _, err := move(t, svc, ctx, "proposed", person); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.g1"})
	forged := got.Object
	forged.Capabilities = cloneCaps(got.Object.Capabilities)
	forged.Capabilities[api.CapLifecycle]["history"] = []any{map[string]any{"to": "proposed", "by": map[string]any{"id": "ceo", "kind": "person"}, "at": "2026-01-01T00:00:00Z"}}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: forged}); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "history") {
		t.Fatalf("a raw write of a history: %v, want refused", err)
	}
	// Nor at birth.
	born := api.Object{ID: "billing.g2", Type: "goal"}
	born.Name = "g2"
	_ = born.SetCapability(api.CapLifecycle, map[string]any{"history": []any{}})
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: born}); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "history") {
		t.Fatalf("a create carrying a history: %v, want refused", err)
	}
}

// An object stored with one (written before it was retired) stays writable
// while the write keeps it unchanged, and removing it is always allowed.
func TestAStoredRetiredFieldIsRepairedOneWriteAtATime(t *testing.T) {
	old := api.Object{ID: "m.g"}
	_ = old.SetCapability(api.CapLifecycle, map[string]any{"state": "done", "history": []any{map[string]any{"to": "done"}}})
	kept := old
	kept.Capabilities = cloneCaps(old.Capabilities)
	if err := checkRetiredLifecycleFields(&old, kept); err != nil {
		t.Errorf("an unchanged stored history: %v, want allowed", err)
	}
	changed := old
	changed.Capabilities = cloneCaps(old.Capabilities)
	changed.Capabilities[api.CapLifecycle]["history"] = []any{}
	if err := checkRetiredLifecycleFields(&old, changed); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("a changed stored history: %v, want refused", err)
	}
	removed := old
	removed.Capabilities = cloneCaps(old.Capabilities)
	delete(removed.Capabilities[api.CapLifecycle], "history")
	if err := checkRetiredLifecycleFields(&old, removed); err != nil {
		t.Errorf("removing the stored history: %v, want allowed", err)
	}
}
