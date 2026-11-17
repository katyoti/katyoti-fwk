// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"context"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Extension contributes the orchestration plugin (team, coordinator, swarm
// tools), and the agent memory written before the wall to the person: its
// acts (ktw-agents:agent-memory/<verb>, no root command) and its questions in
// « À faire ».
func Extension(init *application.ApplicationInit) {
	init.AddContribute("orchestration", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, StrategyDef)
	})
	publish.PublishSimpleService[AgentMemoryService](
		func(context.Context) (AgentMemoryService, error) { return newMemoryService(), nil },
		publish.WithServiceDef(AgentMemoryServiceDef),
		publish.WithKTSPContainer(),
		publish.WithKTSPSlotTags(map[string]string{"cli": "false"}),
	)(init)
	init.AddContribute("orchestration", ".waiting", func(app *application.Application) error {
		if err := application.Contribute(app, MemoryWaitingSource()); err != nil {
			slog.Debug("orchestration: no « À faire » in this binary, quarantined agent memory is not listed there", "err", err)
		}
		return nil
	})
}
