// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ObjectPanelKind is the contribution kind of a declared tab; a pack carries
// them in `object-panels/` (contrib.Kinds).
const ObjectPanelKind = "object-panel"

// A GLOBAL panel (`global: true`) is also a tab of the Global strip, where
// there is no object: its Source is called with the TARGET — empty for every
// module, a module id, or a release id — instead of an object id. Its Filter
// still places it on the objects it admits (a module → called with {id}), so
// one declaration is the same panel at both levels; a global panel with an
// empty filter shows on no object.

// DefaultPanelOrder is the order of a panel that declares none, and of the
// fields tabs derived from capability schemas — so a declared panel can sit
// before (lower) or after (higher) them.
const DefaultPanelOrder = 100

// PanelDecl is an `object-panel` contribution: one tab an object shows
// when the panel's filter admits it. The PANEL says which objects it is for;
// a capability does not list panels — one panel can serve several
// capabilities, and the filter is where that is said once. Declared and read
// by the object pane; a pack carries them in `object-panels/`.
//
// A panel shows AT MOST ONCE on an object, however many ways it matches.
// Tabs are sorted by Order, then Name.
//
// What it shows is Content — a render.Def whose `${...}` strings are resolved
// against the object — or Source, a method called with the object id whose
// result IS a render.Def (with its data). Declared together, Source answers the
// DATA and Content is the display it fills: a page binds it as its `data`, a
// view or a table as its value. That is how a panel over an existing verb is
// pure declaration — no method written only to shape a display.
type PanelDecl struct {
	// ID is the contribution id (the file's basename). Set by the loader,
	// never authored in the body.
	ID string `json:"-" yaml:"-"`

	Name        string                       `json:"name"                  yaml:"name"                  jsonschema:"description=Tab label; a name carrying ${...} is evaluated over the object like the content (empty: the panel id)"`
	Description string                       `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"description=Tooltip on the tab"`
	Icon        *contribapi.ContributionIcon `json:"icon,omitempty"        yaml:"icon,omitempty"        jsonschema:"description=Tab icon: a tabler name for the GUI and an emoji for text surfaces"`
	Order       int                          `json:"order,omitempty"       yaml:"order,omitempty"       jsonschema:"description=Sort key; lower first; 100 when absent"`
	Filter      cwmapi.ObjectFilter          `json:"filter"                yaml:"filter"                jsonschema:"description=Which objects show this panel"`
	Content     *render.Def                  `json:"content,omitempty"     yaml:"content,omitempty"     jsonschema:"description=What the panel displays; string values may use ${...} over the object"`
	Source      string                       `json:"source,omitempty"      yaml:"source,omitempty"      jsonschema:"description=Action URI called with the object id: its result is the display definition and its data; or when content is declared the data that content binds"`
	Header      bool                         `json:"header,omitempty"      yaml:"header,omitempty"      jsonschema:"description=The object's header instead of a tab: drawn above its tabs (its title - chips - a stepper of its workflow - its next action) on every object it matches"`
	Global      bool                         `json:"global,omitempty"      yaml:"global,omitempty"      jsonschema:"description=Also a tab of the Global strip: there its source is called with the target (empty for every module or a module id or a release id) instead of an object id; with an empty filter it shows on no object"`
	Targets     string                       `json:"targets,omitempty"     yaml:"targets,omitempty"     jsonschema:"description=Action URI answering the targets a global panel offers ({targets: [{value; label; group}]}); empty: every module"`
	Count       string                       `json:"count,omitempty"       yaml:"count,omitempty"       jsonschema:"description=Action URI answering a count {count: n; color?: tone}: on an object's tab called with {id} and the panel's input under it and drawn as the tab's badge; on the Global strip called with the target and shown beside the panel's name; none when 0"`
	Refresh     *PanelRefresh                `json:"refresh,omitempty"     yaml:"refresh,omitempty"     jsonschema:"description=Which changes to objects re-read the panel while it is shown; absent: the module for a panel with a source and none for static content"`
	// Badge is a badge on the panel's tab read from the object as the
	// header chips read theirs: a study's « Sections 2 »
	// in red.
	Badge *cwmapi.BadgeDef `json:"badge,omitempty" yaml:"badge,omitempty" jsonschema:"description=A badge on the panel's tab read from the object: its field and its colours by value (* for any value); none when the value is empty or 0"`
	// Input is laid under what the source is called with ({id} or {target},
	// the person's language, the page's last query): the panel's own
	// parameters — a search panel's saved views.
	Input map[string]any `json:"input,omitempty" yaml:"input,omitempty" jsonschema:"description=Laid under what the source is called with (the object id or the target and the language and the page's last query): the panel's own parameters"`
	// ReplacesChildren names child types whose children tab this panel
	// stands for: on an object it shows on, those types get no tab of their
	// own (the panel lists them, the way it wants).
	ReplacesChildren []string `json:"replaces_children,omitempty" yaml:"replaces_children,omitempty" jsonschema:"description=Child types whose children tab this panel stands for: on an object it shows on they get no tab of their own"`
	// Empty is what the panel shows when its source answers nothing to show:
	// a sentence and one action inviting the next step.
	Empty *PanelEmpty `json:"empty,omitempty" yaml:"empty,omitempty" jsonschema:"description=What the panel shows when it has nothing to show: a sentence and one action inviting the next step"`
}

// PanelEmpty is a panel's empty state: the sentence a person reads when the
// panel has nothing to show, and the one action that starts filling it.
type PanelEmpty struct {
	Text   string         `json:"text"             yaml:"text"             jsonschema:"description=The sentence shown when the panel has nothing to show"`
	Label  string         `json:"label,omitempty"  yaml:"label,omitempty"  jsonschema:"description=The label of the button that runs the action"`
	Action string         `json:"action,omitempty" yaml:"action,omitempty" jsonschema:"description=The action or task URI the button runs; empty: no button"`
	Input  map[string]any `json:"input,omitempty"  yaml:"input,omitempty"  jsonschema:"description=The input the action is run with"`
}

// The refresh scopes of a panel: which object changes re-read it while it is
// shown.
const (
	RefreshSelf     = "self"     // the panel's object
	RefreshChildren = "children" // the panel's object and the objects directly under it
	RefreshModule   = "module"   // any object of the panel's module (a global panel: of its target module, or every module)
	RefreshUniverse = "universe" // any object of the universe
	RefreshNone     = "none"     // never: the panel re-reads only after its own actions
)

// PanelRefresh says which changes to objects re-read a panel while it is
// shown: the objects of a scope, of some types only when Types is set. The
// host matches each CWM object event (its object, module, type, parent)
// against it and re-reads the panel once a burst of matching events is over.
type PanelRefresh struct {
	Scope string   `json:"scope,omitempty" yaml:"scope,omitempty" jsonschema:"description=Which objects: self (the panel's object) or children (it and the objects directly under it) or module (its module) or universe (any) or none (never),enum=self|children|module|universe|none"`
	Types []string `json:"types,omitempty" yaml:"types,omitempty" jsonschema:"description=Only changes to objects of these types; empty: any type"`
}

// EffectiveRefresh is the declared refresh, its scope defaulted: module for a
// panel with a source (what it reads may change under it), none for static
// content.
func (d PanelDecl) EffectiveRefresh() PanelRefresh {
	var r PanelRefresh
	if d.Refresh != nil {
		r = *d.Refresh
	}
	if r.Scope == "" {
		r.Scope = RefreshNone
		if d.Source != "" {
			r.Scope = RefreshModule
		}
	}
	return r
}

// OnObjects reports whether the panel shows on objects at all: every panel
// but a global one with an empty filter.
func (d PanelDecl) OnObjects() bool {
	f := d.Filter
	return !d.Global || len(f.Types) > 0 || len(f.Capabilities) > 0 || f.When != ""
}

// EffectiveOrder is Order, or DefaultPanelOrder when none is declared.
func (d PanelDecl) EffectiveOrder() int {
	if d.Order == 0 {
		return DefaultPanelOrder
	}
	return d.Order
}

// PaneCapability is the object pane's capability — and the section name a
// type writes its pane settings under. Type-level: no group on objects.
const PaneCapability = "pane"

// PaneSettings is a type's `pane:` section: how an object of the type shows.
// The Go type is the one source of its schema (the `type-pane` data-type).
type PaneSettings struct {
	// View is how an object of the type is shown in its Information tab — a
	// render-def over the object. Empty = a form built from the type's fields.
	View *render.Def `json:"view,omitempty" yaml:"view,omitempty" jsonschema:"description=How an object of this type is shown in its Information tab; empty = a form from its fields"`
	// Badges are the type's header chips, each reading one field and coloured
	// by value; a capability's own badges are declared on the capability.
	Badges []cwmapi.BadgeDef `json:"badges,omitempty" yaml:"badges,omitempty" jsonschema:"description=Header chips: each reads a field and is coloured by its value"`
	// Tabs are the main tabs of every object of the type, in order, before
	// any its aspects list - as an aspect's (the type, not an aspect, says
	// what an object of it shows first).
	Tabs []string `json:"tabs,omitempty" yaml:"tabs,omitempty" jsonschema:"description=The main tabs of every object of this type in order (a panel stem or a full tab id); every other tab goes behind More"`
	// More names the tabs behind « More… », in order:
	// given, every tab neither listed nor in More is hidden, and Information
	// may be one of them.
	More []string `json:"more,omitempty" yaml:"more,omitempty" jsonschema:"description=The tabs behind More in order (a panel stem or a full tab id); given - every tab in neither list is hidden and information may be one"`
	// Variants name the main tabs by condition: the
	// first whose when holds on the object gives them, else Tabs.
	Variants []PaneVariant `json:"variants,omitempty" yaml:"variants,omitempty" jsonschema:"description=Main tabs by condition: the first variant whose when holds on the object gives them; else tabs"`
	// Only makes the listed tabs the whole pane: no other tab, not even
	// behind More - and, unless Header says otherwise, no object header above
	// them: a type whose own panels draw its head and everything it shows
	//
	Only bool `json:"only,omitempty" yaml:"only,omitempty" jsonschema:"description=The listed tabs are the whole pane: no other tab (not even behind More) and no object header unless header is true - for a type whose own panels draw its head"`
	// Header says whether the pane draws the standard object header (icon,
	// type, name, tags, description, actions, capabilities) above its tabs:
	// each type's own option. Unset, the header shows
	// except under Only; true keeps it under Only; false leaves it out.
	Header *bool `json:"header,omitempty" yaml:"header,omitempty" jsonschema:"description=Whether the pane draws the standard object header above its tabs; unset - shown except under only; true - shown even under only; false - never"`
}

// ShowsHeader is whether a type with these settings draws the standard
// object header: Header when written, else not when Only makes its listed
// tabs the whole pane.
func (s PaneSettings) ShowsHeader() bool {
	if s.Header != nil {
		return *s.Header
	}
	return !s.Only || len(s.Tabs) == 0
}

// PaneOf is a type's `pane` section, zero when it writes none.
func PaneOf(t cwmapi.ObjectType) (PaneSettings, error) {
	s, _, err := cwmapi.TypeSection[PaneSettings](t, PaneCapability)
	return s, err
}

// AspectPane is an aspect's `pane:` section: how an object carrying the aspect
// shows. CWM carries the section opaque (cwmapi.Aspect.Pane); the object pane
// types it here.
type AspectPane struct {
	// Tabs are the object's main tabs, in order: each a panel's stem (`tab:<stem>`
	// on the strip) or a full tab id (`tab:children/<type>`). Every
	// other tab goes behind « More… »; a name the strip does not hold is
	// skipped.
	Tabs []string `json:"tabs,omitempty" yaml:"tabs,omitempty"`
	// More names the tabs behind « More… », as a type's.
	More []string `json:"more,omitempty" yaml:"more,omitempty"`
	// Variants name the main tabs by condition, as a type's.
	Variants []PaneVariant `json:"variants,omitempty" yaml:"variants,omitempty"`
}

// PaneVariant is a list of main tabs that holds when its expression is true
// on the object.
type PaneVariant struct {
	When string   `json:"when" yaml:"when" jsonschema:"description=An expression over the object; the variant holds when it is true"`
	Tabs []string `json:"tabs" yaml:"tabs" jsonschema:"description=The main tabs in order while it holds"`
	// More names the tabs behind « More… » while it holds - not the plain
	// pane's.
	More []string `json:"more,omitempty" yaml:"more,omitempty" jsonschema:"description=The tabs behind More in order while it holds; given - every other tab is hidden"`
}

// VariantTabs is the tabs of the first variant that holds, else plain.
func VariantTabs(plain []string, variants []PaneVariant, holds func(string) bool) []string {
	tabs, _ := VariantPane(plain, nil, variants, holds)
	return tabs
}

// VariantPane is the main and « More… » tabs of the first variant that
// holds, else the plain ones.
func VariantPane(plainTabs, plainMore []string, variants []PaneVariant, holds func(string) bool) (tabs, more []string) {
	for _, v := range variants {
		if v.When != "" && holds != nil && holds(v.When) {
			return v.Tabs, v.More
		}
	}
	return plainTabs, plainMore
}

// AspectPaneOf is an aspect's `pane` section, zero when it writes none.
func AspectPaneOf(a cwmapi.Aspect) (AspectPane, error) {
	var p AspectPane
	if len(a.Pane) == 0 {
		return p, nil
	}
	raw, err := json.Marshal(a.Pane)
	if err != nil {
		return p, fmt.Errorf("aspect %q: pane: %w", a.Name, err)
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("aspect %q: pane: %w", a.Name, err)
	}
	return p, nil
}
