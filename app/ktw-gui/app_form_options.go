// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log/slog"

	coreschemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// formOptionSources are the `options_from` sources a form the GUI serves can
// draw its choices from (modules/framework/schema/dsl-content-types.md, "Options from
// a source"): the universe's catalog, its deck themes and its content
// templates (the project's and the sidecars'). A source not listed here
// leaves its field as free text, with a warning.
func formOptionSources(cat mbapi.CatalogReader) map[string]coreschemaapi.OptionSource {
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}
	return map[string]coreschemaapi.OptionSource{
		mbapi.OptionSourceCatalog:            mbapi.CatalogOptions(q),
		deckapi.OptionSourceDeckThemes:       deckapi.ThemeOptions(q),
		docsapi.OptionSourceContentTemplates: docsapi.TemplateOptions(q),
	}
}

// formOptionsCtx is the context a form's options are resolved in for a
// universe: its project's deck themes and content templates (objects) come
// before the sidecars'.
func (a *App) formOptionsCtx(universeName string) context.Context {
	ctx := a.deckThemeCtx(universeName)
	if sender, err := a.universeSender(universeName); err == nil {
		ctx = docsapi.WithTemplateObjects(ctx, senderThemeObjects{sender: sender})
	}
	return ctx
}

// resolveFormOptions fills, in s, the choices of every field that declares an
// `options_from`, from cat. Best effort: a failed field stays free text and
// the form still opens.
func resolveFormOptions(ctx context.Context, s *coreschemaapi.Schema, cat mbapi.CatalogReader) {
	for _, err := range coreschemaapi.ResolveOptions(ctx, s, formOptionSources(cat)) {
		slog.Warn("form options: a field keeps free text", "schema", s.Name, "error", err)
	}
}
