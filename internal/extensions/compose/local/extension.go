// SPDX-License-Identifier: Apache-2.0

// Package local wires ComposeService into the ktw-compose binary:
// publishes the service (KTSP + flat CLI via the binary's WithFlatCLI) and
// registers the orchestrator service in-process so the universe verbs
// resolve it via DI — WITHOUT publishing a second orchestrator KTSP slot
// (ktw-univers stays the single bus authority for universe methods).
package local

import (
	"context"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	composecore "github.com/katyoti/katyoti-fwk/internal/extensions/compose/internal/core"
	orchestratorlocal "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/local"
)

// Extension publishes ComposeService. efs is the binary's embedded FS;
// the templates live under embed/templates within it.
func Extension(efs fs.FS) application.Extension {
	factory := func(ctx context.Context, cfg *composecore.ComposeServiceConfig) (api.ComposeService, error) {
		tmplFS, err := fs.Sub(efs, "embed/templates")
		if err != nil {
			return nil, err
		}
		return composecore.New(tmplFS), nil
	}
	return func(init *application.ApplicationInit) {
		// Orchestrator service as a plain DI registration (no KTSP slot
		// here — ktw-univers owns the bus surface for universe methods).
		init.AddExtension(orchestratorlocal.ServiceExtension())

		init.AddExtension(appcore.PublishService[api.ComposeService, composecore.ComposeServiceConfig](
			factory,
			appcore.WithServiceDef(api.ComposeServiceDef),
			appcore.WithKTSPContainer(),
		))
		// `inspect` — one entry of any kind; hand-written for its YAML default.
		init.AddExtension(appcore.CommandExt(newInspectCommand()))
	}
}
