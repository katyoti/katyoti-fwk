// SPDX-License-Identifier: Apache-2.0

// Package agentplugin registers the skill-access agent plugin.
//
// It is a package of its own, not part of the skills wiring, because only a
// binary that runs the agent loop needs it — and package, not function, is what
// the linker sees. Keeping it beside the CLI wiring made every sidecar link the
// agent plugin surface for a capability it can never use.
package agentplugin

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	skillplugin "github.com/katyoti/katyoti-fwk/internal/extensions/skills/internal/plugin"
)

// Extension registers the `skill_access` agent plugin strategy. Add it only in a
// binary that includes the agent loop: elsewhere the agent's PluginFactory
// extension point does not exist and the contribution fails at AppInit.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("skills", ".plugin", func(app *application.Application) error {
			return skillplugin.ContributeStrategy(app)
		})
	}
}
