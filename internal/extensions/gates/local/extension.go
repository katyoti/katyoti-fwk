// SPDX-License-Identifier: Apache-2.0

// Package local wires the gates service into a binary (`<provider>:gates/...`).
// Its CWM pack — the `gates` capability, which adds gate(name) to expressions
// — is contrib.Pack. It needs a CWMService and the catalog in the binary.
package local

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	gatescore "github.com/katyoti/katyoti-fwk/internal/extensions/gates/internal/core"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// Extension publishes the GatesService over KTSP, and reads the gates again
// when the catalog's gates change.
func Extension() application.Extension {
	publishSvc := publish.PublishSimpleService[api.GatesService](
		func(context.Context) (api.GatesService, error) { return gatescore.New(), nil },
		publish.WithServiceDef(api.GatesServiceDef),
		publish.WithKTSPContainer(),
	)
	refresh := manifestlocal.ContributeRefreshHook(mbapi.RefreshHook{
		Name:  "gates",
		Kinds: []string{"gate", "gate-requirement"},
		Refresh: func(ctx context.Context, _ contribapi.ContributionsChangedEvent) error {
			svc, err := application.UseService[api.GatesService](ctx)
			if err != nil {
				return fmt.Errorf("gates refresh: %w", err)
			}
			if s, ok := svc.(interface{ Invalidate() }); ok {
				s.Invalidate()
			}
			return nil
		},
	})
	return func(init *application.ApplicationInit) {
		publishSvc(init)
		refresh(init)
	}
}
