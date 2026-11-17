// SPDX-License-Identifier: Apache-2.0

// Package api is the goals service contract: the operations a person thinks
// in when managing a module's goals. Every one is implemented over the CWM
// Object API — the service keeps no storage and no index of its own.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// GoalType is the object type a goal is stored as.
const GoalType = "goal"

// EdgeServes is the edge kind from what serves a goal to the goal.
const EdgeServes = "serves"

// GoalsServiceDef describes the service for KTSP/CLI auto-generation.
var GoalsServiceDef = meta.ServiceDef{
	Name:    "goals",
	Short:   "A module's goals: add, order, reparent, link what serves them, find the uncovered ones",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Add":        meta.Method("Add a goal to a module, under the module or under another goal"),
		"Remove":     meta.Method("Remove a goal that has no child goals"),
		"List":       meta.Method("List a module's goals in their order"),
		"Reorder":    meta.Method("Set the order of the goals under one parent"),
		"Reparent":   meta.Method("Hang a goal under another goal of its module, or under the module"),
		"LinkServer": meta.Method("Record that an object serves a goal (a serves edge)"),
		"Uncovered":  meta.Method("List the goals nothing serves"),
		"Board":      {Short: "Every goal of an object's subtree as the page the module's Goals tab shows: by state, each with its state, the object it sits under, its reachability and what serves it", ReadOnly: true},
	},
}

// GoalsService manages goal objects.
type GoalsService interface {
	Add(ctx context.Context, in AddInput) (AddOutput, error)
	Remove(ctx context.Context, in RemoveInput) (RemoveOutput, error)
	List(ctx context.Context, in ListInput) (ListOutput, error)
	Reorder(ctx context.Context, in ReorderInput) (ReorderOutput, error)
	Reparent(ctx context.Context, in ReparentInput) (ReparentOutput, error)
	LinkServer(ctx context.Context, in LinkServerInput) (LinkServerOutput, error)
	Uncovered(ctx context.Context, in UncoveredInput) (UncoveredOutput, error)
	// Board is the Goals panel's source.
	Board(ctx context.Context, in BoardInput) (render.Def, error)
}

// BoardInput names the object whose goals to show (a module: its whole
// subtree; a project: every module) and the page's state filter.
type BoardInput struct {
	ID       string `json:"id"                 jsonschema:"Object id; required"`
	Language string `json:"language,omitempty" jsonschema:"The person's language (en or fr); empty = English"`
	State    string `json:"state,omitempty"    jsonschema:"Only the goals in this lifecycle state"`
}

// Goal is the service's view of one goal object.
type Goal struct {
	ID           string `json:"id"`
	Module       string `json:"module"`
	Title        string `json:"title"`
	Parent       string `json:"parent"                 jsonschema:"The module or the goal it sits under"`
	Reachability string `json:"reachability,omitempty" jsonschema:"asymptotic or closable"`
	Order        int    `json:"order,omitempty"        jsonschema:"Position among its siblings; 0 = unordered (after the ordered ones)"`
}

// AddInput adds a goal.
type AddInput struct {
	Module       string `json:"module"                 jsonschema:"Module the goal belongs to; required"`
	Title        string `json:"title"                  jsonschema:"The wanted outcome in one line; required"`
	Parent       string `json:"parent,omitempty"       jsonschema:"A goal of the same module to sit under; empty = the module itself"`
	Reachability string `json:"reachability,omitempty" jsonschema:"asymptotic, the only reachability of a goal (empty = asymptotic); something a test can declare done is a feature"`
}

// AddOutput returns the created goal.
type AddOutput struct {
	Goal Goal   `json:"goal"`
	Path string `json:"path" jsonschema:"Repo-relative path of the goal's file"`
}

// RemoveInput removes one goal.
type RemoveInput struct {
	ID string `json:"id" jsonschema:"Goal id; required"`
}

// RemoveOutput echoes the removed id.
type RemoveOutput struct {
	ID string `json:"id"`
}

// ListInput selects a module's goals.
type ListInput struct {
	Module string `json:"module" jsonschema:"Module whose goals to list; required"`
}

// ListOutput returns the goals, ordered by parent then position then id.
type ListOutput struct {
	Goals []Goal `json:"goals"`
}

// ReorderInput sets the order of the goals under ONE parent.
type ReorderInput struct {
	IDs []string `json:"ids" jsonschema:"Every goal under one parent, in the wanted order; required"`
}

// ReorderOutput returns the goals in their new order.
type ReorderOutput struct {
	Goals []Goal `json:"goals"`
}

// ReparentInput moves a goal within its module.
type ReparentInput struct {
	ID        string `json:"id"                   jsonschema:"Goal id; required"`
	NewParent string `json:"new_parent,omitempty" jsonschema:"A goal of the same module; empty = the module itself"`
}

// ReparentOutput returns the moved goal.
type ReparentOutput struct {
	Goal Goal `json:"goal"`
}

// LinkServerInput records that Server serves Goal.
type LinkServerInput struct {
	Goal   string `json:"goal"   jsonschema:"The goal served; required"`
	Server string `json:"server" jsonschema:"The object that serves it (a use-case, a feature, a sub-goal ...); required"`
}

// LinkServerOutput echoes the edge.
type LinkServerOutput struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// UncoveredInput selects the goals to check.
type UncoveredInput struct {
	Module string `json:"module,omitempty" jsonschema:"Only this module's goals; empty = every goal"`
}

// UncoveredOutput lists the goals nothing serves.
type UncoveredOutput struct {
	Goals []Goal `json:"goals"`
}
