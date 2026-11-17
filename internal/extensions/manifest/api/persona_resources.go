// SPDX-License-Identifier: Apache-2.0

package api

import schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"

// ResourceTypePersona is the discriminator for persona entries
// emitted into a sidecar's descriptor. A persona is structurally
// a `TASK.md` of `type: agent-loop` plus persona-specific
// frontmatter (`category: persona`, `bind_mode`, `default_gate`,
// `types`, …). The catalog ships the resolved
// descriptor so consumers (CWM's persona library) can resolve +
// list without round-tripping the host sidecar.
//
// Each method pack contributes its own personas via
// `personascontrib.ContributePersonaLoader` (parallel to
// `skillslocal.ContributeSkillLoader`). The ktw-agents binary
// contributes ZERO built-in personas itself — every persona
// ships through a method-pack sidecar.
//
// Canonical URI: `<provider>:/persona/<name>`.
const ResourceTypePersona = "persona"

// PersonaBody is the typed body for resources with type
// "persona". Embeds `TaskBody` because a persona IS a task at
// the wire level (the TaskService runs it via the existing
// `agent-loop` machinery); the embedded fields carry the
// task-level definition (Name, Type, Description, Config, Body,
// Category, Hashtags). The persona-specific fields below
// supplement that with the resolution metadata CWM's persona
// library needs.
//
// Discriminator: every PersonaBody has `Category: "persona"` at
// the embedded TaskBody level.
type PersonaBody struct {
	TaskBody

	// BindMode controls how the persona is parameterised on
	// spawn. One of:
	//   - "object": spawn requires a `target` object id. The
	//               embedded TaskBody's Types lists the object types
	//               the persona is meant for (empty = any).
	//   - "global": spawn runs without a target (e.g. a
	//               system-health auditor).
	//   - "custom": the persona's TASK.md body defines its own
	//               bind logic. It is routed to "object"; full
	//               support is deferred.
	BindMode string `json:"bind_mode,omitempty" jsonschema:"description=how the persona is spawned: object (on a target object) | global | custom,enum=object|global|custom"`

	// DefaultGate is the DoD criterion ID (namespaced:
	// `<owner-node-uuid>.<local-id>`) the persona toggles on
	// successful completion. `persona.Spawn` (PersonaService) accepts a
	// `gate` override; when omitted, the substrate uses this.
	DefaultGate string `json:"default_gate,omitempty"`

	// Skills lists the skills the agent loop preloads when this
	// persona runs. Skill names follow the `<provider>:<name>`
	// convention surfaced by the skill catalog.
	Skills []string `json:"skills,omitempty"`

	// Tier marks where the persona was resolved from. Set at
	// resolve time by the persona library — not authored in the
	// TASK.md frontmatter. One of "workspace" | "universe" |
	// "built-in".
	Tier string `json:"tier,omitempty"`

	// Augments names the baseline persona this file layers ON TOP
	// of (same `name:`, different provider). When set, the persona
	// library MERGES this body into the named baseline at load time
	// and never surfaces it standalone — a method pack ships the
	// universal baseline, a language pack ships the augmentation.
	// Empty for a baseline persona. See the persona-augmentation
	// merge resolver.
	Augments string `json:"augments,omitempty"`

	// AppliesTo narrows an augmentation to the work it concerns: when a
	// spawn states its context (the files the work touches, its module),
	// the layer merges only if the context matches. Nil — or a spawn with
	// no context — merges it always.
	AppliesTo *PersonaScope `json:"applies_to,omitempty"`
}

// OverrideSchema leaves `type` optional in the persona schema: the embedded
// TaskBody requires it (the task kind does), but a persona is always an
// agent-loop (Spawn forces it) and the persona format never required it. The
// JSON form is unchanged: TaskBody.Type carries it.
func (*PersonaBody) OverrideSchema(root *schemaapi.Type) {
	if f := root.Fields["type"]; f != nil {
		f.Optional = true
	}
}

// PersonaScope is the work an augmentation concerns. Each list that is set
// must match; an empty list does not narrow.
type PersonaScope struct {
	// Files are globs (`**/*.go`); the layer applies when a file the work
	// touches matches one of them.
	Files []string `json:"files,omitempty"`
	// Modules are module ids; the layer applies when the work is in one.
	Modules []string `json:"modules,omitempty"`
}
