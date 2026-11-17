// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
)

// THE REACT CHANNEL: an extension reacts, in process, to CWM's lifecycle events
// (EventObserver). The point is part of CWM's contract: an extension declares
// its observer here without importing CWM's wiring, and the service factory
// (cwm/local) attaches every observer when CWM is created. It advertises
// nothing in the catalog.

// ObserveEvents plugs an in-process reaction into CWM's lifecycle
// events: the CWM service attaches it when it is created, in the process that
// holds the workspace (EventObserver). An extension declares it once; a
// binary that does not host CWM never runs it.
func ObserveEvents(obs EventObserver) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddRegisterPoints("contribution", ".cwm.observers.point", func(app *application.Application) error {
			err := application.RegisterPoint(app, em.NewListPoint[EventObserver]())
			var dup em.ErrDuplicatePointID
			if errors.As(err, &dup) {
				return nil // another observer of this binary registered it first
			}
			return err
		})
		init.AddContribute("contribution", ".cwm.observers."+obs.Name, func(app *application.Application) error {
			return application.Contribute(app, obs)
		})
	}
}
