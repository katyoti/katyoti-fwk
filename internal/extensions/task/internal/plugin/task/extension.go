// SPDX-License-Identifier: Apache-2.0

// Package task provides the complete/delegate tool extension.
package task

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Extension is an Extension that contributes the complete/delegate plugin.
func Extension(init *application.ApplicationInit) {
	init.AddContribute("taskplugin", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, StrategyDef)
	})
}
