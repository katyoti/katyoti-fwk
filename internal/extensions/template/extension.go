// SPDX-License-Identifier: Apache-2.0

// Package template provides the template rendering extension.
// Registers both a plugin (render-template tool) and a machine strategy
// (template step in workflows).
package template

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Extension is an Extension that contributes the template plugin and machine strategy.
func Extension(init *application.ApplicationInit) {
	init.AddContribute("template", "", func(app *application.Application) error {
		if err := agentapi.ContributePluginStrategy(app, PluginStrategyDef); err != nil {
			return err
		}
		return strategies.ContributeStrategy(app, MachineStrategyDef)
	})
}
