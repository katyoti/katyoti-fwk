// SPDX-License-Identifier: Apache-2.0

// Package wkflow provides the workflow task extension.
// Contains the multi-step executor, flow control (condition, loop, parallel,
// sub-workflow), step reference resolution, and expression evaluation.
//
// Named "wkflow" to avoid collision with the legacy internal/workflow package.
package wkflow

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
)

// Extension contributes the workflow machine strategy to MachineDomain.
func Extension(init *application.ApplicationInit) {
	init.AddContribute("workflow", "", func(app *application.Application) error {
		return strategies.ContributeStrategy(app, MachineStrategyDef)
	})
}
