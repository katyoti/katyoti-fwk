// SPDX-License-Identifier: Apache-2.0

package local

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/plugin"
)

// AgentPluginExtension contributes the `cwm` PluginFactory strategy (the cwm-*
// agent tools, built in cwm/internal/plugin).
// Only ktw-agents (and other binaries that include the agent loop)
// should add this. Sidecars that only expose the CLI surface MUST
// NOT add it — the agent's PluginFactory extension point doesn't
// exist there and the contribution would fail at AppInit.
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("cwm", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, plugin.Strategy)
		})
	}
}
