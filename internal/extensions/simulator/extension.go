// SPDX-License-Identifier: Apache-2.0

package simulator

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
)

// Extension contributes the simulator machine strategy to MachineDomain.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("simulator", "", func(app *application.Application) error {
			return strategies.ContributeStrategy(app, MachineStrategyDef)
		})
	}
}
