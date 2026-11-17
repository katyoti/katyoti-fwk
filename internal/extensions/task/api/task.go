// SPDX-License-Identifier: Apache-2.0

package api

import "encoding/json"

// Task represents a loaded TASK.md file.
//
// The frontmatter maps to TypeDef fields (DefMeta + Strategy + Config) plus
// task-domain-specific fields (InputSchema, OutputSchema).
//
// Three-level model:
//
//	Level 1 — Strategy (Go code): "agent-loop", "workflow", etc.
//	Level 2 — Task Type (this struct): strategy + config + schemas
//	Level 3 — Instance (runtime): task type + input values + context → state machine
//
// ConfigKeyCapability is the `config:` key that selects the LLM profile
// (`capability: smart`). Declared here, at the leaf, because the PUBLISHER
// reads it (task/contrib) and must not import the presets package for one
// string: presets drags the state-machine engine, agent/api and models/api
// into every sidecar that merely publishes tasks (found 2026-09-22).
const ConfigKeyCapability = "capability"

type Task struct {
	// Identity + display metadata (maps to strategies.DefMeta).
	// Icon convention matches skills + the loader's strict metaFields:
	// `icon` = emoji (terminal), `iconName` = Tabler icon (GUI). (The old
	// `ascii_icon`/`icon` inversion never matched any task file — all 54
	// use icon+iconName — nor the strict validator; fixed 2026-06-15.)
	Name string `yaml:"name"`
	// Title is the human-friendly display label. Optional; when empty the
	// GUI and pickers fall back to Name. Name stays the runnable id (shown
	// in small chars on cards + the `<binary> run <name>` argument).
	Title       string   `yaml:"title,omitempty"`
	Description string   `yaml:"description"`
	Icon        string   `yaml:"icon,omitempty"`     // emoji for terminal
	IconName    string   `yaml:"iconName,omitempty"` // Tabler icon for GUI
	Hashtags    []string `yaml:"hashtags,omitempty"` // categorization tags
	Category    string   `yaml:"category,omitempty"` // primary grouping
	Tags        []string `yaml:"tags,omitempty"`     // behavioral markers (e.g., "quick-action")

	// Advertised controls list/picker visibility. Default true.
	// `advertised: false` hides the task from the GUI gallery, pickers, and
	// bulk selectors (`*`, glob) — but the task stays callable by its EXACT
	// name/URI (direct run, or as a delegated worker of a top-level task).
	// Pointer-to-bool to encode three states (nil/default-true, explicit
	// true, explicit false) — mirrors the skills `advertised` convention.
	Advertised *bool `yaml:"advertised,omitempty"`

	// Kind is a hint about what context this task targets. Default `global`
	// means "runs from anywhere, no targeting." `node` means the task is
	// designed to operate on a CWM context node — the runtime threads the
	// current node through to the strategy via RuntimeContext.CurrentNode.
	// Surfaced via TaskSummary so GUIs can route the task to the right
	// affordance (e.g. "Review document" only appearing in a document node's
	// context menu). The runtime does NOT enforce kind compatibility today —
	// kind is metadata; enforcement is the caller's responsibility.
	Kind string `yaml:"kind,omitempty"`

	// Source is the TASK file as written, byte for byte — set by discovery,
	// never from YAML. It becomes the entry's `source` layer (the file a
	// reader opens, what provenance signs); `data` is the decoded body. Until
	// 2026-09-22 a task entry carried data only, so no reader — the pane, a
	// lint, a consumer — could get the file back.
	Source []byte `yaml:"-" json:"-"`

	// Types lists the CWM object types this task targets when Kind is
	// "object" (one object of those types) or "collection" (all objects of a
	// type); `*` = any. Empty with those kinds is refused at parse time;
	// ignored for "global".
	Types []string `yaml:"types,omitempty"`

	// Audience says whom the task is for: "end-user" (an action a person
	// using the universe runs), "technical" (a worker, a maintenance or
	// admin task) or "demo". Empty is undeclared - not end-user. A tagged
	// universe's first-level task list keeps the end-user tasks.
	Audience string `yaml:"audience,omitempty"`

	// SessionMode declares how a run is surfaced when this task is invoked as
	// an action: "none" (sync, inline value) | "session" (async, journaled) |
	// "chat" (interactive Chat tab). Parsed from the `session.mode` frontmatter;
	// when unset the loader infers "chat" from an agent `config.mode: chat`.
	// The action resolver reads it to route a task-declared action to a chat
	// vs a headless run (ms-action-system: session.mode is authoritative).
	SessionMode string `yaml:"session_mode,omitempty"`

	// Filter is an optional el predicate over the OBJECT vocabulary
	// (el.ObjectVocabulary: object / id / type / parent_id / children) that
	// decides whether the task is offered on a given object. When empty,
	// Types alone decides. When set it is the second pass, after the type
	// membership check, and must return a bool.
	//
	// Examples:
	//   filter: object.lifecycle.state == "active"
	//   filter: 'production' in object.tags && len(children) > 0
	//
	// Compile errors — a syntax error, or a name outside the vocabulary — are
	// refused at discovery time naming the task. It is evaluated by CWM's
	// object action list; an object it rejects shows the action disabled with
	// the reason.
	Filter string `yaml:"filter,omitempty"`

	// Provider is the binary/sidecar that hosts this task (e.g., "ktw-agents",
	// "ktw-demo"). Injected by the loader at discovery time — not a TASK.md
	// frontmatter field. Combined with Name, forms the canonical URI
	// `provider:name`, mirroring the skills naming convention.
	Provider string `yaml:"-"`

	// PermissionProfile names the kernel permission profile this task (and the
	// children it delegates) runs under, when the caller did not pass an
	// explicit --permission-profile. Empty = the configured default
	// (workspace-edit). Set it when a task authors files OUTSIDE the workspace
	// subdir (e.g. the compose wizard writing a sidecar's embed/), which the
	// default profile would gate. The name is a permission-profile contribution.
	PermissionProfile string `yaml:"permission_profile,omitempty"`

	// Group is the task group its runs are processed in (TaskGroups.yaml)
	// when the caller names none. Empty = the parent run's group, else the
	// default group.
	Group string `yaml:"group,omitempty"`

	// Strategy reference — which strategy creates the machine.
	Type string `yaml:"type"` // "agent", "workflow", "template", etc.

	// Config holds strategy-specific parameters from the TASK.md config: block.
	// Decoded by the strategy's TypeFactory into a typed struct.
	Config map[string]any

	// InputSchema is the JSON Schema declaring what input this task accepts.
	// Callers' input is validated against this before dispatch.
	// Hybrid: YAML-declared schema takes precedence; strategy default as fallback.
	InputSchema json.RawMessage

	// OutputSchema is the JSON Schema declaring what output this task produces.
	// Used by workflows for state.results typing and by UI for display.
	// Hybrid: YAML-declared schema takes precedence; strategy default as fallback.
	OutputSchema json.RawMessage

	// RawConfig is the full frontmatter as map — includes ALL fields.
	// Used internally by the loader; prefer Config for strategy-specific params.
	RawConfig map[string]any

	// Body is the markdown content after frontmatter.
	// The strategy declares which config field this maps to via BodyFieldName()
	// (e.g., agent → "prompt", template → "template").
	Body string
}

// IsAdvertised reports whether this task should appear in the GUI gallery,
// pickers, and bulk selectors. Default true when frontmatter omits
// `advertised:`. Mirrors `SkillMeta.IsAdvertised()`.
func (t *Task) IsAdvertised() bool {
	if t == nil || t.Advertised == nil {
		return true
	}
	return *t.Advertised
}

// URI returns the task's `provider:name` identifier. Falls back to bare
// Name if Provider is empty (legacy/unattributed tasks).
//
// Mirrors `SkillInfo.URI()` — same pattern.
func (t *Task) URI() string {
	if t == nil {
		return ""
	}
	if t.Provider == "" {
		return t.Name
	}
	return t.Provider + ":" + t.Name
}

// Task kind identifiers. The closed set is enforced at parse time.
const (
	// KindGlobal is the default kind. Task runs from anywhere; no targeting.
	KindGlobal = "global"

	// KindObject targets one CWM object of the declared Types; the runtime
	// threads the object into the strategy.
	KindObject = "object"

	// KindCollection targets ALL objects of a type (a type's list header) —
	// act on the type, not one object. Declares Types.
	KindCollection = "collection"
)

// Task audiences. The closed set is enforced at parse time; empty is
// undeclared.
const (
	AudienceEndUser   = "end-user"
	AudienceTechnical = "technical"
	AudienceDemo      = "demo"
)

// IsKnownAudience reports whether audience is empty or in the closed set.
func IsKnownAudience(audience string) bool {
	switch audience {
	case "", AudienceEndUser, AudienceTechnical, AudienceDemo:
		return true
	}
	return false
}

// IsKnownKind reports whether kind is in the closed set this version
// supports. Empty string is treated as KindGlobal (back-compat).
func IsKnownKind(kind string) bool {
	switch kind {
	case "", KindGlobal, KindObject, KindCollection:
		return true
	}
	return false
}
