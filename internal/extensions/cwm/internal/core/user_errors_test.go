// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A person-facing refusal carries its message key and params (the GUI says it
// in the person's language from them), and its English text is unchanged.
func TestRefusalsCarryTheirMessageKey(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = svc.Close() })
		_, err = svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{ID: "cwm.never-was"})
		wantKey(t, err, "cwm.objectNotFound", "id", "cwm.never-was")
		if !strings.Contains(err.Error(), `object "cwm.never-was"`) {
			t.Errorf("text changed: %v", err)
		}
	})
	t.Run("hook refused", func(t *testing.T) {
		f := &hookActions{replies: map[string]string{"x:gate/create": `{"accept":false,"reason":"not in this module"}`}}
		svc, ctx, _ := dispatchFixture(t, f)
		_, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: gatedObject("billing.a")})
		wantKey(t, err, "cwm.refusedByHook", "reason", "not in this module")
	})
	t.Run("no transition", func(t *testing.T) {
		svc, ctx, _ := transitionFixture(t, nil)
		_, err := move(t, svc, ctx, "active", person)
		wantKey(t, err, "cwm.transitionNone", "to", "active")
	})
	t.Run("force not allowed", func(t *testing.T) {
		svc, ctx, _ := transitionFixture(t, nil)
		if _, err := move(t, svc, ctx, "proposed", person); err != nil {
			t.Fatal(err)
		}
		_, err := move(t, svc, ctx, "accepted", person)
		if m, ok := usermsg.Of(err); !ok || m.Key != "cwm.transitionRefused" {
			t.Errorf("a guard's refusal: key = %+v (%v), want cwm.transitionRefused", m, err)
		}
	})
	t.Run("size budget", func(t *testing.T) {
		svc := svcWithTypes(t, api.TypeValidationError, []api.ObjectType{{Name: "budgeted", MaxSizeBytes: 300}})
		_, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: budgeted("cwm.big", strings.Repeat("é", 150))})
		wantKey(t, err, "cwm.sizeBudget", "id", "cwm.big")
	})
	t.Run("unknown type", func(t *testing.T) {
		svc := svcWithTypes(t, api.TypeValidationError, []api.ObjectType{{Name: "budgeted"}})
		obj := budgeted("cwm.x", "court")
		obj.Type = "never-declared"
		_, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: obj})
		wantKey(t, err, "cwm.unknownType", "type", "never-declared")
	})
}

func wantKey(t *testing.T, err error, key, param, value string) {
	t.Helper()
	m, ok := usermsg.Of(err)
	if !ok || m.Key != key {
		t.Fatalf("err = %v: key %+v, want %s", err, m, key)
	}
	if got, _ := m.Params[param].(string); got != value {
		t.Errorf("%s: params[%s] = %v, want %q", key, param, m.Params[param], value)
	}
}
