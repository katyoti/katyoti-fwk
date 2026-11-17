// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// PluginDomain is the declared domain for agent plugins.
// Internal — external extensions use api.ContributeStrategy to contribute.
var PluginDomain = strategies.NewDomain[api.PluginFactory]("agent.plugin")

// NewPluginDomainExt returns an Extension that bootstraps the plugin domain.
func NewPluginDomainExt(extraStrategies ...api.PluginStrategyDef) application.Extension {
	return func(init *application.ApplicationInit) {
		// Standard domain lifecycle (register points + provider-based type creation).
		init.AddExtension(PluginDomain.Extension())

		// Contribute extra strategies.
		if len(extraStrategies) > 0 {
			init.AddContribute("agent.plugin", ".extra", func(app *application.Application) error {
				return strategies.ContributeStrategy(app, extraStrategies...)
			})
		}

		// Register PluginManager DI service
		init.AddExtension(PluginManagerExtension)
	}
}
