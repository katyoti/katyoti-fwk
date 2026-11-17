// SPDX-License-Identifier: Apache-2.0

package application

import (
	"github.com/katyoti/katyoti-fwk/internal/core/em"
)

// RegisterPoint registers a typed extension point with the EM.
// Only allowed during Phase 1 (RegisterPoints).
func RegisterPoint[T any](app *Application, point em.ExtensionPoint[T]) error {
	if app.phase != PhaseRegisterPoints {
		return ErrPhaseViolation{
			Operation:    "RegisterPoint",
			CurrentPhase: app.phase,
			AllowedPhase: PhaseRegisterPoints.String(),
		}
	}
	return em.RegisterPoint(app.em, point)
}

// Contribute adds an item to a registered extension point.
// Allowed during Phase 2 (Contribute) and Phase 3 (AppInit).
func Contribute[T any](app *Application, item T) error {
	if app.phase != PhaseContribute && app.phase != PhaseAppInit {
		return ErrPhaseViolation{
			Operation:    "Contribute",
			CurrentPhase: app.phase,
			AllowedPhase: "Contribute or AppInit",
		}
	}
	return em.Contribute(app.em, item)
}

func UseAppExtensions[T any](app *Application) []T {
	point, err := em.GetPoint[T](app.em)
	if err != nil {
		return nil
	}
	return point.Entries()
}

// PointInfo returns metadata about a registered extension point.
// Returns nil if the point is not found.
func (app *Application) PointInfo(pointID string) *em.PointInfo {
	return app.em.PointInfo(pointID)
}

// PointsInfo returns metadata about all registered extension points.
func (app *Application) PointsInfo() []em.PointInfo {
	return app.em.PointsInfo()
}
