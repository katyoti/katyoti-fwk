// SPDX-License-Identifier: Apache-2.0

// Package local wires the slide-deck engine into a host binary: its CLI and
// its service (the lint hook of a slide-deck object). The host publishes the
// built-in layouts and the CWM pack itself, through the contrib package, and
// ships the kind declaration and the data-types in its own pack.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/commands"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/internal/core"
)

// Extension adds the `deck` command group (lint, layouts) and publishes the
// DeckService (`ktw-docs:deck/lint-object`).
func Extension() application.Extension {
	cli := appcore.CommandExt(commands.NewDeckCommand())
	service := publish.PublishSimpleService[deckapi.DeckService](
		func(context.Context) (deckapi.DeckService, error) { return core.Service{}, nil },
		publish.WithServiceDef(deckapi.DeckServiceDef),
		publish.WithKTSPContainer(),
	)
	return func(init *application.ApplicationInit) {
		cli(init)
		service(init)
	}
}
