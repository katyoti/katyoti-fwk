// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// CAPABILITIES ON THE INSTANCE (concepts.md §4, 5d): an object may carry a
// capability its type never declared — and that group must NAME A DECLARED
// CAPABILITY. An undeclared group is refused, not silently carried: a typo'd
// group name would otherwise be stored, indexed and read by nobody, which is
// the "declared and read by nobody" shape this whole model exists to close.
//
// The same two rules as type validation apply: no declared capabilities ⇒ no
// check (an unmigrated workspace behaves as before), and warn is a visible
// log, never silence.
func svcWithCapabilities(t *testing.T, mode string, caps []api.Capability) *Service {
	t.Helper()
	root := freshWorkspace(t)
	cfg := &CWMServiceConfig{WorkspaceRoot: root, ObjectTypeValidation: mode}
	cfg.CatalogTypes.Objects = declared
	cfg.CatalogTypes.Capabilities = caps
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

var declaredCaps = []api.Capability{
	{Name: "container"}, {Name: "content"}, {Name: "checkable"}, {Name: "composition"}, {Name: "want"}, // want: the obj() helper carries it
}

func withGroup(o api.Object, group string) api.Object {
	_ = o.SetCapability(group, map[string]any{"members": []string{"cwm.other"}})
	return o
}

func TestErrorMode_RefusesUndeclaredInstanceCapability(t *testing.T) {
	svc := svcWithCapabilities(t, api.TypeValidationError, declaredCaps)
	o := withGroup(obj("cwm.x", "use-case", ""), "nonsense")
	_, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o})
	if !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("error mode must REFUSE an object carrying the undeclared capability group %q, got %v", "nonsense", err)
	}
}

// The point of instance capabilities: a DECLARED capability the TYPE does not
// list is accepted — that is what "type ∪ instance" means.
func TestErrorMode_AcceptsDeclaredCapabilityNotOnTheType(t *testing.T) {
	svc := svcWithCapabilities(t, api.TypeValidationError, declaredCaps)
	o := withGroup(obj("cwm.x", "use-case", ""), "composition") // use-case declares container/content/checkable only
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Fatalf("a DECLARED capability added on the instance must be accepted, got %v", err)
	}
}

func TestWarnMode_AcceptsUndeclaredInstanceCapability(t *testing.T) {
	svc := svcWithCapabilities(t, api.TypeValidationWarn, declaredCaps)
	o := withGroup(obj("cwm.x", "use-case", ""), "nonsense")
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Fatalf("warn mode must accept (and log) an undeclared group, got %v", err)
	}
}

func TestNoDeclaredCapabilities_GroupCheckIsOff(t *testing.T) {
	svc := svcWithCapabilities(t, api.TypeValidationError, nil)
	o := withGroup(obj("cwm.x", "use-case", ""), "nonsense")
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Fatalf("with NO declared capabilities the group check must be off, got %v", err)
	}
}

// Update is the other write path; a group cannot sneak in there either.
func TestErrorMode_UpdateRefusesUndeclaredInstanceCapability(t *testing.T) {
	svc := svcWithCapabilities(t, api.TypeValidationError, declaredCaps)
	o := obj("cwm.x", "use-case", "")
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: o}); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err := svc.ObjectUpdate(stdctx.Background(), api.ObjectUpdateInput{Module: "cwm", Object: withGroup(o, "nonsense")})
	if !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("update must refuse an undeclared group, got %v", err)
	}
}
