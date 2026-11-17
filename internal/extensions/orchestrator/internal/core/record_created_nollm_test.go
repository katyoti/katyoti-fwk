// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// : recording a folder as created on this PC lets it join a
// universe beside other members past guard, so no agent may
// call it. The orchestrator slot publishes record-created-project on KTSP (the
// marketplace calls it) tagged mcp=false, which keeps it off the MCP tools,
// the sidecar agent plugin, `llm tools` and the toolproxy.
func TestRecordCreatedProjectIsNotAnAgentTool(t *testing.T) {
	slot := ktslocal.BuildKTSPSlotWithTags("orchestrator", ktsp.DefaultInstance, "management",
		&Service{}, nil, api.OrchestratorServiceDef.Methods)
	// forget-created-project, the record's other half, is
	// kept off the same surfaces.
	for _, name := range []string{"record-created-project", "forget-created-project"} {
		def := slot.Def(name)
		if def == nil {
			t.Fatalf("%s is not published on KTSP: %v", name, slot.Methods())
		}
		if got := def.Opts().Tags[ktsp.TagMethodMCP]; got != "false" {
			t.Errorf("%s carries mcp=%q; want \"false\" (an agent could change what the record vouches for)", name, got)
		}
	}
}
