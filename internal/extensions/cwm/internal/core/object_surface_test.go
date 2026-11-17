// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// surfaceSvc builds a workspace with a type composing two capabilities plus a
// policy that attaches itself.
func surfaceSvc(t *testing.T) (*Service, stdctx.Context) {
	t.Helper()
	ctx := stdctx.Background()
	root := freshWorkspace(t)
	// WARN: the fixture stores a group nobody declares (`want`) on purpose — what
	// is tested is that the surface reports it, which error mode would refuse
	// to store in the first place.
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ObjectTypeValidation: api.TypeValidationWarn}
	cfg.CatalogTypes.Objects = []api.ObjectType{{
		Name:         "use-case",
		Capabilities: []string{"checkable", "content", "nobody-declares-this"},
		Actions:      []api.ActionRef{{URI: "p:tasks/type-action", Label: "curated"}},
		Sections:     map[string]map[string]any{"context": {"propagation": "local"}},
	}}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{
			Name: "checkable", Schema: "x:/data-type/cap-checkable",
			Badges: []api.BadgeDef{{Field: "status"}},
		},
		{
			Name: "content", Schema: "x:/data-type/cap-content",
			Actions: []api.ActionRef{{URI: "p:tasks/cap-action"}},
		},
		// Type-level: set by the type's `context:` section, no group on objects.
		{Name: "context", TypeSchema: "x:/data-type/type-context"},
	}
	cfg.CatalogTypes.Policies = []api.Policy{{
		Name: "require-tests", Label: "Tests required",
		Applies: api.ShapeFilter{Types: []string{"use-case"}},
	}}
	svc, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	o := obj("cwm.uc", "use-case", "")
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Fatalf("create: %v", err)
	}
	return svc, ctx
}

// THE POINT OF PHASE 5: a capability's contributions actually reach the object.
// Without a resolver those slots are declared and read by nobody — the exact
// shape this refactor exists to close.
func TestCapabilityContributionsReachTheObject(t *testing.T) {
	svc, ctx := surfaceSvc(t)
	got, err := svc.ObjectEffectiveSurface(ctx, api.ObjectEffectiveSurfaceInput{ID: "cwm.uc"})
	if err != nil {
		t.Fatalf("surface: %v", err)
	}

	// Actions: the type's own binding AND the capability's, with the source
	// recorded so a curated binding stays distinguishable.
	if len(got.Actions) != 2 {
		t.Fatalf("expected 2 actions (type + capability), got %+v", got.Actions)
	}
	if got.Actions[0].Source != "type" || got.Actions[0].Ref.Label != "curated" {
		t.Errorf("the type's curated binding is not first: %+v", got.Actions[0])
	}
	if got.Actions[1].Source != "capability:content" {
		t.Errorf("capability action source = %q", got.Actions[1].Source)
	}

	// Badges arrive from capabilities.
	if len(got.Badges) != 1 || got.Badges[0].Field != "status" {
		t.Errorf("capability badge did not reach the object: %+v", got.Badges)
	}
}

// A type's SECTION puts its capability in force: the surface — what the
// extensions read — lists the type-level capability the type's `context:`
// section sets, though the type's list does not name it.
func TestATypeSectionPutsItsCapabilityInForce(t *testing.T) {
	svc, ctx := surfaceSvc(t)
	got, err := svc.ObjectEffectiveSurface(ctx, api.ObjectEffectiveSurfaceInput{ID: "cwm.uc"})
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	if !containsString(got.Capabilities, "context") {
		t.Errorf("the section's capability is not in force: %v", got.Capabilities)
	}
	eff, err := svc.ObjectEffectiveCapabilities(ctx, api.ObjectEffectiveCapabilitiesInput{ID: "cwm.uc"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range eff.Capabilities {
		if c.ID == "context" {
			found = c.Source == "type"
		}
	}
	if !found {
		t.Errorf("effective capabilities do not carry the section's capability from the type: %+v", eff.Capabilities)
	}
}

// An undeclared capability is REPORTED, never invented. A capability that
// contributes nothing because nobody declared it must not look identical to one
// that declares nothing.
func TestUndeclaredCapabilityIsReported(t *testing.T) {
	svc, ctx := surfaceSvc(t)
	got, err := svc.ObjectEffectiveSurface(ctx, api.ObjectEffectiveSurfaceInput{ID: "cwm.uc"})
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	// 7b: the surface reads the EFFECTIVE set, so an instance group nobody
	// declares (the fixture's `want`) is reported beside the type's undeclared
	// id — both are "declared by nobody", which is the finding.
	if !containsString(got.UndeclaredCapabilities, "nobody-declares-this") {
		t.Errorf("undeclared capability not reported: %v", got.UndeclaredCapabilities)
	}
	if containsString(got.Capabilities, "nobody-declares-this") {
		t.Error("an undeclared capability was counted as in force")
	}
}

// A policy attaching itself shows up as a capability AND is named as a policy,
// so a bar another module imposed stays distinguishable from one the type chose.
func TestAttachedPolicyIsDistinguishable(t *testing.T) {
	svc, ctx := surfaceSvc(t)
	got, err := svc.ObjectEffectiveSurface(ctx, api.ObjectEffectiveSurfaceInput{ID: "cwm.uc"})
	if err != nil {
		t.Fatalf("surface: %v", err)
	}
	if containsString(got.Capabilities, "require-tests") {
		t.Errorf("a policy is not a capability: %v", got.Capabilities)
	}
	if !containsString(got.Policies, "require-tests") {
		t.Errorf("the policy that applies is not named: %v", got.Policies)
	}
	if containsString(got.Policies, "checkable") {
		t.Error("a type-composed capability was mislabelled a policy")
	}
}
