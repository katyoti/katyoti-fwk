// SPDX-License-Identifier: Apache-2.0

package main

import (
	"sort"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// DeckSlideGroups is the slides of each slide group a deck includes (th22),
// by name: the project's groups (slide-group objects) first, then the
// sidecars'. The deck view shows them in place of their include slides; a
// group not in reach is left out and the view says where it goes.
//
// Wails-bound: window.go.main.App.DeckSlideGroups(universeName, markdown).
func (a *App) DeckSlideGroups(universeName, markdown string) (map[string]string, error) {
	if len(deckapi.Includes(deckapi.Parse(markdown))) == 0 {
		return map[string]string{}, nil
	}
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		return nil, err
	}
	return deckapi.GroupsFor(a.deckThemeCtx(universeName), mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}, markdown)
}

// DeckSlideGroupInfo is one slide group offered by « Insérer un groupe de
// slides »: its name (what include= writes), what it is for, who provides it
// (ProviderProject for a slide-group object) and its slides, for the preview.
type DeckSlideGroupInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Provider    string `json:"provider"`
	Slides      string `json:"slides"`
}

// DeckSlideGroupList is every slide group in reach, the project's first, then
// the sidecars', each by name.
//
// Wails-bound: window.go.main.App.DeckSlideGroupList(universeName).
func (a *App) DeckSlideGroupList(universeName string) ([]DeckSlideGroupInfo, error) {
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		return nil, err
	}
	groups, err := deckapi.SlideGroups(a.deckThemeCtx(universeName), mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
	if err != nil {
		return nil, err
	}
	return slideGroupInfos(groups), nil
}

func slideGroupInfos(groups []mbapi.Attributed[deckapi.SlideGroupBody]) []DeckSlideGroupInfo {
	out := make([]DeckSlideGroupInfo, 0, len(groups))
	for _, g := range groups {
		out = append(out, DeckSlideGroupInfo{Name: g.Body.Name, Description: g.Body.Description, Provider: g.Provider, Slides: g.Body.Slides})
	}
	project := func(i int) bool { return out[i].Provider == deckapi.ProviderProject }
	sort.SliceStable(out, func(i, j int) bool { return project(i) && !project(j) })
	return out
}
