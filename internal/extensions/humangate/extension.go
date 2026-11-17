// SPDX-License-Identifier: Apache-2.0

// Package humangate provides the user interaction extension.
// Registers both a plugin (ask_user tool) and a machine strategy
// (human-gate step in workflows).
package humangate

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
)

// Extension contributes the human-gate machine strategy to the MachineDomain.
// The ask_user agent tool is now registered separately via
// tools/ask_user.Extension (standalone tool with auto-generated schema,
// proper defaults, and permission-rules integration).
func Extension(init *application.ApplicationInit) {
	init.AddContribute("humangate", "", func(app *application.Application) error {
		return strategies.ContributeStrategy(app, MachineStrategyDef)
	})
}
