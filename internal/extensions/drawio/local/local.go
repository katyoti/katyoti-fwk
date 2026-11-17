// SPDX-License-Identifier: Apache-2.0

// Package local wires the drawio engine into a host binary: the layout
// presets read from the host's embedded FS and the CLI command. The
// host contributes its stencil sets itself, through
// contrib.ContributeStencilSetResources.
package local

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/commands"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/archdiagram"
)

// Extension wires the drawio engine for a host that embeds its ELK
// layout presets under embed/elk-layouts/ of efs.
func Extension(efs embed.FS) application.Extension {
	archdiagram.SetEmbeddedFS(efs)
	return appcore.CommandExt(commands.NewDrawioCommand())
}
