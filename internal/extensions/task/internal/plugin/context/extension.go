// SPDX-License-Identifier: Apache-2.0

// Package context provides the task context injection extension.
package context

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Extension is an Extension that contributes the context plugin (system message injection).
func Extension(init *application.ApplicationInit) {
	init.AddContribute("context", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, StrategyDef)
	})
}
