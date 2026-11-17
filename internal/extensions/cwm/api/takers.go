// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
)

// THE HANDOVER OF MATCHES: an extension takes, in process, the matches CWM
// records for the asynchronous subscriptions (MatchTaker) — the events
// extension puts a reaction per match in the module's queue. The point is part
// of CWM's contract: a taker declares itself here without importing CWM's
// wiring, and the service factory (cwm/local) attaches every taker once the
// application has booted; CWM imports no taker.

// TakeMatches plugs a taker into CWM's matches: the CWM service attaches it in
// the process that holds the workspace, and hands it every pending match then
// each new one, in the background. A binary that does not host CWM never runs
// it.
func TakeMatches(t MatchTaker) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddRegisterPoints("contribution", ".cwm.match-takers.point", func(app *application.Application) error {
			err := application.RegisterPoint(app, em.NewListPoint[MatchTaker]())
			var dup em.ErrDuplicatePointID
			if errors.As(err, &dup) {
				return nil // another taker of this binary registered it first
			}
			return err
		})
		init.AddContribute("contribution", ".cwm.match-takers."+t.Name, func(app *application.Application) error {
			return application.Contribute(app, t)
		})
	}
}
