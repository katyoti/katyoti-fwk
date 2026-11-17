// SPDX-License-Identifier: Apache-2.0

package api

// Slide groups (th22, th21 D4): slides a team keeps once and shows in many
// decks — « our team », « the company context », « architecture principles ».
// A deck shows one with a slide whose settings line is only
// `<!-- slide: include=<group> -->`; the group's slides take its place
// wherever the deck is shown or exported, so updating the group updates
// every deck. A group is a `slide-group` contribution (a sidecar ships it) or
// a `slide-group` object of the project (its body the slides), the
// project's first, hiding a contribution of the same name. A parametrised
// group is not included: an agent copies its slides into a deck and fills
// them.

import (
	"context"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api/deckbody"
)

const (
	// ContributionTypeSlideGroup is the kind a sidecar publishes a group as.
	ContributionTypeSlideGroup = "slide-group"
	// ObjectTypeSlideGroup is a group kept in the project: its local id is
	// its name, its body the slides.
	ObjectTypeSlideGroup = "slide-group"
)

// SlideGroupBody is a slide group's declaration (deckbody, which the deck
// kinds' leaf reads).
type SlideGroupBody = deckbody.SlideGroupBody

// SlideGroups is every group in reach — the project's first — by name.
func SlideGroups(ctx context.Context, q mbapi.Q) ([]mbapi.Attributed[SlideGroupBody], error) {
	q.Kind = ContributionTypeSlideGroup
	// A catalog that cannot answer (the kind not published yet) leaves the
	// project's groups: they are not the catalog's.
	entries, qerr := mbapi.QueryTyped[SlideGroupBody](ctx, q)
	seen := map[string]bool{}
	var out []mbapi.Attributed[SlideGroupBody]
	if src := themeObjects(ctx); src != nil {
		if objs, err := src.ObjectList(ctx, cwmapi.ObjectListInput{Type: ObjectTypeSlideGroup}); err == nil {
			// A listed object carries no body (the index keeps metadata
			// only): a group's slides are read from the object itself.
			getter, _ := src.(objectGetter)
			for _, o := range objs.Objects {
				if o.Body == "" && getter != nil {
					if m, err := cwmapi.ModuleOfObjectID(o.ID); err == nil {
						if full, err := getter.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: m, ID: o.ID}); err == nil {
							o = full.Object
						}
					}
				}
				g := GroupOfObject(o)
				if !themeName.MatchString(g.Name) || seen[g.Name] {
					continue
				}
				seen[g.Name] = true
				out = append(out, mbapi.Attributed[SlideGroupBody]{Provider: ProviderProject, Body: g})
			}
		}
	}
	for _, e := range entries {
		if !seen[e.Body.Name] {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Body.Name < out[j].Body.Name })
	if qerr != nil && len(out) == 0 {
		return nil, qerr
	}
	return out, nil
}

// objectGetter reads one object in full — its body included. CWMService has
// it; a host's own source (the GUI's) adds it.
type objectGetter interface {
	ObjectGet(ctx context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error)
}

// GroupOfObject is a slide-group object as a group: its local id the name,
// its description, its body the slides.
func GroupOfObject(o cwmapi.Object) SlideGroupBody {
	name := o.ID
	if _, local, err := cwmapi.SplitObjectID(o.ID); err == nil {
		name = local
	}
	return SlideGroupBody{Name: name, Description: o.Description, Slides: o.Body}
}

// Includes are the group names a deck includes, in order, once each.
func Includes(d Deck) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range d.Slides {
		if n := strings.TrimSpace(s.Settings["include"]); n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// GroupsFor is the slides of each group md includes and q reaches — what a
// host hands to the deck view (and to the exports) with the deck. A group
// not in reach is left out: the view shows its place as not found.
func GroupsFor(ctx context.Context, q mbapi.Q, md string) (map[string]string, error) {
	names := Includes(Parse(md))
	out := map[string]string{}
	if len(names) == 0 {
		return out, nil
	}
	groups, err := SlideGroups(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, n := range names {
		for _, g := range groups {
			if g.Body.Name == n {
				out[n] = g.Body.Slides
				break
			}
		}
	}
	return out, nil
}

// InlineGroups is md with each include slide's settings line replaced by the
// group's slides as written — what is sent where a deck leaves the editor
// (an export, the presenting window), which needs no groups. A group not in
// groups stays an include line. The TypeScript inlineGroups is the same rule.
func InlineGroups(md string, groups map[string]string) string {
	if len(groups) == 0 {
		return md
	}
	lines := splitLines(md)
	d := Parse(md)
	for i := len(d.Slides) - 1; i >= 0; i-- {
		s := d.Slides[i]
		g, ok := groups[strings.TrimSpace(s.Settings["include"])]
		if !ok || s.SettingsLine == 0 {
			continue
		}
		at := s.SettingsLine - 1
		repl := strings.Split(strings.TrimSpace(strings.ReplaceAll(g, "\r\n", "\n")), "\n")
		lines = append(lines[:at], append(repl, lines[at+1:]...)...)
	}
	return strings.Join(lines, "\n")
}
