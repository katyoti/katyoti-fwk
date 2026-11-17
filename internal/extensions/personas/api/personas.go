// SPDX-License-Identifier: Apache-2.0

// Package api is the persona library's public surface: the
// PersonaService interface, its I/O types and its ServiceDef.
//
// Personas are NOT a task type of their own — they are TASK.md files
// of type `agent-loop` plus persona frontmatter, shipped by sidecars
// through the manifest catalog (`mbapi.ResourceTypePersona`, produced
// by `personas/contrib.PersonaProvider`, the generic producer). This service reads
// them back from the catalog and spawns them through the TaskService.
package api

import (
	"context"
	"errors"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

var (
	// ErrInvalidInput is returned for a malformed request (e.g. no name).
	ErrInvalidInput = errors.New("personas: invalid input")
	// ErrNotFound is returned when no catalog persona carries the name.
	ErrNotFound = errors.New("personas: not found")
)

// PersonaService resolves, lists and spawns catalog personas.
type PersonaService interface {
	// Resolve returns one persona's metadata by name.
	Resolve(ctx context.Context, in ResolveInput) (ResolveOutput, error)
	// List returns the catalog's personas, optionally filtered.
	List(ctx context.Context, in ListInput) (ListOutput, error)
	// Spawn runs a persona as an agent-loop task on a target node.
	Spawn(ctx context.Context, in SpawnInput) (SpawnOutput, error)
}

// PersonaServiceDef describes the published PersonaService for
// KTSP / HTTP / auto-CLI discovery.
var PersonaServiceDef = meta.ServiceDef{
	Name:    "persona",
	Short:   "Persona library — resolve, list and spawn catalog personas",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Resolve": meta.Method("Resolve a persona by name from the catalog"),
		"List":    {Short: "List available personas", RenderTemplate: personaListTmpl},
		"Spawn":   meta.Method("Spawn a persona on a target node with a gate"),
	},
}

const personaListTmpl = `{{ header "Personas" -}}
{{ if .personas -}}
{{ table .personas "name" "category" "tier" "bind_mode" "default_gate" }}
{{ dim (printf "%d persona(s)" (count .personas)) }}
{{- else -}}
{{ dim "No personas available." }}
{{- end }}`

// Tier identifies which tier of the library a persona resolves from
// (workspace > universe > built-in). The catalog yields built-in only.
type Tier string

const (
	TierBuiltIn   Tier = "built-in"
	TierUniverse  Tier = "universe"
	TierWorkspace Tier = "workspace"
)

// Info describes one available persona.
type Info struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"           jsonschema:"specialist | manager"`
	Tier        Tier     `json:"tier"               jsonschema:"Which tier resolved this persona"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Types       []string `json:"types,omitempty"    jsonschema:"Object types the persona may target; empty = any"`
	BindMode    string   `json:"bind_mode"          jsonschema:"object | global | custom"`
	DefaultGate string   `json:"default_gate,omitempty" jsonschema:"Canonical gate criterion this persona toggles"`
	Skills      []string `json:"skills,omitempty"   jsonschema:"Skill names the persona depends on"`
	Path        string   `json:"path,omitempty"     jsonschema:"Resolution hint (the persona name)"`
}

// ResolveInput resolves one persona by name.
type ResolveInput struct {
	Name string `json:"name" jsonschema:"Persona name (e.g. reviewer, project-manager); required"`
}

// ResolveOutput returns the resolved persona's metadata.
type ResolveOutput struct {
	Persona Info `json:"persona"`
}

// ListInput optionally filters by category, object type or tier.
type ListInput struct {
	Category string `json:"category,omitempty" jsonschema:"Filter: specialist | manager"`
	Type     string `json:"type,omitempty"     jsonschema:"Filter: only personas that may target this object type"`
	Tier     string `json:"tier,omitempty"     jsonschema:"Filter: built-in | universe | workspace"`
}

// ListOutput returns matching persona descriptors.
type ListOutput struct {
	Personas []Info `json:"personas"`
}

// SpawnInput spawns a persona on a target object with a gate. The
// persona's TASK.md runs as an agent-loop sub-task.
type SpawnInput struct {
	Name          string         `json:"name"               jsonschema:"Persona name; required"`
	Target        string         `json:"target,omitempty"   jsonschema:"Target object id; required for bind_mode object"`
	Gate          string         `json:"gate,omitempty"     jsonschema:"gate criterion ID the persona must satisfy"`
	Inputs        map[string]any `json:"inputs,omitempty"   jsonschema:"Persona-specific inputs (validated against its input-schema)"`
	ParentSpawnID string         `json:"parent_spawn_id,omitempty" jsonschema:"Spawn ID of the parent agent (for child-spawn audit chain)"`
	SessionID     string         `json:"session_id,omitempty" jsonschema:"The session the run joins (a request's work runs in one); empty starts its own"`
	Context       *SpawnContext  `json:"context,omitempty" jsonschema:"The work the persona is spawned for; a specialised layer (applies_to) merges only when it matches. Unset merges every layer"`
	// Group, Priority and Origin pass to the run: the persona's run is
	// governed by its task group and shows who started it, like any run.
	Group    string             `json:"group,omitempty"    jsonschema:"The task group the run joins (TaskGroups.yaml); empty: the default group"`
	Priority int                `json:"priority,omitempty" jsonschema:"The run's priority in its group's queue"`
	Origin   *taskapi.RunOrigin `json:"origin,omitempty"   jsonschema:"Who starts the run: a person or the planner or an agent"`
}

// SpawnContext is the work a persona is spawned for — what a specialised
// layer's applies_to is matched against.
type SpawnContext struct {
	Files  []string `json:"files,omitempty"  jsonschema:"The paths or globs the work touches"`
	Module string   `json:"module,omitempty" jsonschema:"The module the work is in"`
}

// SpawnOutput returns the spawned task identifier.
type SpawnOutput struct {
	SpawnID string `json:"spawn_id"            jsonschema:"Unique spawn UUID"`
	TaskID  string `json:"task_id,omitempty"   jsonschema:"Task run id (when the run started)"`
	Status  string `json:"status"              jsonschema:"running | queued | rejected"`
	Reason  string `json:"reason,omitempty"    jsonschema:"Why the spawn was rejected or queued"`
}
