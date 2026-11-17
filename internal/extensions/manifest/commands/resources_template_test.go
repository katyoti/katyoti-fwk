// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/console"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// TestContribGetTemplate_RendersARealContribution: Go templates are not
// type-checked, so a field removed from the envelope (Body, e13) left
// `sc resources get` exiting 1 on every binary while JSON output kept
// working (re-audit N19, 2026-09-25). Rendering the template against a real
// Contribution in raw mode is the check the compiler cannot do.
func TestContribGetTemplate_RendersARealContribution(t *testing.T) {
	var c contribapi.Contribution
	c.Provider, c.Type, c.ID = "ktw-x", "skill", "one"
	c.SetData(json.RawMessage(`{"name":"one"}`))
	out := mbapi.ContributionGetOutput{Contribution: c}
	out.Contribution.URI = out.Contribution.ResolvedURI()
	if err := console.RenderConsole(console.OutputRaw, "contributions-get", out, contribGetTemplate); err != nil {
		t.Fatalf("template does not render a real Contribution: %v", err)
	}
	// The index lane carries no inline data above the band: the block is
	// optional and the template must still render.
	out.Contribution.DropInline()
	if err := console.RenderConsole(console.OutputRaw, "contributions-get", out, contribGetTemplate); err != nil {
		t.Fatalf("template does not render a body-free Contribution: %v", err)
	}
}
