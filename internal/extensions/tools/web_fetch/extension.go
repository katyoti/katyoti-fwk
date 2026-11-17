// SPDX-License-Identifier: Apache-2.0

package web_fetch

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// Extension contributes the web_fetch tool as a standalone agent plugin strategy.
func Extension(init *application.ApplicationInit) {
	init.AddContribute("web_fetch-tool", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, toolsapi.ToolExtension(NewWebFetchTool()))
	})
}
