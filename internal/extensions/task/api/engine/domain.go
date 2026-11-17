// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
)

// MachineDomain is the declared domain for machine strategies.
// Extensions contribute machine types: agent-loop, workflow, program, etc.
// The Passthrough machine is built-in (registered at engine init).
//
// 3-level pattern:
//
//	Level 1: Strategy (code, contributed)  → "agent-loop", "workflow"
//	Level 2: MachineFactory (configured)   → creates machines from requests
//	Level 3: Machine (runtime instance)    → runs via Executor
var MachineDomain = strategies.NewDomain[MachineFactory]("machine")

// MachineStrategyDef is the strategy definition type for the machine domain.
type MachineStrategyDef = strategies.StrategyDef[MachineFactory]

// LookupFactory resolves a MachineFactory by type name from MachineDomain.
// Returns an error if the type is not registered or has no implementation.
func LookupFactory(typeName string) (MachineFactory, error) {
	td := MachineDomain.GetTypeDef(typeName)
	if td.Impl == nil {
		return nil, fmt.Errorf("no factory for machine type %q", typeName)
	}
	return td.Impl, nil
}
