// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Extension contributes the bash tool as a standalone agent plugin strategy.
func Extension(init *application.ApplicationInit) {
	init.AddContribute("bash-tool", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, StrategyDef)
	})
}
