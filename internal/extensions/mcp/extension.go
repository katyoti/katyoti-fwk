// SPDX-License-Identifier: Apache-2.0

// Package mcp provides the MCP server bridge extension.
package mcp

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Extension is an Extension that contributes the MCP plugin (server bridges).
func Extension(init *application.ApplicationInit) {
	init.AddContribute("mcp", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, StrategyDef)
	})
}
