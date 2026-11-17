// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Extension contributes the ledger machine strategy to MachineDomain.
// Mirrors simulator.Extension() — registers a token-free, event-driven machine
// that holds a shared mutable state document for the Magentic pattern.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("ledger", "", func(app *application.Application) error {
			return strategies.ContributeStrategy(app, MachineStrategyDef)
		})
	}
}

// AgentPluginExtension contributes both ledger agent plugin strategies to
// the agent plugin domain:
//
//   - `ledger-worker`  — snapshot + update tools (delegates).
//   - `ledger-manager` — snapshot + update + complete tools, plus an
//     OnResult/OnAbort safety hook that auto-fires
//     LedgerCompleteEvent if the manager finished
//     without calling ledger-complete explicitly.
//
// Mirrors skillslocal.AgentPluginExtension() — only ktw-agents (the binary
// that runs the agent loop) wires this in; sidecars without the agent
// PluginFactory extension point should NOT include it.
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("ledger-worker", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, WorkerPluginStrategyDef)
		})
		init.AddContribute("ledger-manager", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, ManagerPluginStrategyDef)
		})
	}
}
