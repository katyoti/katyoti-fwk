// SPDX-License-Identifier: Apache-2.0

package strategies

import "github.com/katyoti/katyoti-fwk/internal/core/services"

// strategyExtractor extracts a services.TypeFactory from a StrategyDef.
// Used by DeclaredDomain.AsTypeFactoryLookup().
func strategyExtractor[TType any](sd StrategyDef[TType]) services.TypeFactory[TType] {
	return sd.Strategy
}
