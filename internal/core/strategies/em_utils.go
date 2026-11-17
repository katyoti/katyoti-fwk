// SPDX-License-Identifier: Apache-2.0

package strategies

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// ContributeStrategy registers StrategyDefs via the Application's EM.
// The EM routes to the correct domain point based on the StrategyDef type parameter.
func ContributeStrategy[TType any](app *application.Application, defs ...StrategyDef[TType]) error {
	for _, def := range defs {
		if err := application.Contribute(app, def); err != nil {
			return err
		}
	}
	return nil
}

// ContributeType registers TypeDefs via the Application's EM.
// The EM routes to the correct domain point based on the TypeDef type parameter.
func ContributeType[TType any](app *application.Application, defs ...TypeDef[TType]) error {
	for _, td := range defs {
		if err := application.Contribute(app, td); err != nil {
			return err
		}
	}
	return nil
}

// ContributeProvider registers ProviderDefs via the Application's EM.
// The ProviderDef type parameter routes the contribution to the correct domain.
// Domain packages should provide a typed wrapper (see agentapi.ContributeProvider).
func ContributeProvider[TType any](app *application.Application, providers ...ProviderDef[TType]) error {
	for _, dp := range providers {
		if err := application.Contribute(app, dp); err != nil {
			return err
		}
	}
	return nil
}
