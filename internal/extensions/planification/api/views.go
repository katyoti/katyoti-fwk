// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// MANAGEMENT VIEWS — the work, visible, in any format. Each view answers
// Markdown documents (render.Document), never a file and never a target: a
// converter owned by the target (Confluence first) publishes them. A page's
// stable key is `meta.key`, its parent's `meta.parent`, so a converter keeps
// the same page across publications.

// Document meta keys a view sets.
const (
	MetaKey    = "key"
	MetaParent = "parent"
)

// ViewServiceDef describes the service for KTSP/CLI auto-generation.
var ViewServiceDef = meta.ServiceDef{
	Name:    "views",
	Short:   "Management views of the work: change log, release boards, release plans with their sprints — as documents",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"ChangeLog":        meta.Method("The change log: released releases and their done work, and work delivered outside a release"),
		"ReleaseBoard":     meta.Method("A release's board: its scope, done or not, the module releases shipping in it, its plan in figures"),
		"ReleasePlan":      meta.Method("A release's plan: its sprints (steps), what each holds and waits on, the critical path, what can start now"),
		"Stream":           meta.Method("The whole page set of a stream: a root, the change log, the releases, a board and a plan per release"),
		"DeckTab":          {Short: "One tab of the planning deck, loaded when it opens: history, deliveries or groups - of every module, a module or a release", ReadOnly: true},
		"Deck":             {Short: "The planning deck: a page of tabs over the plans, the demands, the task groups and the releases — of every module (target empty), a module (target or id) or a release (target) — with its actions", ReadOnly: true},
		"Work":             {Short: "A module's work (every module's when empty): its to-dos by state - running, ready in the order they are taken, waiting with what they wait on, failed - and the ended ones counted", ReadOnly: true},
		"DeckTargets":      {Short: "What the planning deck can be about: every module, each module, each open release", ReadOnly: true},
		"PersonQueue":      {Short: "« À faire »: everything waiting on the person - each item asking exactly one action: decide, test by hand, validate, answer or unblock (notices apart) - from the person steps, the open decisions with a proposal, the hand tests and the inbox entries; of every module, a module or an object", ReadOnly: true},
		"PersonQueuePage":  {Short: "The « À faire » page: the person's queue by action, each card saying what is already proven with the buttons doing what it asks", ReadOnly: true},
		"PersonQueueCount": {Short: "How many items wait on the person (the notices left out): the rail's badge of « À faire »", ReadOnly: true},
		"Request":          {Short: "A request's panel: where it stands and what a person is expected to do with the actions that do it, its deliverables (verify one by hand), a form revising them, its details, its steps and its trail", ReadOnly: true},
	},
}

// ViewService renders management views.
type ViewService interface {
	ChangeLog(ctx context.Context, in ChangeLogInput) (DocumentsOutput, error)
	ReleaseBoard(ctx context.Context, in ViewInput) (DocumentsOutput, error)
	ReleasePlan(ctx context.Context, in ViewInput) (DocumentsOutput, error)
	Stream(ctx context.Context, in StreamInput) (DocumentsOutput, error)
	Deck(ctx context.Context, in DeckInput) (render.Def, error)
	DeckTab(ctx context.Context, in DeckTabInput) (render.Def, error)
	DeckTargets(ctx context.Context, in DeckTargetsInput) (DeckTargetsOutput, error)
	Request(ctx context.Context, in RequestViewInput) (render.Def, error)
	Work(ctx context.Context, in WorkInput) (DocumentsOutput, error)
	PersonQueue(ctx context.Context, in PersonQueueInput) (PersonQueueOutput, error)
	PersonQueuePage(ctx context.Context, in PersonQueueInput) (render.Def, error)
	PersonQueueCount(ctx context.Context, in PersonQueueInput) (PersonQueueCountOutput, error)
}

// WorkInput names whose work the Work view shows.
type WorkInput struct {
	Module string `json:"module,omitempty" jsonschema:"description=The module; empty: every module"`
	Lang   string `json:"lang,omitempty"   jsonschema:"description=en (default) or fr"`
}

// RequestViewInput names the request the request panel shows.
type RequestViewInput struct {
	ID       string `json:"id" jsonschema:"description=The request,required"`
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}

// DeckTargetsInput asks what the deck can be about.
type DeckTargetsInput struct{}

// DeckTarget is one choice of the deck's target selector.
type DeckTarget struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Group string `json:"group,omitempty"`
}

// DeckTargetsOutput is the deck's targets: modules, then open releases.
type DeckTargetsOutput struct {
	Targets []DeckTarget `json:"targets"`
}

// DeckInput names the deck's target: every module when both are empty, a
// module by `target` (the Global strip's selector) or by `id` (the module the
// pane is on).
type DeckInput struct {
	Target string `json:"target,omitempty" jsonschema:"description=The module or the release the deck is about; empty: every module"`
	ID     string `json:"id,omitempty"     jsonschema:"description=The module whose pane shows the deck; used when target is empty"`
	// Period is what the Done tile and lane count: today, the current week
	// (from Monday) or the current month.
	Period string `json:"period,omitempty" jsonschema:"description=What Done counts: day or week or month; empty: week,enum=day|week|month"`
	// Language is the person's: the deck's title, tabs and Overview
	//
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}

// DeckTabInput names one tab of the deck a section loads when it opens:
// what the deck is about, as DeckInput, and the tab.
type DeckTabInput struct {
	Target string `json:"target,omitempty" jsonschema:"description=The module or the release the deck is about; empty: every module"`
	ID     string `json:"id,omitempty"     jsonschema:"description=The module whose pane shows the deck; used when target is empty"`
	Tab    string `json:"tab"              jsonschema:"description=The tab: history or deliveries or groups,enum=history|deliveries|groups,required"`
}

// The deck's tabs loaded as sections when they open.
const (
	DeckTabHistory    = "history"
	DeckTabDeliveries = "deliveries"
	DeckTabGroups     = "groups"
)

// The periods the deck's Done tile counts.
const (
	PeriodDay   = "day"
	PeriodWeek  = "week"
	PeriodMonth = "month"
)

// ViewInput names what a view shows.
type ViewInput struct {
	ID   string `json:"id"             jsonschema:"The release; required"`
	Lang string `json:"lang,omitempty" jsonschema:"The labels' language: en (default) or fr"`
}

// ChangeLogInput scopes the change log.
type ChangeLogInput struct {
	Module string `json:"module,omitempty" jsonschema:"Only the work of this module and its sub-modules; empty = the project"`
	Lang   string `json:"lang,omitempty"  jsonschema:"en (default) or fr"`
}

// StreamInput names the stream the page set is about.
type StreamInput struct {
	Name string `json:"name"           jsonschema:"The stream's name (KaTyoti); required"`
	Lang string `json:"lang,omitempty" jsonschema:"en (default) or fr"`
}

// DocumentsOutput is a view: pages, parents before children.
type DocumentsOutput struct {
	Documents []render.Document `json:"documents"`
}
