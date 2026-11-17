// SPDX-License-Identifier: Apache-2.0

package local

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// TestResourcesGetTemplate_RendersARealContribution: `llm resources get
// --out rich` exited 1 on every binary after the envelope lost its Body
// field (e13) because Go templates are not type-checked (re-audit N19,
// 2026-09-25). Rendering against a real Contribution is the check.
func TestResourcesGetTemplate_RendersARealContribution(t *testing.T) {
	var c contribapi.Contribution
	c.Provider, c.Type, c.ID = "ktw-x", "skill", "one"
	c.SetData(json.RawMessage(`{"name":"one"}`))
	c.URI = c.ResolvedURI()
	result := api.LLMGetResourceOutput{Contribution: c}
	if err := console.RenderConsole(console.OutputRaw, "llm-resources-get", result, resourcesGetTemplate); err != nil {
		t.Fatalf("template does not render a real Contribution: %v", err)
	}
	result.Contribution.DropInline()
	if err := console.RenderConsole(console.OutputRaw, "llm-resources-get", result, resourcesGetTemplate); err != nil {
		t.Fatalf("template does not render a body-free Contribution: %v", err)
	}
}
