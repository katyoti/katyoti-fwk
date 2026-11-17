// SPDX-License-Identifier: Apache-2.0

// Package api is the object-context service contract: what a chat started on
// one object receives (AgentContext) and which objects of a kind are in force
// at an object (Effective — the inheritance a kind declares with
// `propagation:` and `resolution_strategy:`).
//
// The DECLARATIONS are CWM's vocabulary (cwmapi.AgentDecl on a capability, a
// type, a policy, an object; cwmapi.AgentFraming, the `agent-context`
// contribution; a type's propagation fields); this service reads them — from
// the catalog and through CWM's reads — and resolves them.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ObjectContextServiceDef describes the service for KTSP publishing.
var ObjectContextServiceDef = meta.ServiceDef{
	Name:    "object-context",
	Short:   "What a chat on an object receives, and which objects of a kind are in force at it",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"AgentContext": {Short: "What a chat started on an object receives: sections, tools and skills, layered from the project to the object, with what was left out and why", ReadOnly: true},
		"Effective":    {Short: "Which objects of a kind are in force at an object: its own, what the kind's propagation reaches, what its type injects, reduced by the kind's resolution strategy", ReadOnly: true},
	},
}

// ObjectContextService resolves an object's context.
type ObjectContextService interface {
	AgentContext(ctx context.Context, in AgentContextInput) (AgentContextOutput, error)
	Effective(ctx context.Context, in EffectiveInput) (EffectiveOutput, error)
}

// ObjectContextConfig is ObjectContextService.yaml: the project layer of every
// object's agent context and the bound on its sections.
type ObjectContextConfig struct {
	// Agent is the project layer: what every chat on every object receives
	// first.
	Agent *cwmapi.AgentDecl `yaml:"agent,omitempty" json:"agent,omitempty"`
	// MaxBytes bounds the system sections (0 = DefaultMaxBytes); over it they
	// are trimmed farthest first, each trim reported.
	MaxBytes int `yaml:"max_bytes,omitempty" json:"max_bytes,omitempty"`
}

// DefaultMaxBytes bounds the system sections when ObjectContextService.yaml says
// nothing.
const DefaultMaxBytes = 24 * 1024

// Layer names, farthest first. A source is `<layer>` or `<layer>:<name>`.
const (
	LayerProject    = "project"
	LayerRule       = "rule"
	LayerPolicy     = "policy"
	LayerExtension  = "extension"
	LayerCapability = "capability"
	LayerType       = "type"
	LayerObject     = "object"
)

// SectionCurrentObject is the object's identity block, contributed at the
// object layer so an object's own declaration may replace it.
const SectionCurrentObject = "current-object"

// AgentContextInput names the object a chat starts on.
type AgentContextInput struct {
	ID string `json:"id" jsonschema:"Object id; required"`
}

// Section is one section as the agent receives it.
type Section struct {
	ID     string `json:"id"`
	Title  string `json:"title,omitempty"`
	Text   string `json:"text"`
	Order  int    `json:"order,omitempty"`
	Source string `json:"source" jsonschema:"project | rule:<id> | policy:<name> | extension:<name> | capability:<name> | type:<name> | object"`
	// File is capability/name when the section is a named file's content.
	File string `json:"file,omitempty"`
}

// Tool is one tool with where it came from.
type Tool struct {
	URI    string `json:"uri,omitempty"`
	Plugin string `json:"plugin,omitempty"`
	Source string `json:"source"`
}

// Skill is one skill selector with where it came from.
type Skill struct {
	Selector string `json:"selector"`
	Source   string `json:"source"`
}

// Note kinds.
const (
	NoteRemoved = "removed" // a closer `when: false` took it out
	NoteSkipped = "skipped" // its `when` does not hold
	NoteError   = "error"   // its `when` or its `${...}` cannot be read; not injected
	NoteTrimmed = "trimmed" // over the budget, farthest first
	NoteEmpty   = "empty"   // a named file not written yet, or a section with no text
)

// Note says why an entry is not (wholly) in the context.
type Note struct {
	List   string `json:"list"   jsonschema:"system | tools | skills | files"`
	Key    string `json:"key"`
	Source string `json:"source"`
	Kind   string `json:"kind"   jsonschema:"removed | skipped | error | trimmed | empty"`
	Reason string `json:"reason,omitempty"`
}

// AgentContextOutput is what a chat on the object receives.
type AgentContextOutput struct {
	ObjectID string    `json:"object_id"`
	Type     string    `json:"type"`
	System   []Section `json:"system"`
	Tools    []Tool    `json:"tools"`
	Skills   []Skill   `json:"skills"`
	Notes    []Note    `json:"notes,omitempty"`
	// Bytes is the size of the system sections kept; Budget the bound.
	Bytes  int `json:"bytes"`
	Budget int `json:"budget"`
}

// FramingSource is an `agent-context` contribution's name in the output.
func FramingSource(f AgentFraming) string {
	if f.Provider == "" {
		return LayerExtension + ":" + f.Name
	}
	return LayerExtension + ":" + f.Provider + "/" + f.Name
}

// EffectiveInput asks which objects of a kind are in force at one object.
type EffectiveInput struct {
	// ID is the focused object — the one the set is "in force at".
	ID string `json:"id" jsonschema:"The object the set is resolved for; required"`
	// Type is the kind being collected. ITS declaration carries the
	// propagation and the resolution strategy: how far a rule reaches belongs
	// to rules, not to whoever is asking.
	Type string `json:"type" jsonschema:"The object type being collected; its declaration carries propagation and resolution_strategy. Required"`
	// IncludeBy adds the by-source grouping, a diagnostic: which of the three
	// sources each result came from.
	IncludeBy bool `json:"include_by,omitempty" jsonschema:"Include the by-source grouping (local / tree / type)"`
}

// EffectiveOutput is the reduced set, with the rules that were applied.
type EffectiveOutput struct {
	Objects     []cwmapi.ObjectBrief `json:"objects"`
	BySource    *BySource            `json:"by_source,omitempty"`
	Strategy    string               `json:"strategy,omitempty"`
	Propagation string               `json:"propagation,omitempty"`
}

// BySource groups the result by where each object came from.
type BySource struct {
	Local []cwmapi.ObjectBrief `json:"local,omitempty"`
	Tree  []cwmapi.ObjectBrief `json:"tree,omitempty"  jsonschema:"From ancestors (downstream kinds) or descendants (upstream rollup)"`
	Type  []cwmapi.ObjectBrief `json:"type,omitempty"  jsonschema:"Type-injected by the object's type"`
}
