// SPDX-License-Identifier: Apache-2.0

// Package workspace provides the workspace file I/O extension.
package workspace

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Extension is an Extension that contributes the workspace plugin (file I/O tools).
func Extension(init *application.ApplicationInit) {
	init.AddContribute("workspace", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, StrategyDef)
	})
}
