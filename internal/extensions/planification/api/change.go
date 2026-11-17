// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// THE CHANGE LOG AS DATA — each user-visible change is a `change` object under
// the release it ships in (sys-planification, `changes/`): its version and its
// stream are its release's, never repeated. A session adds an entry instead of
// editing a page, so two sessions never write the same file; a release's page
// (docs/confluence-tew/changelog/<version>.md) is rendered from its entries,
// between the page's head and tail the release keeps (ChangeLogHead,
// ChangeLogTail). Import reads an existing page into entries once.

const (
	// ChangeType is the object type of a change-log entry.
	ChangeType = "change"
	// CapChange is its field-group.
	CapChange = "change"
	// ChangeLogDir is where the release pages are written, from the project root.
	ChangeLogDir = "docs/confluence-tew/changelog"
)

// The values of a change, in their order on a page.
const (
	ValueMajor       = "major"
	ValueNotable     = "notable"
	ValueImprovement = "improvement"
	ValueFix         = "fix"
)

// ValueOrder is the values from the most to the least important.
var ValueOrder = []string{ValueMajor, ValueNotable, ValueImprovement, ValueFix}

// ValueLabels is how a page writes a value.
var ValueLabels = map[string]string{
	ValueMajor:       "★★★ Majeur",
	ValueNotable:     "★★ Notable",
	ValueImprovement: "★ Amélioration",
	ValueFix:         "🛠 Correction",
}

// ChangeFields is the `change` field-group (cap-change). The entry's title is
// the object's name and its text the object's body.
type ChangeFields struct {
	Number   int      `json:"number" jsonschema:"description=Its number on the pages: numbered across the releases and never reused,required"`
	Label    string   `json:"label,omitempty" jsonschema:"description=Its number as a page wrote it when it is not one: 1.001-9 is change 9 of 1.001 listed again"`
	Domain   string   `json:"domain,omitempty" jsonschema:"description=The user-facing domain it belongs to (Planification & méthode - Interface - ...)"`
	Value    string   `json:"value,omitempty" jsonschema:"description=major or notable or improvement or fix,enum=major|notable|improvement|fix"`
	Maturity string   `json:"maturity,omitempty" jsonschema:"description=Prévu or Construit et testé or À valider en interface or Disponible or Reporté"`
	ForWhom  string   `json:"for_whom,omitempty" jsonschema:"description=Who it is for (architectes - tous - ...)"`
	Modules  []string `json:"modules,omitempty" jsonschema:"description=The modules it changed"`
	Requests []string `json:"requests,omitempty" jsonschema:"description=The requests and features it delivers"`
	Date     string   `json:"date,omitempty" jsonschema:"description=When it was recorded (YYYY-MM-DD)"`
	Update   string   `json:"update,omitempty" jsonschema:"description=Its line in the page's Dernière mise à jour (une phrase - construit et testé)"`
	Badge    string   `json:"badge,omitempty" jsonschema:"description=Its badge line as an imported page wrote it: kept as written when set"`
}

// Change is an entry as the service reads it.
type Change struct {
	ID      string `json:"id"`
	Release string `json:"release"`
	Version string `json:"version,omitempty"`
	Stream  string `json:"stream,omitempty"`
	Title   string `json:"title"`
	Text    string `json:"text,omitempty" jsonschema:"description=What it brings: the paragraphs under its badge line"`
	ChangeFields
}

// AddChangeInput records one change in a release's change log.
type AddChangeInput struct {
	Release  string   `json:"release" jsonschema:"description=The release it ships in: its id or its version (2026.10),required"`
	Title    string   `json:"title" jsonschema:"description=Its title: the gain in plain words,required"`
	Domain   string   `json:"domain" jsonschema:"description=Its domain (Planification & méthode - Interface - ...),required"`
	Value    string   `json:"value" jsonschema:"description=major or notable or improvement or fix,enum=major|notable|improvement|fix,required"`
	Maturity string   `json:"maturity,omitempty" jsonschema:"description=Its maturity; Construit et testé when empty"`
	ForWhom  string   `json:"for_whom,omitempty" jsonschema:"description=Who it is for; tous when empty"`
	Text     string   `json:"text" jsonschema:"description=What it brings (markdown): Ce que ça vous apporte and Le progrès,required"`
	Requests []string `json:"requests,omitempty" jsonschema:"description=The requests and features it delivers; its modules are theirs"`
	Modules  []string `json:"modules,omitempty" jsonschema:"description=The modules it changed when no request names them"`
	Update   string   `json:"update,omitempty" jsonschema:"description=Its line in Dernière mise à jour; its title in lower case when empty"`
	Date     string   `json:"date,omitempty" jsonschema:"description=YYYY-MM-DD; today when empty"`
}

// UpdateChangeInput changes an entry; an empty field keeps its value.
type UpdateChangeInput struct {
	ID       string   `json:"id" jsonschema:"description=The entry,required"`
	Release  string   `json:"release,omitempty" jsonschema:"description=Move it to this release (its id or its version): its number kept - refused out of a released release or into a released or dropped one"`
	Title    string   `json:"title,omitempty"`
	Domain   string   `json:"domain,omitempty"`
	Value    string   `json:"value,omitempty" jsonschema:"enum=major|notable|improvement|fix"`
	Maturity string   `json:"maturity,omitempty"`
	ForWhom  string   `json:"for_whom,omitempty"`
	Text     string   `json:"text,omitempty"`
	Requests []string `json:"requests,omitempty"`
	Update   string   `json:"update,omitempty"`
}

// ListChangesInput filters the entries; every filter is optional.
type ListChangesInput struct {
	Stream  string `json:"stream,omitempty" jsonschema:"description=The stream its release is in"`
	Release string `json:"release,omitempty" jsonschema:"description=Its release: an id or a version"`
	Module  string `json:"module,omitempty" jsonschema:"description=A module it changed"`
	From    string `json:"from,omitempty" jsonschema:"description=Recorded on or after (YYYY-MM-DD)"`
	To      string `json:"to,omitempty" jsonschema:"description=Recorded on or before (YYYY-MM-DD)"`
	Text    string `json:"text,omitempty" jsonschema:"description=Words found in its title or text (any case)"`
}

// ChangeOutput is one entry.
type ChangeOutput struct {
	Change Change `json:"change"`
}

// ListChangesOutput is the entries, by number then id.
type ListChangesOutput struct {
	Changes []Change `json:"changes"`
}

// RenderChangeLogInput renders a release's page.
type RenderChangeLogInput struct {
	Release string `json:"release" jsonschema:"description=The release: its id or its version,required"`
	Write   bool   `json:"write,omitempty" jsonschema:"description=Write the page to docs/confluence-tew/changelog/<version>.md"`
}

// RenderChangeLogOutput is the page.
type RenderChangeLogOutput struct {
	Release  string `json:"release"`
	Path     string `json:"path" jsonschema:"description=The page's path from the project root"`
	Markdown string `json:"markdown"`
	Changes  int    `json:"changes"`
	Written  bool   `json:"written,omitempty"`
}

// ImportChangeLogInput reads a release's page into entries, once.
type ImportChangeLogInput struct {
	Release  string `json:"release" jsonschema:"description=The release: its id or its version,required"`
	Markdown string `json:"markdown,omitempty" jsonschema:"description=The page's text; read from docs/confluence-tew/changelog/<version>.md when empty"`
}

// ImportChangeLogOutput says what the import made.
type ImportChangeLogOutput struct {
	Release string   `json:"release"`
	Changes []string `json:"changes" jsonschema:"description=The entries created"`
}

// WhatsNewInput asks what is new since the entry a person last saw.
type WhatsNewInput struct {
	After string `json:"after,omitempty" jsonschema:"description=The entry the person last saw (its id - the key a previous answer gave); empty: the latest version only"`
}

// WhatsNewOutput is the What's new text: each version holding entries recorded
// after the one last seen, newest first, its En un coup d'œil then its changes.
type WhatsNewOutput struct {
	Markdown string   `json:"markdown" jsonschema:"description=The text to show; empty when nothing is new"`
	Key      string   `json:"key,omitempty" jsonschema:"description=The newest entry shown: what to pass as after next time"`
	Versions []string `json:"versions" jsonschema:"description=The versions it shows"`
	// Document is the installed letter the text comes from, when it does: its
	// whole body - the letter then the full change log - opened by its id.
	Document string `json:"document,omitempty" jsonschema:"description=The installed letter document the text is from (its id): it opens whole - the letter then the full change log"`
}

// ChangeServiceDef describes the service for KTSP/CLI auto-generation.
var ChangeServiceDef = meta.ServiceDef{
	Name:    "changes",
	Short:   "The change log as data: each user-visible change an entry under its release; queried by stream - module - period - text; the release pages rendered from them",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Add":      meta.Method("Record a user-visible change in a release's change log: numbered after every entry - its modules from the requests it names"),
		"Update":   meta.Method("Change an entry: only what is given changes - a release moves it there with its number kept (never out of a released release)"),
		"Get":      {Short: "Read one entry", ReadOnly: true},
		"List":     {Short: "The entries - by stream - release - module - period (from - to) and words in their title or text", ReadOnly: true},
		"Render":   meta.Method("Render a release's change-log page from its entries (its head and tail kept by the release); write it to docs/confluence-tew/changelog/<version>.md when asked"),
		"WhatsNew": {Short: "What is new since the entry a person last saw: each version holding newer entries - newest first - its En un coup d'œil then its changes; the key to pass next time", ReadOnly: true},
		"Import":   meta.Method("Read a release's existing change-log page into entries - its head and tail kept on the release; refused when the release already has entries"),
	},
}

// ChangeService manages the change log.
type ChangeService interface {
	Add(ctx context.Context, in AddChangeInput) (ChangeOutput, error)
	Update(ctx context.Context, in UpdateChangeInput) (ChangeOutput, error)
	Get(ctx context.Context, in GetInput) (ChangeOutput, error)
	List(ctx context.Context, in ListChangesInput) (ListChangesOutput, error)
	Render(ctx context.Context, in RenderChangeLogInput) (RenderChangeLogOutput, error)
	Import(ctx context.Context, in ImportChangeLogInput) (ImportChangeLogOutput, error)
	WhatsNew(ctx context.Context, in WhatsNewInput) (WhatsNewOutput, error)
}
