// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"sort"
	"testing"

	scriptsapi "github.com/katyoti/katyoti-fwk/internal/extensions/scripts/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// Minimal in-memory ScriptService stub so we can drive the per-script
// slot-method handler without spinning up engines / loaders. Captures
// the URI it was called with so the test can assert dispatch.
type stubScriptSvc struct {
	scripts  []scriptsapi.ScriptInfo
	lastIn   scriptsapi.ScriptRunInput
	lastSeen string
}

func (s *stubScriptSvc) List(_ context.Context) (scriptsapi.ScriptListOutput, error) {
	return scriptsapi.ScriptListOutput{Scripts: s.scripts}, nil
}

func (s *stubScriptSvc) Run(_ context.Context, in scriptsapi.ScriptRunInput) (scriptsapi.ScriptRunOutput, error) {
	s.lastIn = in
	s.lastSeen = in.Name
	return scriptsapi.ScriptRunOutput{
		Stdout:   "ran " + in.Name,
		ExitCode: 0,
	}, nil
}

// addPerScriptMethodsForTest mirrors what PerScriptSlotMethodsExtension's
// Bootstrap hook does, minus the application-resolution scaffolding —
// keeps this test in `local_test` (external) without importing internal
// helpers. It exercises the same Slot.Add codepath against the same
// service interface so any drift in the production hook breaks the
// test loudly.
func addPerScriptMethodsForTest(t *testing.T, slot *ktsp.Slot, svc scriptsapi.ScriptService) {
	t.Helper()
	out, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, info := range out.Scripts {
		info := info // capture
		methodName := info.Name
		uri := info.URI()
		slot.Add(ktsp.RawMethod(methodName, ktsp.MethodOpts{
			Desc: "test per-script method",
			Tags: ktsp.MethodTags{ktsp.TagMethodCLI: "false"},
		}, func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			out, err := svc.Run(ctx, scriptsapi.ScriptRunInput{Name: uri})
			if err != nil {
				return nil, err
			}
			return json.Marshal(out)
		}))
	}
}

func TestPerScriptSlotMethod_RegistersOnePerScript(t *testing.T) {
	svc := &stubScriptSvc{scripts: []scriptsapi.ScriptInfo{
		{Name: "hello", Provider: "ktw-docs", Extension: ".sh"},
		{Name: "build", Provider: "ktw-docs", Extension: ".ts"},
		{Name: "deploy", Provider: "ktw-example", Extension: ".py"},
	}}

	slot := ktsp.NewSlot("scripts", ktsp.DefaultInstance, "test", nil)
	addPerScriptMethodsForTest(t, slot, svc)

	got := slot.Methods()
	sort.Strings(got)
	want := []string{"build", "deploy", "hello"}
	if len(got) != len(want) {
		t.Fatalf("registered %d methods, want %d: %v", len(got), len(want), got)
	}
	for i, m := range got {
		if m != want[i] {
			t.Errorf("method[%d] = %q, want %q", i, m, want[i])
		}
	}
}

func TestPerScriptSlotMethod_DispatchesByURI(t *testing.T) {
	svc := &stubScriptSvc{scripts: []scriptsapi.ScriptInfo{
		{Name: "hello", Provider: "ktw-docs"},
	}}
	slot := ktsp.NewSlot("scripts", ktsp.DefaultInstance, "test", nil)
	addPerScriptMethodsForTest(t, slot, svc)

	resp, err := slot.Handle(context.Background(), "hello", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	var out scriptsapi.ScriptRunOutput
	if err := json.Unmarshal(resp, &out); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if out.Stdout != "ran ktw-docs:hello" {
		t.Errorf("dispatch did not forward URI properly: got Stdout=%q", out.Stdout)
	}
	if svc.lastSeen != "ktw-docs:hello" {
		t.Errorf("svc.Run saw Name=%q, want %q", svc.lastSeen, "ktw-docs:hello")
	}
}

func TestPerScriptSlotMethod_PerMethodTagsCLIFalse(t *testing.T) {
	svc := &stubScriptSvc{scripts: []scriptsapi.ScriptInfo{
		{Name: "hello", Provider: "ktw-docs"},
	}}
	slot := ktsp.NewSlot("scripts", ktsp.DefaultInstance, "test", nil)
	addPerScriptMethodsForTest(t, slot, svc)

	def := slot.Def("hello")
	if def == nil {
		t.Fatal("hello method missing")
	}
	if got := def.Info().Tags[ktsp.TagMethodCLI]; got != "false" {
		t.Errorf("per-script method should have cli=false tag, got %q", got)
	}
}

func TestPerScriptSlotMethod_MissingMethodNotFound(t *testing.T) {
	svc := &stubScriptSvc{scripts: []scriptsapi.ScriptInfo{
		{Name: "hello", Provider: "ktw-docs"},
	}}
	slot := ktsp.NewSlot("scripts", ktsp.DefaultInstance, "test", nil)
	addPerScriptMethodsForTest(t, slot, svc)

	_, err := slot.Handle(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected ErrMethodNotFound for unknown method")
	}
	// ErrMethodNotFound returns an *RPCError carrying the method name.
	rpcErr, ok := err.(*ktsp.RPCError)
	if !ok {
		t.Errorf("expected *ktsp.RPCError, got %T: %v", err, err)
	} else if rpcErr.Message == "" {
		t.Errorf("expected RPCError to carry a message")
	}
}
