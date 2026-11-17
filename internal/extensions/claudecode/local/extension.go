// SPDX-License-Identifier: Apache-2.0

// Package local wires the Claude Code driver: the published service and the
// foreground run command.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/commands"
	"github.com/katyoti/katyoti-fwk/internal/extensions/claudecode/internal/core"
)

// Extension publishes the Claude Code service (ClaudeCode.yaml) and adds the
// run command.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishService[api.ClaudeCode, core.ClaudeCodeConfig](
			newService,
			appcore.WithServiceDef(api.ServiceDef),
			appcore.WithKTSPContainer(),
		))
		init.AddExtension(appcore.CommandExt(commands.NewRunCommand()))
	}
}

func newService(_ context.Context, cfg *core.ClaudeCodeConfig) (api.ClaudeCode, error) {
	c := core.ClaudeCodeConfig{}
	if cfg != nil {
		c = *cfg
	}
	return core.New(c), nil
}
