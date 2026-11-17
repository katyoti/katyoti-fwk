// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	deckdisplay "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/display"
)

// DeckThemePublish makes the deck theme `name` of a universe available to the
// deck widget: it resolves the theme (its declaration and every file it names,
// one layer each through the hub catalog) and posts it to the display
// server's theme store, where the deck page reads it by name
// (modules/slidedeck/, app/ktw-display/src/theme-store.ts). Returns the
// theme's version; UIDeck hands it to the page so it reads the theme again.
//
// The display server never fetches, and a whole theme is larger than one
// KTSP message: the host resolves file by file and posts once, Go to the
// display server over localhost.
//
// Wails-bound: window.go.main.App.DeckThemePublish(universeName, name).
func (a *App) DeckThemePublish(universeName, name string) (string, error) {
	if name == "" || name == deckapi.DefaultTheme {
		return "", nil // the component's own defaults: nothing to publish
	}
	widget, err := a.GetWidgetURL(universeName, "deck", "", "", "")
	if err != nil {
		return "", err
	}
	u, err := url.Parse(widget)
	if err != nil {
		return "", fmt.Errorf("widget url %q: %w", widget, err)
	}
	origin, token := u.Scheme+"://"+u.Host, u.Query().Get("token")
	ctx := a.deckThemeCtx(universeName)
	// A sidecar's theme is cached a while, per display server (a restarted
	// one holds nothing and is posted again); a project theme never is — it
	// is edited on its pane, and it hides a sidecar's theme of its name.
	key := universeName + "\x00" + origin + "\x00" + token + "\x00" + name
	project := deckapi.IsProjectTheme(ctx, name)
	if v, ok := deckThemesPublished.Load(key); ok && !project {
		if p := v.(publishedTheme); time.Since(p.at) < deckThemeFresh {
			return p.version, nil
		}
	}
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		return "", fmt.Errorf("cannot reach universe %q: %v", universeName, err)
	}
	theme, err := deckapi.ResolveTheme(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}, name)
	if err != nil {
		return "", err
	}
	version, err := deckdisplay.PublishTheme(a.ctx, http.DefaultClient, origin, token, theme)
	if err != nil {
		return "", err
	}
	if theme.Provider != deckapi.ProviderProject {
		deckThemesPublished.Store(key, publishedTheme{version: version, at: time.Now()})
	}
	return version, nil
}

// A theme changes when a sidecar is updated, not while a deck is being
// written: a published version is trusted this long before it is resolved
// again.
const deckThemeFresh = 2 * time.Minute

type publishedTheme struct {
	version string
	at      time.Time
}

var deckThemesPublished sync.Map
