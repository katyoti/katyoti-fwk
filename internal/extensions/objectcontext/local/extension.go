// SPDX-License-Identifier: Apache-2.0

// Package local wires the object-context service into a binary
// (`<provider>:object-context/...`, configured by ObjectContextService.yaml) and its
// agent plugin `object-context`, which the agent factory attaches to every run
// that has a target object.
package local

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	occore "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/internal/core"
)

// Extension publishes the ObjectContextService over KTSP, and reads the
// declarations again when the catalog's capabilities, policies or
// agent-contexts change.
func Extension() application.Extension {
	publishSvc := publish.PublishService[api.ObjectContextService, api.ObjectContextConfig](
		func(_ context.Context, cfg *api.ObjectContextConfig) (api.ObjectContextService, error) {
			if cfg == nil {
				cfg = &api.ObjectContextConfig{}
			}
			return occore.New(*cfg), nil
		},
		publish.WithServiceDef(api.ObjectContextServiceDef),
		publish.WithKTSPContainer(),
	)
	refresh := manifestlocal.ContributeRefreshHook(mbapi.RefreshHook{
		Name:  "object-context",
		Kinds: []string{"capability", "policy", "agent-context"},
		Refresh: func(ctx context.Context, _ contribapi.ContributionsChangedEvent) error {
			svc, err := application.UseService[api.ObjectContextService](ctx)
			if err != nil {
				return fmt.Errorf("object-context refresh: %w", err)
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

// ExtensionWith publishes the service over a given config instead of
// ObjectContextService.yaml.
func ExtensionWith(cfg api.ObjectContextConfig) application.Extension {
	return publish.PublishSimpleService[api.ObjectContextService](
		func(context.Context) (api.ObjectContextService, error) { return occore.New(cfg), nil },
		publish.WithServiceDef(api.ObjectContextServiceDef),
		publish.WithKTSPContainer(),
	)
}
