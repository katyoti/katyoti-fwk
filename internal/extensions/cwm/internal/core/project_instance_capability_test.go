// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE PROJECT COMPOSES CAPABILITIES LIKE ANY OBJECT (phase 7-first): the
// project object at the project root is an ordinary object — an instance
// group naming a DECLARED capability its type does not list is in force on it,
// and a group naming an undeclared capability is refused in error mode (5d).
func projectCapabilityFixture(t *testing.T) *Service {
	t.Helper()
	cfg := &CWMServiceConfig{
		WorkspaceRoot:        freshWorkspace(t),
		ProjectRoot:          t.TempDir(),
		ObjectTypeValidation: api.TypeValidationError,
	}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: api.DefaultProjectType, Capabilities: []string{api.CapProject, "content"}},
	}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: api.CapProject}, {Name: "content"}, {Name: "confluence"},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.ProjectInit(stdctx.Background(), api.ProjectInitInput{Name: "acme", Title: "Acme"}); err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	return svc
}

func projectWithGroup(t *testing.T, svc *Service, group string) api.Object {
	t.Helper()
	got, err := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{ID: "acme"})
	if err != nil {
		t.Fatalf("ObjectGet project: %v", err)
	}
	o := got.Object
	if err := o.SetCapability(group, map[string]any{"space": "ACME"}); err != nil {
		t.Fatalf("SetCapability: %v", err)
	}
	return o
}

func TestProjectInstanceCapabilityIsEffective(t *testing.T) {
	svc := projectCapabilityFixture(t)
	ctx := stdctx.Background()
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Object: projectWithGroup(t, svc, "confluence")}); err != nil {
		t.Fatalf("a declared capability added on the project must be accepted, got %v", err)
	}
	eff, err := svc.ObjectEffectiveCapabilities(ctx, api.ObjectEffectiveCapabilitiesInput{ID: "acme"})
	if err != nil {
		t.Fatalf("ObjectEffectiveCapabilities: %v", err)
	}
	source := map[string]string{}
	for _, c := range eff.Capabilities {
		source[c.ID] = c.Source
	}
	if source["confluence"] != "object" {
		t.Errorf("the project's instance group confluence is not in force from the object: %v", source)
	}
	if source[api.CapProject] != "type" {
		t.Errorf("the project type's own capability is not in force from the type: %v", source)
	}
}

func TestProjectUndeclaredInstanceCapabilityIsRefused(t *testing.T) {
	svc := projectCapabilityFixture(t)
	_, err := svc.ObjectUpdate(stdctx.Background(), api.ObjectUpdateInput{Object: projectWithGroup(t, svc, "nonsense")})
	if !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("error mode must REFUSE the project carrying the undeclared group %q, got %v", "nonsense", err)
	}
}
