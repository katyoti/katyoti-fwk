// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/app/katyoti/internal/launcher"
	"github.com/katyoti/katyoti-fwk/app/katyoti/internal/startup"
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	marketplacelocal "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/local"
	orchestratorlocal "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/local"
	profilelocal "github.com/katyoti/katyoti-fwk/internal/extensions/profile/local"
)

// buildExtensions declares the extensions for the katyoti front-door CLI.
// katyoti is a control CLI (marketplace install/configure/update + launcher),
// not a hub sidecar — so KTSP/HTTP exposure is off; the marketplace domain
// contributes the top-level commands + the in-process SetupService.
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName:    "katyoti",
		Version:    version,
		Short:      "katyoti — install / configure / update katyoti (marketplace front door)",
		EmbeddedFS: efs,
	},
		// orchestratorlocal.ServiceExtension: the registry-backed UniverseService
		// registered in-process (no KTSP slot) so marketplace's universe verbs
		// (create/list) delegate to the SINGLE universe owner instead of the
		// identity-only wire path — fixes create-invisible-to-list.
		sidecarkit.WithDomain(marketplacelocal.Extension(), orchestratorlocal.ServiceExtension(), profilelocal.CommandExtension(), startup.Extension(), launcher.Extension()),
		sidecarkit.WithoutHTTPServer(),
		sidecarkit.WithoutKTSP(),
	)
}
