// SPDX-License-Identifier: Apache-2.0

package session_read

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// Extension contributes the session-read tool as a standalone agent plugin
// strategy (plugin type `session-read`). Eval tasks (test-scenario,
// task-eval-run) declare it to read run journals by id.
func Extension(init *application.ApplicationInit) {
	init.AddContribute("session_read-tool", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, toolsapi.ToolExtension(NewSessionReadTool()))
	})
}
