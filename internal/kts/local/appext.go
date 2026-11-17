// SPDX-License-Identifier: Apache-2.0

package local

import (
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/cli"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/spf13/cobra"
)

// AddKTSPCommands adds KTSP-related commands (extensions, serve, CLI) to the root
// cobra command if a KTSP Endpoint is registered in the ServiceManager.
// Called from appcore's bootstrap phase.
func AddKTSPCommands(app *application.Application, root *cobra.Command, cfg KTSPCommandConfig) {
	container, err := application.AppUseService[*ktsp.Endpoint](app)
	if err != nil {
		return // no KTSP container registered — skip
	}

	if cfg.ExtensionsCmd != nil {
		root.AddCommand(cfg.ExtensionsCmd(container))
	}
	if cfg.KTSPDescribeCmd != nil {
		ktspGroup := cfg.KTSPDescribeCmd(container)
		ktspGroup.AddCommand(NewStdioSubCommand(container))
		ktspGroup.AddCommand(NewNATSSubCommand(container))
		root.AddCommand(ktspGroup)
	} else {
		root.AddCommand(NewFallbackKTSPCommand(container))
	}
	cli.GenerateCLI(root, container)
}

// KTSPCommandConfig provides the extensions command builder.
// This avoids a circular dependency: appcore builds the extensions command,
// kts/local adds it to the root.
type KTSPCommandConfig struct {
	ExtensionsCmd   func(container *ktsp.Endpoint) *cobra.Command
	KTSPDescribeCmd func(container *ktsp.Endpoint) *cobra.Command
}

// KTSPExt returns an Extension that puts the KTSP container in the ServiceManager.
func KTSPExt(container *ktsp.Endpoint) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddAppInit("kts", ".ktsp", func(app *application.Application) error {
			application.PutService[*ktsp.Endpoint](app, "default", container)
			return nil
		})
	}
}
