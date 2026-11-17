// SPDX-License-Identifier: Apache-2.0

package local

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

func TestNATSSubCommand_ContractShape(t *testing.T) {
	ep := ktsp.NewEndpoint("test", "", "")
	cmd := NewNATSSubCommand(ep)
	if cmd.Use != "nats" {
		t.Errorf("Use = %q, want nats", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("Short is empty — should describe the command")
	}
	if cmd.RunE == nil {
		t.Error("RunE is nil — command must be runnable")
	}
}

// Resolution behaviour (lockfile auto-discovery, manifest-based universe,
// "local" tenant default) is exercised by the end-to-end smoke harness
// rather than unit tests, because it requires a populated
// `application.Env()` from `cenv` — wiring that up in-test couples to
// preflight machinery without testing anything load-bearing. The
// command-shape test above is the unit-level surface; the end-to-end
// smoke (ktw-hub serve + ktw-docs sc ktsp nats) covers behaviour.
