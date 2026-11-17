// SPDX-License-Identifier: Apache-2.0

package api

import (
	"reflect"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// Resource type discriminators for skill/task resources emitted into a
// sidecar's descriptor. Consumers filter by these strings via
// Manifest.GetContributionsByType[T].
//
// These types are additive today: sidecars that wire
// `skillslocal.ContributeSkillResources` (or the task equivalent) emit
// these entries into their descriptor, but the legacy per-sidecar
// `SkillManager` / `TaskLibrary` aggregator paths still work in
// parallel. The full cutover (consumer migration + aggregator deletion)
// lands in `bus-split-aggregator-to-catalog`.

import "encoding/json"

const (
	ResourceTypeSkill = "skill"
	ResourceTypeTask  = "task"
)

// SkillFrontmatterKeys returns the keys a SKILL.md frontmatter may carry —
// the json names of SkillBody, which a drift test holds to the skills
// pack's `<binary>:/data-type/skill`. Compose's skill-format lint reads this; it used
// to keep its own copy of the list.
func SkillFrontmatterKeys() map[string]bool {
	rt := reflect.TypeOf(SkillBody{})
	keys := make(map[string]bool, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		if name, _, _ := strings.Cut(tag, ","); name != "" && name != "-" {
			keys[name] = true
		}
	}
	return keys
}

// LayerInstructions is the skill's L1 layer: the markdown body after the
// frontmatter, as text — the kind's default layer, what a reader opens. The
// file as written is `source`; the frontmatter is `data`.
const LayerInstructions = contribapi.LayerInstructions

// SkillBody is a skill's data layer: its SKILL.md frontmatter, exactly, under
// `<binary>:/data-type/skill`.
//
// The picker fields (name, description, icon, iconName, hashtags, category,
// advertised) are written here ONCE by the producer; the manifest builder
// derives the envelope's meta from them (the kind declares `meta.from:
// data`), so a listing reads meta and the body cannot disagree with it. From
// c22 to 2026-09-22 the body carried only the wiring fields and the producer
// wrote meta itself — two writers with no guard between them.
//
// `requires` / `provides` are wiring: what the skill needs, what it exposes.
// L1 instructions are the `instructions` layer, and L2 references and scripts
// stay out of band — fetch them by URI when something actually reads them.
//
// The entry's ID is the skill name (e.g. "doc-create"); the canonical
// URI is `<provider>:/skill/<id>`.
//
// This struct is the ONE source of the contract (2026-09-22, e8 S7): the
// parser decodes the YAML frontmatter into it (`yaml` tags, strict), the
// data layer carries it (`json` tags), and the skills pack publishes its shape
// as `<binary>:/data-type/skill` (skills/contrib/skillkind) — the
// descriptions below are the schema's descriptions (NO commas in them: the
// jsonschema tag grammar splits on commas). Change a field here and every
// reader, writer and validator moves together.
type SkillBody struct {
	// Name is the kebab-case identifier — the entry's ID.
	Name string `json:"name" yaml:"name" jsonschema:"description=kebab-case skill identifier; the entry's ID in the URI"`
	// Description is the one-paragraph summary a picker shows.
	Description string `json:"description,omitempty" yaml:"description" jsonschema:"description=one paragraph for the picker and the agent's skill list,required"`
	// Icon is the emoji rendering; IconName the Tabler icon for GUIs.
	Icon     string `json:"icon,omitempty" yaml:"icon,omitempty" jsonschema:"description=emoji rendering for terminals and the agent prompt block"`
	IconName string `json:"iconName,omitempty" yaml:"iconName,omitempty" jsonschema:"description=Tabler icon name for GUI consumers"`
	// Hashtags and Category group the skill in pickers.
	Hashtags []string `json:"hashtags,omitempty" yaml:"hashtags,omitempty" jsonschema:"description=free tags for grouping and search"`
	Category string   `json:"category,omitempty" yaml:"category,omitempty" jsonschema:"description=the picker group this skill lists under"`
	// Advertised is visibility in listings; nil means advertised. Visibility,
	// NOT exposure — see Provides.
	Advertised *bool `json:"advertised,omitempty" yaml:"advertised,omitempty" jsonschema:"description=visibility in listings and the agent's skill block; absent means advertised. Visibility NOT exposure: an unadvertised skill stays callable by name"`

	// Plugin dependencies (PluginRef format, e.g. "skill:template-engine").
	// Used by `requires:` resolution at agent setup time.
	Requires []string `json:"requires,omitempty" yaml:"requires,omitempty" jsonschema:"description=capability tokens this skill depends on"`

	// Exposure declarations (from 'provides' field in SKILL.md frontmatter):
	// task, cli, mcp-prompt, mcp-tool, mcp-resource. Drives whether the
	// skill is wrapped as a task, a CLI command, an MCP entry, etc.
	//
	// Exposure, NOT visibility. `advertised` is the visibility flag.
	Provides []string `json:"provides,omitempty" yaml:"provides,omitempty" jsonschema:"description=exposure declarations (task / cli / mcp-prompt / mcp-tool / mcp-resource). Exposure NOT visibility — see advertised"`
}

// IsAdvertised reports whether this skill appears in listings and the
// agent's skill block. Absent means yes.
func (b SkillBody) IsAdvertised() bool { return b.Advertised == nil || *b.Advertised }

// TaskBody is the typed body for resources with type "task".
//
// Mirrors `task.api.TaskSummary` + `TaskInfo` (the L0 picker
// metadata + the full task definition including config block and body
// markdown) so the catalog can ship task definitions without spawning
// the host sidecar.
//
// Entry ID is the task name (e.g. "doc-create"); canonical URI is
// `<provider>:/task/<id>`.
type TaskBody struct {
	// Core identity + L0.
	Name string `json:"name" jsonschema:"description=task type id (canonical name; forms <provider>:<name>)"`
	// Title is the human-friendly display label; consumers fall back to
	// Name when empty. Name stays the runnable id.
	Title       string `json:"title,omitempty" jsonschema:"description=display title"`
	Type        string `json:"type" jsonschema:"description=strategy name that creates the machine (agent / task / workflow / simulator)"` // Machine type (strategy or named type)
	Description string `json:"description,omitempty" jsonschema:"description=human-readable summary"`

	// Frontmatter `config:` block — strategy-specific params decoded by
	// the machine factory at runtime. Kept opaque (JSON RawMessage) here
	// so the catalog doesn't need to know each strategy's config schema.
	// json.RawMessage (not []byte) so it marshals as the raw JSON object
	// on the wire — a []byte base64-encodes it (failing the task
	// data-type's `config: object` validation).
	Config json.RawMessage `json:"config,omitempty" jsonschema:"description=strategy-specific parameters (decoded by the strategy)"`

	// Markdown body after the frontmatter (the task's prompt / template
	// / workflow YAML). Drives `BodyFieldName()` injection at run time.
	Body string `json:"body,omitempty" jsonschema:"description=the markdown after the frontmatter (an agent's system prompt)"`

	// PermissionProfile carries the task's root-level `permission_profile:`
	// frontmatter across the descriptor boundary. A remote task (resolved by
	// another binary via the catalog) needs this so its delegated children run
	// under the declared profile (e.g. compose-artifact's `compose-edit`)
	// instead of falling back to the restrictive default. Without it the field
	// is silently dropped and the task is denied writes outside the workspace.
	PermissionProfile string `json:"permissionProfile,omitempty" jsonschema:"description=the kernel permission profile a run uses when its caller passes none"`

	// Advertised controls list/picker visibility. Default true (the field
	// is omitted on the wire when nil/true). Explicit false hides the task
	// from the gallery + bulk selectors; it stays callable by exact name.
	// Pointer-to-bool to encode default-true / explicit-true / explicit-false.
	Advertised *bool `json:"advertised,omitempty" jsonschema:"description=false hides the task from listings; it still runs when named"`

	// Group is the task group the task's runs are processed in when the
	// caller names none (TaskGroups.yaml).
	Group string `json:"group,omitempty" jsonschema:"description=the task group its runs are processed in when the caller names none"`

	// Sort / picker metadata.
	Capability string   `json:"capability,omitempty" jsonschema:"description=the LLM profile shortcut - read into config.capability"`
	Icon       string   `json:"icon,omitempty" jsonschema:"description=emoji for terminal and agent"`
	IconName   string   `json:"iconName,omitempty" jsonschema:"description=Tabler icon name for the GUI"`
	Hashtags   []string `json:"hashtags,omitempty" jsonschema:"description=categorization tags"`
	Category   string   `json:"category,omitempty" jsonschema:"description=primary grouping"`
	Tags       []string `json:"tags,omitempty" jsonschema:"description=behavioral markers (e.g. quick-action)"` // Behavioral markers (e.g., quick-action)

	// Targeting: kind global | object | collection, and the object types.
	Kind   string   `json:"kind,omitempty" jsonschema:"description=context target: global (when absent) | object | collection,enum=global|object|collection"`
	Types  []string `json:"types,omitempty" jsonschema:"description=CWM object types this task targets when kind is object or collection; * = any"`
	Filter string   `json:"filter,omitempty" jsonschema:"description=el expression over the object gating applicability"` // expr-lang predicate
	// Audience: end-user | technical | demo — whom the task is for; empty is
	// undeclared (not end-user).
	Audience string `json:"audience,omitempty" jsonschema:"description=whom the task is for: end-user | technical | demo; absent: undeclared (not end-user),enum=end-user|technical|demo"`
	// SessionMode: none (sync inline) | session (async journaled) | chat
	// (interactive tab) — how a task-declared action surfaces when run.
	SessionMode string `json:"sessionMode,omitempty" jsonschema:"description=how the task surfaces as an action (chat opens a chat tab); config.mode chat reads as chat when absent"`

	// I/O contracts — schema-driven UI consumers (ktw-gui's WidgetPanel
	// per ms-schema-driven-ui US-S1) read these to render input forms +
	// typed output views. Each is the canonical DSL/AST JSON the
	// SchemaService's Parse → AST pipeline accepts. Empty means the
	// task either takes no structured input / produces no structured
	// output (legacy + agent-loop tasks fall here today).
	InputSchema  json.RawMessage `json:"inputSchema,omitempty" jsonschema:"description=the input contract as a DSL document (set by the catalog from the file beside the task)"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty" jsonschema:"description=the output contract as a DSL document (set by the catalog from the file beside the task)"`
}

// Normalize applies to a task's data as the catalog publishes it (its
// frontmatter as written) the defaults the task parser applies to its file
// (taskcontrib.Parse): kind `global` when absent; a root `capability` written
// into config (over any value there), else Capability read from config;
// sessionMode `chat` when absent and config.mode is chat. Every reader of a
// task's data calls it once.
func (b *TaskBody) Normalize() {
	if b.Kind == "" {
		b.Kind = "global" // taskapi.KindGlobal
	}
	var cfg map[string]any
	if len(b.Config) > 0 {
		_ = json.Unmarshal(b.Config, &cfg)
	}
	if b.Capability != "" {
		if cfg == nil {
			cfg = map[string]any{}
		}
		cfg["capability"] = b.Capability // taskapi.ConfigKeyCapability
		if raw, err := json.Marshal(cfg); err == nil {
			b.Config = raw
		}
	} else if c, ok := cfg["capability"].(string); ok {
		b.Capability = c
	}
	if b.SessionMode == "" {
		if m, _ := cfg["mode"].(string); m == "chat" {
			b.SessionMode = "chat"
		}
	}
}
