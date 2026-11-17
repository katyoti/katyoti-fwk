// SPDX-License-Identifier: Apache-2.0

// Package api is the `universe-view` slot: what a universe's GUI shows at a
// display level, resolved from the universe's tags - the `universe-tag`
// contributions its member packs declare - and its Univers.yaml adjustments.
// Presentation only: no permission, and agents are unaffected.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// TagKind is the contribution kind a pack declares a universe tag with.
const TagKind = "universe-tag"

// The display levels, lowest first: an item seen at a level is seen at every
// level above it.
const (
	LevelEssentiel = "essentiel"
	LevelStandard  = "standard"
	LevelExpert    = "expert"
)

// Levels are the display levels in order, lowest first.
var Levels = []string{LevelEssentiel, LevelStandard, LevelExpert}

// TagItem is one element a tag allows: by its element id `<place>:<name>`
// , and the lowest display level that sees it (essentiel when
// empty).
type TagItem struct {
	ID    string `json:"id" yaml:"id" jsonschema:"title=Element,description=The element's id <place>:<name>: rail:chat; home:actions or home:<global panel stem>; quick:<name> or quick:task/<provider>/<task>; new:<object type>; tab:<panel stem>"`
	Level string `json:"level,omitempty" yaml:"level,omitempty" jsonschema:"title=Lowest level,description=The lowest display level that sees it: essentiel (default); standard or expert"`
}

// UniverseTag is what a pack declares a tag with: the elements a universe
// carrying it shows - one list of element ids, the place in each id - each
// with its lowest level, and the task hashtags listed first. The one source
// of the `universe-tag` data-type.
type UniverseTag struct {
	Label       string    `json:"label,omitempty" yaml:"label,omitempty" jsonschema:"title=Label,description=The tag as a person reads it"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"title=Description,description=What a universe carrying it is for"`
	Elements    []TagItem `json:"elements,omitempty" yaml:"elements,omitempty" jsonschema:"title=Elements,description=The GUI elements shown - by element id - each with the lowest display level that sees it"`
	First       []string  `json:"first,omitempty" yaml:"first,omitempty" jsonschema:"title=Priority tags,description=Task hashtags listed first in every task list - in order"`
}

// ServiceDef describes the `universe-view` slot.
var ServiceDef = meta.ServiceDef{
	Name:    "universe-view",
	Short:   "What this universe's GUI shows at a display level, resolved from its tags",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"ResolveView": {Short: "The view at a display level: the rail, Home panels, quick actions, + New types and object tabs the universe's tags allow, adjusted by its Univers.yaml; everything when it has no tag", ReadOnly: true},
	},
}

// UniverseViewService resolves a universe's view (named for itself: services are keyed by type name, and comms has its own Service).
type UniverseViewService interface {
	ResolveView(ctx context.Context, in ResolveViewInput) (View, error)
}

// ResolveViewInput names the display level; the universe is the one served.
type ResolveViewInput struct {
	Level string `json:"level,omitempty" jsonschema:"description=essentiel (default) or standard or expert - the person's display level"`
}

// View is what the GUI shows at a level. All means the universe has no tag:
// everything is shown, at every level, and the lists are empty.
type View struct {
	Level   string   `json:"level"`
	All     bool     `json:"all" jsonschema:"description=No tag: everything is shown"`
	Tags    []string `json:"tags,omitempty"`
	Unknown []string `json:"unknown,omitempty" jsonschema:"description=Tags of Univers.yaml no member declares"`
	Allowed []string `json:"allowed" jsonschema:"description=The element ids shown at the level"`
	// RetiredHide is what the universe's gui.hide names: gui.hide is retired
	// - its tags alone decide - so a universe still declaring it is said,
	// never guessed.
	RetiredHide []string `json:"retired_hide,omitempty" jsonschema:"description=The ids gui.hide names that the view no longer applies: gui.hide is retired and the universe's tags decide"`
	First       []string `json:"first"`
}
