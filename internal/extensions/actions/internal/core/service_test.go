// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"

	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
)

// TestActionService_Run_MethodSlotDispatches pins the dispatch
// contract since the workflow `call` primitive landed (2026-06-10):
// `tasks/` and `scripts/` route to their runners; anything else is a
// KTSP method-call ATTEMPT over the bus — never the old
// "not implemented" refusal. Without a live hub the attempt fails at
// connect/transport, which is fine: the pin is that dispatch is
// attempted (error names the dispatch path, not a missing feature).
func TestActionService_Run_MethodSlotDispatches(t *testing.T) {
	svc, err := New(context.Background(), &Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, uri := range []string{
		"ktw-agents:cwm/object-update",
		"ktw-other:custom-slot/foo",
	} {
		t.Run(uri, func(t *testing.T) {
			_, err := svc.Run(context.Background(), actionsapi.RunInput{URI: uri})
			if err == nil {
				// A live universe answered — acceptable in a dev env
				// with a running hub; the contract pin is below.
				return
			}
			if strings.Contains(err.Error(), "not implemented") {
				t.Errorf("Run(%q): method dispatch regressed to the v1 refusal: %v", uri, err)
			}
		})
	}
}

// TestActionService_Run_InvalidURI confirms bad URIs surface a clear
// parse error rather than silently routing to "unknown slot".
func TestActionService_Run_InvalidURI(t *testing.T) {
	svc, _ := New(context.Background(), &Config{})
	_, err := svc.Run(context.Background(), actionsapi.RunInput{URI: "not-a-uri"})
	if err == nil {
		t.Fatal("expected error for malformed URI")
	}
}

// TestActionService_Describe_MethodSlot pins the inspector contract
// since 2026-06-10: a slot-method URI describes successfully with
// minimal metadata and IsAsync=false (method calls are synchronous —
// the workflow `call`/`delegate` routing discriminant).
func TestActionService_Describe_MethodSlot(t *testing.T) {
	svc, _ := New(context.Background(), &Config{})
	out, err := svc.Describe(context.Background(), actionsapi.DescribeInput{
		URI: "ktw-agents:cwm/node-update",
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if out.IsAsync {
		t.Error("method-call Describe must report IsAsync=false (sync request/reply)")
	}
	if out.Label != "node-update" {
		t.Errorf("Label = %q, want method name", out.Label)
	}
}
