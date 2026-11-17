// SPDX-License-Identifier: Apache-2.0

package application

// Phase represents a bootstrap phase.
type Phase int

const (
	PhaseInit           Phase = iota // Before Run() — registration of extensions
	PhaseRegisterPoints              // Phase 1: extensions declare their extension points
	PhaseContribute                  // Phase 2: extensions contribute items to points
	PhaseAppInit                     // Phase 3: post-contribution init (factory creation)
	PhaseBootstrap                   // Phase 4: lifecycle startup
	PhaseDone                        // After Run() — all phases complete
)

func (p Phase) String() string {
	switch p {
	case PhaseInit:
		return "Init"
	case PhaseRegisterPoints:
		return "RegisterPoints"
	case PhaseContribute:
		return "Contribute"
	case PhaseAppInit:
		return "AppInit"
	case PhaseBootstrap:
		return "Bootstrap"
	case PhaseDone:
		return "Done"
	default:
		return "Unknown"
	}
}
