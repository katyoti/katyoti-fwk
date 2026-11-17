// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/schematest"
)

// A CAPABILITY MANAGES ITS FIELDS: giving it gives its fields (defaults,
// required, the capabilities it uses) and, when it declares a workflow, its own
// lifecycle — moved by ObjectTransition{capability}, beside the object's own.

const requirementDSL = `$schema: ktw-schema/v1
name: cap-requirement
version: "1.0.0"
root: CapRequirement
types:
  CapRequirement:
    type: object
    fields:
      estimate:
        type: number
      priority:
        type: string@enum(low, normal, high)
        default: normal
        optional: true
      state:
        type: string
        optional: true
      trail:
        type: any
        optional: true
`

func requirementFixture(t *testing.T) (*Service, stdctx.Context) {
	t.Helper()
	compiled, err := schematest.CompileJSONSchema(stdctx.Background(), []byte(requirementDSL))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"}, {Name: "lifecycle"}, {Name: "want"},
		{Name: "requirement", Schema: "x:/data-type/cap-requirement", Uses: []string{"want"},
			Workflow: &api.CapabilityWorkflow{Name: "requirement-flow", State: "state", History: "trail"}},
	}
	cfg.CatalogTypes.CapabilitySchemas = map[string]json.RawMessage{"x:/data-type/cap-requirement": compiled}
	cfg.CatalogTypes.Workflows = []api.Workflow{
		approvalWorkflow(),
		{Name: "requirement-flow", Initial: "new",
			States: []api.WorkflowState{{Name: "new"}, {Name: "ready"}, {Name: "scheduled"}},
			Transitions: []api.WorkflowTransition{
				{Name: "refine", From: []string{"new"}, To: "ready"},
				{Name: "schedule", From: []string{"ready"}, To: "scheduled"},
			}},
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{{Name: "use-case", Capabilities: []string{"container", "lifecycle"}, Workflow: "approval"}}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()
	uc := api.Object{ID: "m.uc1", Type: "use-case"}
	uc.Name = "uc1"
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "m", Object: uc}); err != nil {
		t.Fatalf("create: %v", err)
	}
	return svc, ctx
}

func give(svc *Service, ctx stdctx.Context, capability string, fields map[string]any) (api.ObjectAddCapabilityOutput, error) {
	return svc.ObjectAddCapability(ctx, api.ObjectAddCapabilityInput{ID: "m.uc1", Capability: capability, Fields: fields})
}

func TestGivingACapabilityGivesItsFields(t *testing.T) {
	svc, ctx := requirementFixture(t)
	if _, err := give(svc, ctx, "requirement", map[string]any{"estimate": 3}); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "want") {
		t.Fatalf("requirement without the want it uses: %v, want refused naming want", err)
	}
	if _, err := give(svc, ctx, "want", nil); err != nil {
		t.Fatalf("give want: %v", err)
	}
	if _, err := give(svc, ctx, "requirement", map[string]any{"priority": "high"}); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "estimate") {
		t.Fatalf("a missing required field: %v, want refused naming estimate", err)
	}
	if _, err := give(svc, ctx, "requirement", map[string]any{"estimate": 3, "state": "ready"}); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("starting past the initial state: %v, want refused", err)
	}
	out, err := give(svc, ctx, "requirement", map[string]any{"estimate": 3})
	if err != nil {
		t.Fatalf("give requirement: %v", err)
	}
	g := out.Object.Capabilities["requirement"]
	if g["priority"] != "normal" || g["state"] != "new" || strings.Join(out.Defaulted, ",") != "priority,state" {
		t.Fatalf("group = %v, defaulted %v: want priority from the default and state at the initial", g, out.Defaulted)
	}
	if _, err := give(svc, ctx, "requirement", map[string]any{"estimate": 1}); !errors.Is(err, api.ErrAlreadyExists) {
		t.Fatalf("giving it twice: %v, want ErrAlreadyExists", err)
	}
}

// TestACapabilityHasItsOwnLifecycle: the requirement moves new → ready while the
// use case's own lifecycle stays where it is; the move goes to the trail the
// capability names (the journal records it too); a direct write of the state
// is refused.
func TestACapabilityHasItsOwnLifecycle(t *testing.T) {
	svc, ctx := requirementFixture(t)
	if _, err := give(svc, ctx, "want", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := give(svc, ctx, "requirement", map[string]any{"estimate": 3}); err != nil {
		t.Fatal(err)
	}
	own := func() string {
		got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.uc1"})
		l, _ := got.Object.Lifecycle()
		return l.State
	}
	before := own()
	out, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.uc1", Capability: "requirement", To: "ready", By: person})
	if err != nil {
		t.Fatalf("transition the requirement: %v", err)
	}
	g := out.Object.Capabilities["requirement"]
	trail, _ := json.Marshal(g["trail"])
	if out.From != "new" || g["state"] != "ready" || !strings.Contains(string(trail), `"to":"ready"`) {
		t.Fatalf("requirement = %v (from %s): want ready with the move in trail", g, out.From)
	}
	if own() != before {
		t.Fatalf("the use case's own lifecycle moved: %q → %q", before, own())
	}
	if _, err := svc.ObjectTransition(ctx, api.ObjectTransitionInput{ID: "m.uc1", Capability: "requirement", To: "new", By: person}); !errors.Is(err, api.ErrTransitionRefused) {
		t.Fatalf("a transition the workflow has not: %v, want refused", err)
	}
	// The state changes only through ObjectTransition.
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.uc1"})
	o := got.Object
	o.Capabilities = cloneCaps(o.Capabilities)
	o.Capabilities["requirement"]["state"] = "scheduled"
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: o}); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "requirement") {
		t.Fatalf("a direct write of the requirement's state: %v, want refused naming requirement", err)
	}
}

func TestRemovingACapability(t *testing.T) {
	svc, ctx := requirementFixture(t)
	if _, err := give(svc, ctx, "want", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := give(svc, ctx, "requirement", map[string]any{"estimate": 3}); err != nil {
		t.Fatal(err)
	}
	rm := func(c string) error {
		_, err := svc.ObjectRemoveCapability(ctx, api.ObjectRemoveCapabilityInput{ID: "m.uc1", Capability: c})
		return err
	}
	if err := rm("want"); !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "requirement") {
		t.Fatalf("removing want while requirement uses it: %v, want refused naming requirement", err)
	}
	if err := rm("container"); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("removing a capability the type brings: %v, want refused", err)
	}
	if err := rm("requirement"); err != nil {
		t.Fatalf("remove requirement (with its state): %v", err)
	}
	if err := rm("want"); err != nil {
		t.Fatalf("remove want once nothing uses it: %v", err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.uc1"})
	if _, ok := got.Object.Capabilities["requirement"]; ok {
		t.Fatal("the requirement group is still stored")
	}
}
