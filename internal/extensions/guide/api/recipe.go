// SPDX-License-Identifier: Apache-2.0

// Package guideapi is the contract of a guide recipe: the body of a
// `guide-recipe` contribution (modules/guide/concepts.md). A recipe is a small
// YAML file — its file name is its identity — whose ordered steps the GUI's
// player plays over its own controls; a recipe with a `check` ends in a checked
// result, one without is a tour.
//
// The system data-type `system:/data-type/guide-recipe` is rendered from
// Recipe by `go generate ./internal/core/contribution/`, so this struct is
// the one source of the schema. Descriptions live in the jsonschema tags and
// must not contain commas (the tag grammar splits on them).
package guideapi

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
)

// Kind is the contribution kind a recipe is published as.
const Kind = "guide-recipe"

// Levels a recipe declares.
const (
	LevelBeginner = "beginner"
	LevelRegular  = "regular"
	LevelExpert   = "expert"
)

// What completes a step.
const (
	AwaitClick  = "click"
	AwaitRoute  = "route"
	AwaitManual = "manual"
)

// Recipe is the body of a `guide-recipe` contribution.
type Recipe struct {
	Name        string   `json:"name" yaml:"name" jsonschema:"description=The title the picker shows"`
	Description string   `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"description=One sentence saying what the recipe achieves"`
	Hashtags    []string `json:"hashtags,omitempty" yaml:"hashtags,omitempty" jsonschema:"description=Search words: what a person would type to find the recipe"`
	Level       string   `json:"level,omitempty" yaml:"level,omitempty" jsonschema:"description=Who it is written for,enum=beginner|regular|expert"`
	Requires    []string `json:"requires,omitempty" yaml:"requires,omitempty" jsonschema:"description=Recipes to do first by their identity (the file name without extension)"`
	GUI         string   `json:"gui,omitempty" yaml:"gui,omitempty" jsonschema:"description=GUI versions the recipe was written for (>=0.9)"`
	Video       string   `json:"video,omitempty" yaml:"video,omitempty" jsonschema:"description=Link to a video of the recipe opened outside the player"`
	Related     []string `json:"related,omitempty" yaml:"related,omitempty" jsonschema:"description=Resource URIs of what the recipe teaches (skills and tasks and references)"`
	// Next is what to read once the guide is done: guide ids
	// guide URIs, or element addresses of documents - the player ends the guide
	// on « Pour aller plus loin » listing them.
	Next []string `json:"next,omitempty" yaml:"next,omitempty" jsonschema:"description=What to read next: guide ids or guide URIs or element addresses of documents - the last step Pour aller plus loin lists them"`
	// Body is markdown the guide renders: an Explain me's text, or the
	// commentary beside a Show me's video.
	Body string `json:"body,omitempty" yaml:"body,omitempty" jsonschema:"description=Markdown the guide renders: the text of an Explain me or the commentary beside a Show me's video"`
	// Source makes an Explain me whose text is data: the action answering it
	// when it opens ({markdown; key}), called with {after: the key the person
	// last saw}; nothing new answers no markdown (What's new).
	Source string `json:"source,omitempty" yaml:"source,omitempty" jsonschema:"description=An action URI answering the guide's text when it opens ({markdown; key}): called with {after: the key last seen}; a new key means something new to show"`
	// Essential is the sidecar's choice of what a starter needs: at the
	// Essentiel display level the guide lists its essentials first.
	Essential bool `json:"essential,omitempty" yaml:"essential,omitempty" jsonschema:"description=What a starter needs (the sidecar's choice - not a level): listed first at the Essentiel display level"`
	// Concept marks a document explaining KaTyoti's concepts:
	// the guide lists it first, under « Concepts KaTyoti ».
	Concept bool `json:"concept,omitempty" yaml:"concept,omitempty" jsonschema:"description=A document explaining the main concepts: listed first in the guide's Concepts section"`
	// ShownOnce marks a guide that opens by itself at the person's next
	// start - a Welcome or a What's new - and never again once seen: its
	// place in the queue of such guides.
	ShownOnce int `json:"shown_once,omitempty" yaml:"shown_once,omitempty" jsonschema:"description=Shown once at start: its place in the queue of the guides that open by themselves (1 first); absent or 0: never"`
	// OnceKey makes a shown-once guide new again: it is seen once per value.
	OnceKey string `json:"once_key,omitempty" yaml:"once_key,omitempty" jsonschema:"description=What makes a shown-once guide new again: it is seen once per value (a version)"`
	Steps   []Step `json:"steps,omitempty" yaml:"steps,omitempty" jsonschema:"description=The steps in the order they are played: a guide with steps is a Help me"`
}

// The three natures of a guide, read from its content (modules/guide,
// decision OD-003): steps make a Help me, a video without steps a Show me, a
// markdown body alone an Explain me. No field restates it.
const (
	NatureHelp    = "help"
	NatureShow    = "show"
	NatureExplain = "explain"
)

// Nature is what the guide is, from what it holds: "" for an empty one.
func (r Recipe) Nature() string {
	switch {
	case len(r.Steps) > 0:
		return NatureHelp
	case strings.TrimSpace(r.Video) != "":
		return NatureShow
	case strings.TrimSpace(r.Body) != "" || strings.TrimSpace(r.Source) != "":
		return NatureExplain
	}
	return ""
}

// Step is one instruction of a recipe. Its `id` is the stable key a
// translation names, so a step keeps its translations when steps are added or
// reordered.
type Step struct {
	ID     string  `json:"id" yaml:"id" jsonschema:"description=Stable key of the step (kebab-case) that translations name"`
	Say    string  `json:"say" yaml:"say" jsonschema:"description=The English text shown for the step in one or two sentences"`
	Target *Target `json:"target,omitempty" yaml:"target,omitempty" jsonschema:"description=The control the step points at"`
	// Open is an element the player opens before it points at the target
	// : its address in the person's universe.
	Open  string            `json:"open,omitempty" yaml:"open,omitempty" jsonschema:"description=An element to open before the step: its address in the person's universe - object/<id>/<tab> or home/<entry> or screen/<screen>/<section>"`
	Await string            `json:"await,omitempty" yaml:"await,omitempty" jsonschema:"description=What completes the step (manual when absent),enum=click|route|manual"`
	Route string            `json:"route,omitempty" yaml:"route,omitempty" jsonschema:"description=For await route: the main-pane route or tab kind that completes the step"`
	Do    string            `json:"do,omitempty" yaml:"do,omitempty" jsonschema:"description=Action URI the person can run from the step after confirming"`
	Check string            `json:"check,omitempty" yaml:"check,omitempty" jsonschema:"description=Action URI of a read-only method returning ok and detail about the result"`
	With  map[string]string `json:"with,omitempty" yaml:"with,omitempty" jsonschema:"description=Values passed to the step's check or do action beside the universe the player adds"`
}

// Target is how a step finds its control: its `data-guide` anchor — the
// kind of control (`marketplace.row`) — and, in a list, `name`, the instance
// (its `data-guide-name`). Never a role or a visible label: those are GUI
// text, and the GUI is translated (modules/guide, decision OD-007).
type Target struct {
	Guide string `json:"guide" yaml:"guide" jsonschema:"description=The control's data-guide anchor: the kind of control"`
	Name  string `json:"name,omitempty" yaml:"name,omitempty" jsonschema:"description=The instance in a list (its data-guide-name)"`
}

// IsTour reports whether the recipe is a tour: it has nothing to check.
func (r Recipe) IsTour() bool {
	for _, s := range r.Steps {
		if s.Check != "" {
			return false
		}
	}
	return true
}

// Checks lists the check URIs of the recipe, in step order.
func (r Recipe) Checks() []string {
	var out []string
	for _, s := range r.Steps {
		if s.Check != "" {
			out = append(out, s.Check)
		}
	}
	return out
}

// EffectiveAwait is what completes the step: its `await`, else manual.
func (s Step) EffectiveAwait() string {
	if s.Await == "" {
		return AwaitManual
	}
	return s.Await
}

// ParseRecipe decodes a recipe file. It only decodes: the recipe lint
// (internal/extensions/guide/lint) says what is wrong with it.
func ParseRecipe(raw []byte) (Recipe, error) {
	var r Recipe
	if err := yaml.Unmarshal(raw, &r); err != nil {
		return Recipe{}, fmt.Errorf("guide recipe: %w", err)
	}
	return r, nil
}

// CheckInput is what the player sends a step's check (and its do action):
// the universe on screen and the step's `with` values. A check method takes
// it as its input.
type CheckInput struct {
	Universe string            `json:"universe,omitempty" jsonschema:"The universe on screen when the step runs"`
	With     map[string]string `json:"with,omitempty" jsonschema:"The step's with values"`
}

// CheckResult is what a check answers: whether the result the recipe asked
// for is there, and a sentence the player shows either way. The sentence is
// English; DetailMsg carries it as a message key and its params
// (internal/core/usermsg), which the GUI says in the person's language from
// its `guide` namespace, English when the key is missing there.
type CheckResult struct {
	OK        bool         `json:"ok" jsonschema:"Whether the result is there"`
	Detail    string       `json:"detail" jsonschema:"What was found in one English sentence the player shows"`
	DetailMsg *usermsg.Msg `json:"detailMsg,omitempty" jsonschema:"The detail as a message key and its params for the GUI to translate"`
}

// Passed and Failed build a CheckResult.
func Passed(format string, a ...any) CheckResult {
	return CheckResult{OK: true, Detail: fmt.Sprintf(format, a...)}
}

// Failed reports a result that is not there yet.
func Failed(format string, a ...any) CheckResult {
	return CheckResult{OK: false, Detail: fmt.Sprintf(format, a...)}
}

// WithMsg gives the detail its message key (`checks.<name>`, a key of the
// GUI's `guide` namespace) and the values its sentence names.
func (r CheckResult) WithMsg(key string, params map[string]any) CheckResult {
	r.DetailMsg = &usermsg.Msg{Key: key, Params: params}
	return r
}

// RecipeText is a recipe's text in one language: what a translation replaces.
// Steps maps a step's `id` to its `say`. A string left out falls back to the
// English of the recipe.
type RecipeText struct {
	Name        string            `json:"name,omitempty" yaml:"name,omitempty"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Hashtags    []string          `json:"hashtags,omitempty" yaml:"hashtags,omitempty"`
	Body        string            `json:"body,omitempty" yaml:"body,omitempty"`
	Steps       map[string]string `json:"steps,omitempty" yaml:"steps,omitempty"`
}

// Translations is a recipe's `translations` layer: the file `<name>.i18n.yaml`
// beside the recipe, one RecipeText per language code (`fr`). The recipe file
// itself is the English (modules/guide, decision OD-015).
type Translations map[string]RecipeText

// ParseTranslations decodes a recipe's translations file: one text per
// language code; its `source_lang` (the language a recipe of a non-English
// pack is written in - contribapi.TranslationsSourceLangKey) is no language.
func ParseTranslations(raw []byte) (Translations, error) {
	var all map[string]yaml.Node
	if err := yaml.Unmarshal(raw, &all); err != nil {
		return nil, fmt.Errorf("guide recipe translations: %w", err)
	}
	t := Translations{}
	for lang, node := range all {
		if lang == "source_lang" {
			continue
		}
		var text RecipeText
		if err := node.Decode(&text); err != nil {
			return nil, fmt.Errorf("guide recipe translations: %s: %w", lang, err)
		}
		t[lang] = text
	}
	return t, nil
}

// Text returns the recipe's text in lang, each string falling back to the
// English of the recipe when the translation leaves it out.
func (r Recipe) Text(t Translations, lang string) RecipeText {
	out := RecipeText{Name: r.Name, Description: r.Description, Hashtags: r.Hashtags, Steps: map[string]string{}}
	for _, s := range r.Steps {
		out.Steps[s.ID] = s.Say
	}
	tr, ok := t[lang]
	if !ok {
		return out
	}
	if tr.Name != "" {
		out.Name = tr.Name
	}
	if tr.Description != "" {
		out.Description = tr.Description
	}
	if len(tr.Hashtags) > 0 {
		out.Hashtags = tr.Hashtags
	}
	for id, say := range tr.Steps {
		if _, known := out.Steps[id]; known && say != "" {
			out.Steps[id] = say
		}
	}
	return out
}

// A step's text names a control by its anchor, never by its label: the label
// is GUI text, which is translated (modules/guide, decision OD-007). The
// placeholder is `{control:<anchor>}`, or `{control:<anchor>|<name>}` for an
// instance in a list (its data-guide-name). The player replaces it with the
// label the GUI shows for that control, in bold, in the person's language.
var (
	reControlRef     = regexp.MustCompile(`\{control:([a-z0-9][a-z0-9.:-]*)(?:\|([^{}|]+))?\}`)
	reControlOpening = regexp.MustCompile(`\{\s*control\b`)
)

// ControlRef is one `{control:…}` placeholder of a step's text.
type ControlRef struct {
	Anchor string // the control's data-guide anchor
	Name   string // the instance (data-guide-name), when the anchor is a list's
}

// ControlRefs lists the placeholders of a text, in order, and the number of
// `{control` openings that are not a well-formed placeholder.
func ControlRefs(text string) (refs []ControlRef, malformed int) {
	for _, m := range reControlRef.FindAllStringSubmatch(text, -1) {
		refs = append(refs, ControlRef{Anchor: m[1], Name: strings.TrimSpace(m[2])})
	}
	return refs, len(reControlOpening.FindAllStringIndex(text, -1)) - len(refs)
}
