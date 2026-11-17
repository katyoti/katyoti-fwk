// SPDX-License-Identifier: Apache-2.0

package application

import "fmt"

// ErrPhaseViolation is returned when an operation is attempted outside its
// allowed phase.
type ErrPhaseViolation struct {
	Operation    string
	CurrentPhase Phase
	AllowedPhase string
}

func (e ErrPhaseViolation) Error() string {
	return fmt.Sprintf("application: %s not allowed in phase %s (allowed: %s)",
		e.Operation, e.CurrentPhase, e.AllowedPhase)
}
