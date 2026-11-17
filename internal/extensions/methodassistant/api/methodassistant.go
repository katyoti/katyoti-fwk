// SPDX-License-Identifier: Apache-2.0

// Package api is the method-assistant service contract: what the method's
// panels on an object show — where its scope stands and what to do next, and
// what it is made of (goals, features). Every verb reads and
// writes through the CWM Object API; the service keeps nothing of its own.
//
// The object pane's method panels (ktw-methods' object-panels method-next and
// composition-overview) name these verbs as their `source:`: the panel calls
// the verb with the object id and its view receives the answer.
package api

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/assistant"
)

// ActURI is the action a What's Next answer offers to run one of its actions,
// with {id, kind}: the view runs it through the action path.
const ActURI = "ktw-methods:method-assistant/act"

// MethodAssistantServiceDef describes the service for KTSP publishing.
var MethodAssistantServiceDef = meta.ServiceDef{
	Name:    "method-assistant",
	Short:   "The method on an object: where its scope stands, what to do next, what it is made of",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Overview":    {Short: "Where an object's scope stands and what to do next: its goals, its features with their acceptance criteria, the alignment findings and the ranked actions", ReadOnly: true},
		"Act":         meta.Method("Run one of an object's What's Next actions: an alignment fix is written to CWM; creative work names the skill to launch"),
		"Composition": {Short: "What an object is made of: its goals and its features with the goals they serve", ReadOnly: true},
		"NextPage":    {Short: "What's Next on an object as the page that shows it: the acceptance criteria's standing, the requests to triage, the goals and features with their criteria, the findings and the ranked actions to run", ReadOnly: true},
	},
}

// MethodAssistantService answers the method panels.
type MethodAssistantService interface {
	Overview(ctx context.Context, in ObjectInput) (OverviewOutput, error)
	Act(ctx context.Context, in ActInput) (ActOutput, error)
	Composition(ctx context.Context, in ObjectInput) (CompositionOutput, error)
	// NextPage is Overview drawn: the method-next panel's source.
	NextPage(ctx context.Context, in ObjectInput) (render.Def, error)
}

// ObjectInput names the object a panel is shown on.
type ObjectInput struct {
	ID       string `json:"id" jsonschema:"Object id; required"`
	Language string `json:"language,omitempty" jsonschema:"The person's language (en or fr); empty = English"`
	// Release narrows the scope to the work aimed at a release or an
	// increment: what its scope holds (a project release: its module
	// releases' and its increments' too), the features under or serving it
	// and their criteria. Empty: everything under the object.
	Release string `json:"release,omitempty" jsonschema:"A release or an increment: only the work aimed at it stays in sight; empty = everything"`
	// Advanced adds the planning fields to Composition's tree rows (cost of
	// delay and its parts, reachability); off, a row is its title and state.
	Advanced bool `json:"advanced,omitempty" jsonschema:"Composition: add the planning fields (cost of delay, reachability) to each row"`
}

// OverviewOutput is What's Next on an object, scoped to its subtree (a
// project's scope is the whole index). Beyond the counts, findings and ranked
// actions it carries the navigable structure: the goals, and each feature
// with the acceptance criteria that prove it.
type OverviewOutput struct {
	Overview assistant.Overview  `json:"overview"`
	Findings []assistant.Finding `json:"findings"`
	Actions  []assistant.Action  `json:"actions"`
	Goals    []GoalView          `json:"goals"`
	Stories  []StoryView         `json:"stories"`
	// LooseCriteria are the acceptance criteria in scope under no feature.
	LooseCriteria []CriterionView `json:"looseCriteria"`
	// Act is the action URI that runs one of Actions, with {id, kind}.
	Act string `json:"act" jsonschema:"Action URI running one of the actions with {id; kind}"`
}

// GoalView is one goal with how many features serve it and how many of those
// are delivered.
type GoalView struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	ServingUS   int    `json:"servingUS"`
	DeliveredUS int    `json:"deliveredUS"`
}

// StoryView is one feature with its acceptance criteria inline: those under
// it, directly or through its sub-features.
type StoryView struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	CoD        int             `json:"cod"`
	Delivered  bool            `json:"delivered"`
	PassCount  int             `json:"passCount"`
	TotalCount int             `json:"totalCount"`
	Criteria   []CriterionView `json:"criteria"`
}

// CriterionView is one acceptance criterion: its title, the status of its
// recorded verdict and the object it sits under.
type CriterionView struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status" jsonschema:"Its verdict's status: pass or fail or needs-harness or harness-error; no-test when it has no verdict or no probe"`
	NodeID string `json:"nodeId" jsonschema:"The object the criterion sits under"`
}

// ActInput runs the top action of a kind (the top action overall when Kind is
// empty), in the scope of the object ID.
type ActInput struct {
	ID   string `json:"id"             jsonschema:"Object id whose What's Next is acted on; required"`
	Kind string `json:"kind,omitempty" jsonschema:"Act on the top action of this kind; empty = the top action"`
	// Release narrows the scope acted on as Overview's (ObjectInput.Release).
	Release string `json:"release,omitempty" jsonschema:"A release or an increment: act within the work aimed at it; empty = everything"`
}

// ActOutput is the outcome: OK when a write resolved the finding.
type ActOutput struct {
	Action  string `json:"action"`
	Message string `json:"message"`
	OK      bool   `json:"ok"`
}

// CompositionOutput is what an object is made of, by lens: its direct
// children of type goal (why it exists) and feature (what is built, each
// with the goals it serves).
type CompositionOutput struct {
	Goals    []Item `json:"goals"`
	Features []Item `json:"features"`
	// Tree is what planning reads first: the goals under the object, each
	// feature under the goal it serves (or the feature it breaks down), its
	// sub-features under it.
	Tree []TreeRow `json:"tree"`
	// Advanced says the rows carry the planning fields.
	Advanced bool `json:"advanced,omitempty"`
}

// TreeRow is one goal or feature of the planning tree.
type TreeRow struct {
	ID     string `json:"id"`
	Parent string `json:"parent,omitempty" jsonschema:"The goal or feature it sits under in the tree; empty at the top"`
	Title  string `json:"title"`
	Type   string `json:"type"`
	State  string `json:"state,omitempty"`
	// Meta is the row's second line: its type, and with Advanced its cost of
	// delay and reachability.
	Meta string `json:"meta,omitempty"`
}

// Item is one child as the composition page shows it: its envelope and its
// type's fields (the table's columns).
type Item struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Title   string         `json:"title"`
	Status  string         `json:"status,omitempty"  jsonschema:"Its lifecycle state"`
	Tags    []string       `json:"tags,omitempty"`
	Summary string         `json:"summary,omitempty" jsonschema:"Its description"`
	Fields  map[string]any `json:"fields,omitempty"  jsonschema:"Its type's own fields"`
	// Serves are the goals a feature serves; ServesText says them in the
	// person's language, as the page shows them under the feature.
	Serves     []Ref  `json:"serves,omitempty"     jsonschema:"The goals a feature serves"`
	ServesText string `json:"servesText,omitempty" jsonschema:"The goals a feature serves, as one line"`
}

// Ref names an object: its id and its title.
type Ref struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// MarshalJSON writes an item as the row its type's table reads: the type's
// fields at the top level (the table's columns name them), the envelope over
// them (a lifecycle state wins over a field of the same name), and `fields`
// as well.
func (it Item) MarshalJSON() ([]byte, error) {
	type plain Item
	env, err := json.Marshal(plain(it))
	if err != nil || len(it.Fields) == 0 {
		return env, err
	}
	var top map[string]any
	if err := json.Unmarshal(env, &top); err != nil {
		return nil, err
	}
	row := make(map[string]any, len(it.Fields)+len(top))
	for k, v := range it.Fields {
		row[k] = v
	}
	for k, v := range top {
		row[k] = v
	}
	return json.Marshal(row)
}
