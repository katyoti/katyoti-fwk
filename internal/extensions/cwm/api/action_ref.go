// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ActionRef is one entry in an object type's or capability's `actions:`
// list — an action URI plus optional binding-site metadata. The URI is
// the runnable identity (tasks / scripts / mutations / KTSP methods all
// dispatch through ActionService by URI — modules/actions/concepts.md);
// everything else describes how THIS type binds the action:
// presentation overrides, a confirmation gate, a node-state predicate,
// and pre-bound input values.
//
// Two YAML/JSON spellings decode into the same shape — a bare string is
// shorthand for `{uri: <string>}`:
//
//	actions:
//	  - ktw-example:tasks/example/review        # bare URI
//	  - uri: ktw-agents:mutations/set-field          # object form
//	    label: "Archive"
//	    confirm: true
//	    when: "node.status != 'archived'"
//	    input: {field: status, value: archived}
type ActionRef struct {
	URI         string         `json:"uri"                   yaml:"uri"                   jsonschema:"Action URI to invoke (tasks / scripts / mutations / KTSP method); required"`
	Label       string         `json:"label,omitempty"       yaml:"label,omitempty"       jsonschema:"Binding-site label override; defaults to the action's own label / URI"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"Short human description of what the action does; shown under the label in the action bar. Defaults to the target task's own description when empty."`
	Kind        string         `json:"kind,omitempty"        yaml:"kind,omitempty"        jsonschema:"How the GUI launches it: 'ai' = an interactive agent — opens a CHAT TAB (mode:chat wizards like compose-artifact); 'sync'/'async' (default) = a headless run dispatched via ActionService (fire-and-forget, returns a value/task id). Set 'ai' ONLY for tasks the user converses with."`
	Icon        string         `json:"icon,omitempty"        yaml:"icon,omitempty"        jsonschema:"Binding-site icon override"`
	Confirm     bool           `json:"confirm,omitempty"     yaml:"confirm,omitempty"     jsonschema:"Require an explicit confirmation dialog before Run"`
	Preview     bool           `json:"preview,omitempty"     yaml:"preview,omitempty"     jsonschema:"Marks a not-yet-built 'future surface' action. The GUI renders it with an 'À venir' badge and, on click, shows a coming-soon panel instead of dispatching — it NEVER runs or mutates. Used to paint the full intended surface in a gate demo."`
	Quick       bool           `json:"quick,omitempty"       yaml:"quick,omitempty"       jsonschema:"One of the actions that matter most on the object: also a small button on its first page (it stays in the action panel)"`
	When        string         `json:"when,omitempty"        yaml:"when,omitempty"        jsonschema:"el predicate over the target object (env: object / id / type / parent_id / children); false disables the action"`
	Input       map[string]any `json:"input,omitempty"       yaml:"input,omitempty"       jsonschema:"Pre-bound input values merged under the caller's payload at launch"`

	// --- unified action model (ms-action-system) — all optional + backward
	// compatible. An action declared inline on a type omits Target (it is
	// inferred as {scope: object, types: [owning-type]}); a task self-
	// targeting a type it does not own sets Target explicitly.

	// Target — WHERE this action attaches. Empty = inferred from the
	// declaration site (the owning object type, scope object).
	Target *ActionTarget `json:"target,omitempty" yaml:"target,omitempty" jsonschema:"Where the action attaches: scope (global|object|collection) + types + when. Empty = inferred from the declaring type."`
	// InputSchema — the input DATA the action expects (JSON Schema). The GUI
	// auto-forms the fields not satisfied by Bind. Distinct from Input
	// (pre-bound values): this is the CONTRACT, Input is defaults.
	InputSchema map[string]any `json:"input_schema,omitempty" yaml:"input_schema,omitempty" jsonschema:"JSON Schema for the caller-supplied input data this action expects"`
	// Session — HOW the run is surfaced. Authoritative for the runner.
	Session *SessionSpec `json:"session,omitempty" yaml:"session,omitempty" jsonschema:"How the run is surfaced: mode none (sync inline value) | session (async, journaled, Output tab) | chat (interactive Chat tab, no Output tab)"`
	// Bind — auto-bind expressions evaluated against the target context
	// before run (e.g. {goal_id: \"$item.id\"}). Merged under the caller input.
	Bind map[string]string `json:"bind,omitempty" yaml:"bind,omitempty" jsonschema:"Auto-bind expressions ($node.* / $item.* / $now) resolved from the target context before dispatch"`
}

// Action scopes — where an action attaches.
const (
	ActionScopeGlobal     = "global"
	ActionScopeObject     = "object"     // one object
	ActionScopeCollection = "collection" // all objects of a type (a type's list header)
)

// Session modes — how a run is surfaced. Authoritative for the runner.
const (
	SessionModeNone    = "none"    // sync, inline value, no journal, no tab
	SessionModeSession = "session" // async, journaled, has an Output tab
	SessionModeChat    = "chat"    // interactive Chat tab, NO Output tab
)

// ActionTarget declares where an action attaches (the unified target concept).
type ActionTarget struct {
	Scope string   `json:"scope,omitempty" yaml:"scope,omitempty" jsonschema:"global | object | collection"`
	Types []string `json:"types,omitempty" yaml:"types,omitempty" jsonschema:"object types (scope object or collection); empty for global"`
	When  string   `json:"when,omitempty"  yaml:"when,omitempty"  jsonschema:"el predicate over the target; overrides ActionRef.When for task-declared targets"`
}

// SessionSpec declares how a run is surfaced.
type SessionSpec struct {
	Mode string `json:"mode,omitempty" yaml:"mode,omitempty" jsonschema:"none | session | chat"`
}

// EffectiveScope returns the action's target scope, defaulting to `object`
// (the shipped behaviour: an action attaches to the type that lists it).
func (r *ActionRef) EffectiveScope() string {
	if r.Target != nil && r.Target.Scope != "" {
		return r.Target.Scope
	}
	return ActionScopeObject
}

// ObjectScope is the declared scope, anything but global or collection being
// `object`.
func (r *ActionRef) ObjectScope() string {
	switch r.EffectiveScope() {
	case ActionScopeCollection:
		return ActionScopeCollection
	case ActionScopeGlobal:
		return ActionScopeGlobal
	default:
		return ActionScopeObject
	}
}

// EffectiveWhen returns the target's predicate, preferring an explicit
// Target.When over the legacy binding-site When.
func (r *ActionRef) EffectiveWhen() string {
	if r.Target != nil && r.Target.When != "" {
		return r.Target.When
	}
	return r.When
}

// EffectiveSessionMode resolves how the run is surfaced, mapping the legacy
// Kind field forward when Session is unset: ai→chat, sync→none, else→session.
func (r *ActionRef) EffectiveSessionMode() string {
	if r.Session != nil && r.Session.Mode != "" {
		return r.Session.Mode
	}
	switch r.Kind {
	case "ai":
		return SessionModeChat
	case "sync":
		return SessionModeNone
	default:
		return SessionModeSession
	}
}

// UnmarshalYAML accepts either a scalar string (URI shorthand) or a
// mapping with the full field set.
func (r *ActionRef) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		var uri string
		if err := value.Decode(&uri); err != nil {
			return err
		}
		*r = ActionRef{URI: uri}
		return nil
	case yaml.MappingNode:
		// Local alias type breaks the UnmarshalYAML recursion.
		type plain ActionRef
		var p plain
		if err := value.Decode(&p); err != nil {
			return err
		}
		*r = ActionRef(p)
		return nil
	default:
		return fmt.Errorf("actions entry: expected a URI string or a mapping, got YAML kind %d", value.Kind)
	}
}

// UnmarshalJSON mirrors the YAML dual spelling for JSON payloads
// (KTSP / bus transports re-encode registrations as JSON).
func (r *ActionRef) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var uri string
		if err := json.Unmarshal(b, &uri); err != nil {
			return err
		}
		*r = ActionRef{URI: uri}
		return nil
	}
	type plain ActionRef
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*r = ActionRef(p)
	return nil
}
