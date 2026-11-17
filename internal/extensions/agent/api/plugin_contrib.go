// SPDX-License-Identifier: Apache-2.0

package api

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/provider"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
)

// ContributeStrategy is the public API for contributing plugin strategies.
// External extensions call this during their Contribute phase.
// Routing is type-based via EM — no direct domain reference needed.
func ContributePluginStrategy(app *application.Application, defs ...PluginStrategyDef) error {
	return strategies.ContributeStrategy(app, defs...)
}

// ContributeProvider is the public API for contributing config providers
// to the agent plugin domain.
func ContributePluginProvider(app *application.Application, providers ...provider.ConfigProvider) error {
	dps := make([]strategies.ProviderDef[PluginFactory], len(providers))
	for i, p := range providers {
		dps[i] = strategies.ProviderDef[PluginFactory]{Provider: p}
	}
	return strategies.ContributeProvider(app, dps...)
}
