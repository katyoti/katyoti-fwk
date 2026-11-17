// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Grammar is what a deck is checked against beyond its own syntax: the
// layouts and the themes in reach. The lint takes it; GrammarAt builds it.
// A nil list means "not known" — the lint skips the checks it would feed
// rather than guess. ThemesComplete says the themes are every theme there
// is (the universe's): only then is a theme missing from them unknown rather
// than unchecked.
type Grammar struct {
	Layouts        []LayoutBody `json:"layouts"`
	Themes         []ThemeBody  `json:"themes"`
	ThemesComplete bool         `json:"themesComplete"`
	// Groups are the slide groups in reach, by name; GroupsComplete says the
	// list is the universe's, so a name outside it is unknown.
	Groups         []string `json:"groups,omitempty"`
	GroupsComplete bool     `json:"groupsComplete,omitempty"`
}

// Theme is the theme named name, if it is in the grammar.
func (g Grammar) Theme(name string) (ThemeBody, bool) {
	for _, t := range g.Themes {
		if t.Name == name {
			return t, true
		}
	}
	return ThemeBody{}, false
}

// GrammarAt reads the layouts and themes q reaches — the host's own
// (ReachSelf) or every sidecar's (ReachUniverse); a host outside the universe
// passes the hub's Catalog. It is the one place a deck consumer learns them.
func GrammarAt(ctx context.Context, q mbapi.Q) (Grammar, error) {
	layouts, err := Layouts(ctx, q)
	if err != nil {
		return Grammar{}, err
	}
	themes, err := Themes(ctx, q)
	if err != nil {
		return Grammar{}, err
	}
	// Groups are a newer kind than layouts and themes: a universe whose
	// catalog cannot answer for them (a sidecar not yet rebuilt) loses the
	// group check, never the themes.
	g := Grammar{Layouts: layouts, Themes: bodies(themes), ThemesComplete: q.Reach == mbapi.ReachUniverse}
	if groups, err := SlideGroups(ctx, q); err == nil {
		for _, sg := range groups {
			g.Groups = append(g.Groups, sg.Body.Name)
		}
		g.GroupsComplete = q.Reach == mbapi.ReachUniverse
	}
	return g, nil
}

// Themes reads the themes in reach: the project's deck-theme objects first
// (provider `project`; one hides a contribution of the same name), then the
// deck-theme contributions q reaches (its Reach, and its Catalog when the
// caller holds one — a host outside the universe passes the hub's), sorted
// by name, each with the provider that publishes it (and its files).
func Themes(ctx context.Context, q mbapi.Q) ([]mbapi.Attributed[ThemeBody], error) {
	q.Kind = ContributionTypeDeckTheme
	entries, err := mbapi.QueryTyped[ThemeBody](ctx, q)
	if err != nil {
		return nil, err
	}
	entries = withProjectThemes(projectThemes(ctx), entries)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Body.Name < entries[j].Body.Name })
	return entries, nil
}

func bodies(entries []mbapi.Attributed[ThemeBody]) []ThemeBody {
	out := make([]ThemeBody, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Body)
	}
	return out
}

// ResolvedTheme is a theme with the files it names, ready to hand to the deck
// component.
type ResolvedTheme struct {
	Provider string            `json:"provider"`
	Body     ThemeBody         `json:"body"`
	Assets   map[string][]byte `json:"assets"`
}

// ResolveTheme finds the theme named name among those q reaches and reads
// every file it names: a project theme's from its module's themes/<name>/, a
// contribution's from the deck-asset entries its own provider publishes
// beside it — one layer each, each within the control band.
func ResolveTheme(ctx context.Context, q mbapi.Q, name string) (ResolvedTheme, error) {
	for _, p := range projectThemes(ctx) {
		if p.body.Name == name {
			return resolveProjectTheme(ctx, p)
		}
	}
	entries, err := Themes(ctx, q)
	if err != nil {
		return ResolvedTheme{}, err
	}
	for _, e := range entries {
		if e.Body.Name != name {
			continue
		}
		out := ResolvedTheme{Provider: e.Provider, Body: e.Body, Assets: map[string][]byte{}}
		for _, a := range e.Body.Assets() {
			uri, err := contribapi.NewResourceURI(e.Provider, ContributionTypeDeckAsset, name+"/"+a)
			if err != nil {
				return ResolvedTheme{}, fmt.Errorf("theme %s: asset %s: %w", name, a, err)
			}
			l, err := mbapi.Open(ctx, mbapi.Q{Reach: q.Reach, Catalog: q.Catalog}, uri.String(), "source")
			if err != nil {
				return ResolvedTheme{}, fmt.Errorf("theme %s: asset %s: %w", name, a, err)
			}
			b, err := l.Bytes()
			if err != nil {
				return ResolvedTheme{}, fmt.Errorf("theme %s: asset %s: %w", name, a, err)
			}
			out.Assets[a] = b
		}
		return out, nil
	}
	return ResolvedTheme{}, fmt.Errorf("theme %q is not in reach; themes: %v", name, names(entries))
}

// Version identifies the resolved theme's content: a hash of its declaration
// and every file, so a host re-sends a theme only when it changed and a page
// can cache its files for good.
func (r ResolvedTheme) Version() string {
	h := sha256.New()
	body, _ := json.Marshal(r.Body)
	h.Write(body)
	paths := make([]string, 0, len(r.Assets))
	for p := range r.Assets {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(h, "\x00%s\x00%d\x00", p, len(r.Assets[p]))
		h.Write(r.Assets[p])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func names(entries []mbapi.Attributed[ThemeBody]) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Body.Name)
	}
	return out
}
