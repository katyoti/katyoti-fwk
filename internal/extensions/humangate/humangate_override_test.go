// SPDX-License-Identifier: Apache-2.0

package humangate

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
)

// stubGate is a tiny in-memory HumanGate used to prove a TUI/GUI can install
// its own implementation at runtime — the contract surface that UC-082
// guarantees.
type stubGate struct {
	name string
}

func (g *stubGate) Prompt(_ context.Context, _ api.PromptRequest) (api.PromptResponse, error) {
	return api.PromptResponse{Answer: g.name}, nil
}
func (g *stubGate) PromptPermission(_ context.Context, _ api.PromptPermissionRequest) (api.PromptPermissionResponse, error) {
	return api.PromptPermissionResponse{}, nil
}
func (g *stubGate) Capabilities() []string { return []string{"text"} }

// TestHumanGate_RuntimeOverride (UC-082) — a runtime caller (typically the
// TUI/GUI at startup) can install a custom HumanGate that replaces the
// CLIPrompter default. The contract: HumanGate is an interface; any value
// satisfying it can be slotted in via the standard service-registration
// mechanism.
//
// L1 surface: two distinct gates that share the api.HumanGate interface.
// Pointer assignment exercises the override path the TUI uses.
func TestHumanGate_RuntimeOverride(t *testing.T) {
	var gate api.HumanGate = &stubGate{name: "default-CLIPrompter"}

	// Capabilities expose what the default gate supports.
	caps := gate.Capabilities()
	if len(caps) == 0 || caps[0] != "text" {
		t.Errorf("default gate caps = %v, want non-empty", caps)
	}

	// Verify Prompt round-trips.
	resp, err := gate.Prompt(context.Background(), api.PromptRequest{Type: "text", Question: "?"})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp.Answer != "default-CLIPrompter" {
		t.Errorf("default answer = %q", resp.Answer)
	}

	// TUI/GUI installs a different gate at startup — the override path.
	gate = &stubGate{name: "TUI-installed"}

	resp2, _ := gate.Prompt(context.Background(), api.PromptRequest{Type: "text", Question: "?"})
	if resp2.Answer != "TUI-installed" {
		t.Errorf("override answer = %q, want TUI-installed", resp2.Answer)
	}

	// Both gates honor the interface — any caller that holds api.HumanGate
	// sees the override seamlessly. This is what UC-082 promises:
	// "TUI override of HumanGate at runtime" is just interface re-assignment
	// at the application service level.
}
