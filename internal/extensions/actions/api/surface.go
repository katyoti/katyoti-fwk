// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE ACTION SURFACE: what an object (or a type's list, or nobody) offers.
// Four sources, one list: the actions its type and its effective capabilities
// declare (read from CWM's effective surface), the tasks that apply themselves
// to its type (`kind: object|collection` + `types:` + `filter:` in a TASK.md)
// and the global registry (`kind: global`) — deduplicated, each `when:` /
// `filter:` evaluated and each `input:` interpolated over the object by CWM
// (ObjectEvaluate, so it reads what a guard reads, functions included).

// SurfaceServiceDef describes the service for KTSP publishing.
var SurfaceServiceDef = meta.ServiceDef{
	Name:    "action-surface",
	Short:   "What an object, a type's list or the universe offers as actions",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"List": {Short: "The actions an object (scope object), a type's list under it (collection) or the universe (global) offers: when evaluated, input interpolated", ReadOnly: true},
	},
}

// SurfaceService lists actions.
type SurfaceService interface {
	List(ctx context.Context, in ListInput) (ListOutput, error)
}

// ListInput asks what is offered. The scopes: `object` (one object), `collection`
// (the header of a list of one type under an object) and `global` (no object).
type ListInput struct {
	Scope string `json:"scope,omitempty" jsonschema:"global | object | collection; empty = object when id is set, else global"`
	ID    string `json:"id,omitempty"    jsonschema:"The object the actions are for (required for object and collection)"`
	Type  string `json:"type,omitempty"  jsonschema:"For collection: the object type the list holds"`
}

// ListOutput is the resolved, evaluated action set.
type ListOutput struct {
	Actions []cwmapi.Action `json:"actions"`
}

// TaskAction is one task that offers itself as an action — its targeting, as
// its TASK.md declares it.
type TaskAction struct {
	Provider    string   // e.g. "ktw-methods"
	Name        string   // task name (the action URI is <provider>:tasks/<name>)
	Label       string   // display label (Title)
	Icon        string   // icon id
	Description string   // short description
	Kind        string   // global | object | collection
	Types       []string // object types it targets; `*` = any
	Filter      string   // optional el predicate over the object; false disables
	SessionMode string   // none | session | chat (chat → kind ai: the GUI opens a Chat tab)
}

// TaskActionSource lists the tasks that offer themselves as actions.
type TaskActionSource func(ctx context.Context) []TaskAction
