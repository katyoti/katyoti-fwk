// SPDX-License-Identifier: Apache-2.0

// Package local wires the demo sign-off service into a binary
// (`<provider>:signoff/...`) — ktw-demo, whose pack (app/ktw-demo/embed/cwm)
// names its methods. It needs a CWMService in the binary: in a sidecar, the
// bus client of the CWM ktw-agents hosts (cwm/remote).
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/demosignoff/api"
	signoffcore "github.com/katyoti/katyoti-fwk/internal/extensions/demosignoff/internal/core"
)

// Extension publishes the sign-off service over KTSP (and its CLI), as the
// `signoff` slot: panels, acts, the function and the hooks CWM asks.
func Extension() application.Extension {
	svc := signoffcore.New()
	return publish.PublishSimpleService[api.SignoffService](
		func(context.Context) (api.SignoffService, error) { return signoffSlot{svc}, nil },
		publish.WithServiceDef(api.SignoffServiceDef),
		publish.WithKTSPContainer(),
	)
}

// signoffSlot narrows the implementation to the slot's methods: a slot
// publishes the methods of the value it is given.
type signoffSlot struct{ api.SignoffService }
