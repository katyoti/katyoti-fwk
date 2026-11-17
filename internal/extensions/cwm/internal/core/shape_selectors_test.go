// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Capability attach and policy applies select by shape through one filter:
// a capability attaches to the objects carrying a capability, matched against
// what they carry BEFORE attachment (an attached capability never attaches
// another); a policy applies by capability over the effective set, attached
// ones included.
func TestAttachAndPoliciesSelectByCapability(t *testing.T) {
	ctx := stdctx.Background()
	root := freshWorkspace(t)
	cfg := &CWMServiceConfig{WorkspaceRoot: root}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "probe-log", Attach: &api.ShapeFilter{Capabilities: []string{"checkable"}}},
		{Name: "chained", Attach: &api.ShapeFilter{Capabilities: []string{"probe-log"}}},
	}
	cfg.CatalogTypes.Policies = []api.Policy{
		{Name: "checked-rule", Applies: api.ShapeFilter{Capabilities: []string{"checkable"}}},
		{Name: "logged-rule", Applies: api.ShapeFilter{Capabilities: []string{"probe-log"}}},
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
	caps := func(id string) []string {
		out, err := svc.ObjectEffectiveCapabilities(ctx, api.ObjectEffectiveCapabilitiesInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, c := range out.Capabilities {
			ids = append(ids, c.ID)
		}
		return ids
	}
	surface := func(id string) api.ObjectEffectiveSurfaceOutput {
		out, err := svc.ObjectEffectiveSurface(ctx, api.ObjectEffectiveSurfaceInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	uc := caps("cwm.uc")
	if !slices.Contains(uc, "probe-log") {
		t.Errorf("cwm.uc carries checkable: probe-log must attach, got %v", uc)
	}
	if slices.Contains(uc, "chained") {
		t.Errorf("an attached capability must not attach another: got %v", uc)
	}
	if s := surface("cwm.uc"); !slices.Contains(s.Capabilities, "probe-log") || slices.Contains(s.Capabilities, "chained") {
		t.Errorf("the surface must agree with the effective set: %v", s.Capabilities)
	}
	if got := surface("cwm.uc").Policies; !slices.Contains(got, "checked-rule") || !slices.Contains(got, "logged-rule") {
		t.Errorf("cwm.uc policies %v: both apply (one by its own capability, one by an attached one)", got)
	}

	if mod := caps("cwm.mod"); slices.Contains(mod, "probe-log") {
		t.Errorf("cwm.mod carries no checkable: probe-log must not attach, got %v", mod)
	}
	if got := surface("cwm.mod").Policies; len(got) != 0 {
		t.Errorf("cwm.mod policies %v: none applies", got)
	}
}
