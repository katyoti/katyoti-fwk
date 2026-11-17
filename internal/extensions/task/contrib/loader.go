// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"bytes"
	"encoding/json"
	"fmt"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"gopkg.in/yaml.v3"
)

// metaFields is the closed set of root-level keys the canonical TASK.md
// schema accepts. Anything else at root in strict mode is a parse error.
// `config:` is the strategy-specific opaque block (validated by the
// matching MachineFactory, not here).
var metaFields = map[string]bool{
	// Identity + L0
	"name": true, "type": true, "description": true, "title": true,
	// Display metadata
	"icon": true, "iconName": true, "hashtags": true, "category": true, "tags": true, taskapi.ConfigKeyCapability: true,
	// Visibility (default true; advertised:false hides from gallery/pickers, stays callable by name)
	"advertised": true,
	// Strategy-specific config. The I/O contracts are sibling files
	// (<stem>.input.dsl.yaml, <stem>.output.dsl.yaml); the inline form is
	// still read for task folders outside the repository (the lint says so).
	"config": true, "input-schema": true, "output-schema": true,
	// Kernel permission profile selection (task-declared default); the
	// snake form is the old one, still read.
	"permissionProfile": true, "permission_profile": true,
	// The task group its runs are processed in (TaskGroups.yaml).
	"group": true,
	// Object targeting (CWM-bound tasks): kind object | collection + types.
	"kind": true, "types": true, "filter": true,
	// Whom the task is for: end-user | technical | demo (empty: undeclared).
	"audience": true,
	// Action surfacing when a task is invoked as an action: sessionMode
	// (session: {mode} is the old form, still read).
	"sessionMode": true, "session": true,
}

// Parse parses a TASK.md file into a Task struct in strict mode —
// unknown root frontmatter keys cause a parse error. Use ParseRelaxed
// for migration tooling that needs to read legacy files (the
// flat-frontmatter shape, ascii_icon, snake_case node_types, …).
func Parse(data []byte) (*api.Task, error) {
	return parseTask(data, true)
}

// ParseRelaxed parses a TASK.md file in relaxed mode — unknown root
// frontmatter keys are silently ignored, and the legacy-flat shape
// (non-meta root fields collapsed into Config) is accepted. Intended
// only for the task-migration-sweep that reads legacy files before
// rewriting them. Removed once the sweep lands per NO LEGACY.
func ParseRelaxed(data []byte) (*api.Task, error) {
	return parseTask(data, false)
}

func parseTask(data []byte, strict bool) (*api.Task, error) {
	frontmatter, body, err := splitFrontmatter(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse task: %w", err)
	}

	// Parse frontmatter into raw map.
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(frontmatter), &raw); err != nil {
		return nil, fmt.Errorf("parse task frontmatter: %w", err)
	}
	return taskFromFields(raw, body, strict)
}

// taskFromFields is a task from its frontmatter fields and its body: the one
// reading of a task's fields, whether they come from its file (parseTask) or
// from its data layer in the catalog (CatalogTask).
func taskFromFields(raw map[string]any, body string, strict bool) (*api.Task, error) {
	name, _ := raw["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("parse task: missing required field 'name'")
	}

	typeName, _ := raw["type"].(string)
	if typeName == "" {
		return nil, fmt.Errorf("parse task %q: missing required field 'type'", name)
	}

	// The node / item vocabulary is refused by name, ahead of the unknown-key
	// check, so an old task says what to write instead.
	for _, old := range []string{"node_types", "nodeTypes", "item_types", "itemTypes"} {
		if _, ok := raw[old]; ok {
			return nil, fmt.Errorf("parse task %q: %q is the node / item vocabulary — write `types: [...]` (the object types the task targets)", name, old)
		}
	}

	// Strict mode: reject unknown root keys. Legacy-flat shape is rejected here
	// (any non-meta root key fails, instead of being silently folded into Config).
	if strict {
		for k := range raw {
			if !metaFields[k] {
				return nil, fmt.Errorf("parse task %q: unknown frontmatter key %q (canonical meta set: %s; strategy-specific keys belong under `config:`)", name, k, knownMetaKeys())
			}
		}
	}

	title, _ := raw["title"].(string)
	description, _ := raw["description"].(string)
	icon, _ := raw["icon"].(string)
	iconName, _ := raw["iconName"].(string)
	category, _ := raw["category"].(string)
	permissionProfile, _ := raw["permissionProfile"].(string)
	if permissionProfile == "" {
		permissionProfile, _ = raw["permission_profile"].(string)
	}
	group, _ := raw["group"].(string)
	capability, _ := raw[taskapi.ConfigKeyCapability].(string)

	// advertised: pointer-to-bool — nil when omitted (default true), else the
	// explicit value. Mirrors the skills frontmatter convention.
	var advertised *bool
	if v, ok := raw["advertised"].(bool); ok {
		advertised = &v
	}

	hashtags := stringList(raw, "hashtags")
	tags := stringList(raw, "tags")

	// Kind defaults to global; closed-set validation. object and collection
	// require the object types they target.
	kind, _ := raw["kind"].(string)
	switch kind {
	case "":
		kind = api.KindGlobal
	case "node", "item":
		return nil, fmt.Errorf("parse task %q: kind %q is the node / item vocabulary — write `kind: object`", name, kind)
	case "item-collection":
		return nil, fmt.Errorf("parse task %q: kind %q is the item vocabulary — write `kind: collection`", name, kind)
	}
	if !api.IsKnownKind(kind) {
		return nil, fmt.Errorf("parse task %q: unknown kind %q (expected global | object | collection)", name, kind)
	}
	types := stringList(raw, "types")
	if (kind == api.KindObject || kind == api.KindCollection) && len(types) == 0 {
		return nil, fmt.Errorf("parse task %q: kind=%q requires a non-empty types list (the object types it targets)", name, kind)
	}

	audience, isString := raw["audience"].(string)
	if v, present := raw["audience"]; present && !isString {
		return nil, fmt.Errorf("parse task %q: audience %v is not one word (expected end-user | technical | demo)", name, v)
	}
	if !api.IsKnownAudience(audience) {
		return nil, fmt.Errorf("parse task %q: unknown audience %q (expected end-user | technical | demo)", name, audience)
	}

	// Filter: optional el expression over the object vocabulary. Compiled at
	// parse time so a syntax error or a name outside the vocabulary (the
	// retired `node.*` above all) is refused here, naming the task.
	filter, _ := raw["filter"].(string)
	filter = strings.TrimSpace(filter)
	if filter != "" {
		if _, err := filterExprs.Compile(filter); err != nil {
			return nil, fmt.Errorf("parse task %q: filter compile error: %w", name, err)
		}
	}

	// Extract config: explicit `config:` block only in strict mode.
	// Relaxed mode falls back to "all non-meta root fields become config" for
	// reading legacy files.
	config := extractConfig(raw, strict)

	// session.mode — how a run surfaces when this task is invoked as an action:
	// none (sync inline) | session (async journaled) | chat (interactive tab).
	// Explicit `session.mode` wins; else inferred from an agent `config.mode:
	// chat` so an existing chat task auto-routes to a Chat tab as an action.
	sessionMode := taskSessionMode(raw, config)

	// Extract schemas (JSON Schema as json.RawMessage).
	inputSchema := extractSchema(raw, "input-schema")
	outputSchema := extractSchema(raw, "output-schema")

	t := &api.Task{
		Name:              name,
		Title:             title,
		Advertised:        advertised,
		Type:              typeName,
		Description:       description,
		Icon:              icon,
		IconName:          iconName,
		Hashtags:          hashtags,
		Category:          category,
		Tags:              tags,
		Kind:              kind,
		Types:             types,
		Audience:          audience,
		SessionMode:       sessionMode,
		Filter:            filter,
		PermissionProfile: permissionProfile,
		Group:             group,
		Config:            config,
		InputSchema:       inputSchema,
		OutputSchema:      outputSchema,
		RawConfig:         raw,
		Body:              strings.TrimSpace(body),
	}
	if capability != "" && t.Config == nil {
		t.Config = map[string]any{}
	}
	if capability != "" {
		t.Config[taskapi.ConfigKeyCapability] = capability
	}
	return t, nil
}

// stringList extracts a []string from a yaml-decoded []any field.
func stringList(raw map[string]any, key string) []string {
	rawList, ok := raw[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(rawList))
	for _, v := range rawList {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// taskSessionMode resolves how a task-declared action surfaces. An explicit
// `session.mode` frontmatter block wins; otherwise a `chat` agent config
// (`config.mode: chat`) infers "chat" so an existing conversational task
// auto-routes to a Chat tab when it is invoked as an action. Empty ("" →
// treated as a headless run by the resolver) for every other task.
func taskSessionMode(raw, config map[string]any) string {
	if m, _ := raw["sessionMode"].(string); m != "" {
		return m
	}
	if m := mapStringField(raw["session"], "mode"); m != "" {
		return m
	}
	if m := mapStringField(config, "mode"); m == "chat" {
		return "chat"
	}
	return ""
}

// mapStringField reads a string key from v when v is a map — tolerating both
// `map[string]any` (JSON / yaml.v3) and `map[any]any` (some YAML decoders).
func mapStringField(v any, key string) string {
	switch m := v.(type) {
	case map[string]any:
		s, _ := m[key].(string)
		return s
	case map[any]any:
		s, _ := m[key].(string)
		return s
	}
	return ""
}

// extractConfig returns the strategy-specific config map.
//
// Strict mode: only the explicit `config:` block is used; nil if absent.
// Relaxed mode: if `config:` is missing, all non-meta root fields are
// collected as Config (the legacy-flat shape) — for migration reads only.
func extractConfig(raw map[string]any, strict bool) map[string]any {
	if explicit, ok := raw["config"].(map[string]any); ok {
		return explicit
	}
	if strict {
		return nil
	}
	// Relaxed legacy-flat fallback.
	config := make(map[string]any)
	for k, v := range raw {
		if !metaFields[k] {
			config[k] = v
		}
	}
	if len(config) == 0 {
		return nil
	}
	return config
}

// extractSchema extracts a JSON Schema from a frontmatter field.
// The value can be a map (inline schema) which gets marshaled to JSON.
func extractSchema(raw map[string]any, key string) json.RawMessage {
	v, ok := raw[key]
	if !ok {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// knownMetaKeys returns the sorted canonical meta-key set as a
// comma-separated string — used in strict-mode error messages.
func knownMetaKeys() string {
	keys := make([]string, 0, len(metaFields))
	for k := range metaFields {
		keys = append(keys, k)
	}
	// Stable order for error messages.
	sortStrings(keys)
	return strings.Join(keys, ", ")
}

func sortStrings(s []string) {
	// Tiny insertion sort to avoid a sort import.
	for i := 1; i < len(s); i++ {
		j := i
		for j > 0 && s[j-1] > s[j] {
			s[j-1], s[j] = s[j], s[j-1]
			j--
		}
	}
}

// splitFrontmatter splits a document into YAML frontmatter and body.
// Expects the format:
//
//	---
//	yaml: here
//	---
//	Markdown body here
func splitFrontmatter(content string) (frontmatter, body string, err error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("missing frontmatter delimiter (---)")
	}

	// Find the closing ---
	rest := content[3:] // skip opening ---
	rest = strings.TrimLeft(rest, "\r\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("missing closing frontmatter delimiter (---)")
	}

	frontmatter = strings.TrimSpace(rest[:idx])
	body = strings.TrimSpace(rest[idx+4:]) // skip \n---
	return frontmatter, body, nil
}

// Compile-time assertion that bytes.Buffer is still importable — Parse may
// gain strict-mode YAML KnownFields enforcement in a future revision.
var _ = bytes.Buffer{}
