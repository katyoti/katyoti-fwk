// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"sort"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A POLICY IS NOT A CAPABILITY. A rule put on objects by type applies to them —
// its checks run, its guidance reaches an agent — but it holds no field of the
// object, so it is never in the object's capability set. A capability that
// attaches itself by type (the memory every object has) is in the set, as
// `attached`.
func TestPoliciesApplyButAreNotCapabilities(t *testing.T) {
	ctx := stdctx.Background()
	root := freshWorkspace(t)
	cfg := &CWMServiceConfig{WorkspaceRoot: root}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "memory", Attach: &api.ShapeFilter{Types: []string{"*"}}},
	}
	cfg.CatalogTypes.Policies = []api.Policy{
		{Name: "require-tests", Label: "Tests required",
			Applies: api.ShapeFilter{Types: []string{"use-case", "feature"}}},
		{Name: "config-no-direct-io", Label: "Config through the service",
			Applies: api.ShapeFilter{Types: []string{"*"}},
			Hooks: api.Hooks{api.HookVerify: {{URI: "x:acceptance/probe",
				Input: map[string]any{"id": "${id}", "kind": "command", "probe": "bash modules/config/probes/cfg1-no-loadinto-outside-cli.sh"}}}},
			Agent: &api.AgentDecl{System: []api.AgentSection{{ID: "cfg", Text: "Config goes through the config service."}}}},
	}
	svc, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	for _, o := range []api.Object{
		{ID: "cwm.uc", Type: "use-case", Name: "uc", Capabilities: map[string]map[string]any{"checkable": {"probe": "true"}}},
		{ID: "cwm.mod", Type: "module", Name: "mod"},
	} {
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}

	type row struct{ id, source string }
	caps := func(id string) []row {
		out, err := svc.ObjectEffectiveCapabilities(ctx, api.ObjectEffectiveCapabilitiesInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		var rows []row
		for _, a := range out.Capabilities {
			rows = append(rows, row{a.ID, a.Source})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
		return rows
	}
	if got := caps("cwm.uc"); len(got) != 2 || got[0] != (row{"checkable", "object"}) || got[1] != (row{"memory", "attached"}) {
		t.Errorf("cwm.uc capabilities: %+v — want its own checkable and the attached memory, no policy", got)
	}
	policies := func(id string) []string {
		surf, err := svc.ObjectEffectiveSurface(ctx, api.ObjectEffectiveSurfaceInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		names := append([]string(nil), surf.Policies...)
		sort.Strings(names)
		return names
	}
	if got := policies("cwm.uc"); len(got) != 2 || got[0] != "config-no-direct-io" || got[1] != "require-tests" {
		t.Errorf("cwm.uc policies: %v — want both", got)
	}
	if got := policies("cwm.mod"); len(got) != 1 || got[0] != "config-no-direct-io" {
		t.Errorf("cwm.mod policies: %v — want only the '*' one", got)
	}

	surf, err := svc.ObjectEffectiveSurface(ctx, api.ObjectEffectiveSurfaceInput{ID: "cwm.mod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(surf.Policies) != 1 || containsString(surf.Capabilities, "config-no-direct-io") {
		t.Errorf("surface: policies %v capabilities %v", surf.Policies, surf.Capabilities)
	}

	// A policy is measured by its verify entry, a layer of its own, with its
	// declared input resolved for the object.
	hooks, err := svc.ObjectHooks(ctx, api.ObjectHooksInput{ID: "cwm.mod", Point: api.HookVerify})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, h := range hooks.Hooks {
		found = found || (h.Source == "policy:config-no-direct-io" && h.Applies && h.URI == "x:acceptance/probe")
	}
	if !found {
		t.Errorf("the policy's verify entry is not in force on the module: %+v", hooks.Hooks)
	}
}
