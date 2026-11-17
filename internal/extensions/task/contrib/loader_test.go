// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"strings"
	"testing"
)

func TestParse_BasicFields(t *testing.T) {
	input := `---
name: my-task
description: A test task
type: task
config:
  capability: smart
  max_turns: 30
---
# Task Body

Instructions here.
`
	task, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if task.Name != "my-task" {
		t.Errorf("Name = %q, want %q", task.Name, "my-task")
	}
	if task.Type != "task" {
		t.Errorf("Type = %q, want %q", task.Type, "task")
	}
	if task.Description != "A test task" {
		t.Errorf("Description = %q", task.Description)
	}

	// Strategy-specific fields are in Config
	if task.Config["capability"] != "smart" {
		t.Errorf("Config[capability] = %v, want smart", task.Config["capability"])
	}
	if task.Config["max_turns"] != 30 {
		t.Errorf("Config[max_turns] = %v, want 30", task.Config["max_turns"])
	}

	if !strings.Contains(task.Body, "# Task Body") {
		t.Errorf("Body should contain title, got: %s", task.Body[:30])
	}
}

func TestParse_TitleAndAdvertised(t *testing.T) {
	// title is an optional display label; advertised is a pointer-to-bool
	// visibility switch (nil = default true). Both are canonical meta keys
	// accepted by strict mode.
	input := `---
name: my-task
title: My Lovely Task
type: task
advertised: false
description: A test task
---
Body
`
	task, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if task.Title != "My Lovely Task" {
		t.Errorf("Title = %q, want %q", task.Title, "My Lovely Task")
	}
	if task.Advertised == nil || *task.Advertised != false {
		t.Errorf("Advertised = %v, want explicit false", task.Advertised)
	}
	if task.IsAdvertised() {
		t.Error("IsAdvertised() = true, want false for advertised: false")
	}
}

func TestParse_AdvertisedDefaultsTrueWhenOmitted(t *testing.T) {
	input := `---
name: plain-task
type: task
---
Body
`
	task, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if task.Title != "" {
		t.Errorf("Title = %q, want empty when omitted", task.Title)
	}
	if task.Advertised != nil {
		t.Errorf("Advertised = %v, want nil when omitted", task.Advertised)
	}
	if !task.IsAdvertised() {
		t.Error("IsAdvertised() = false, want true by default")
	}
}

func TestParse_StrictRejectsLegacyFlatFormat(t *testing.T) {
	// Legacy format: config fields at root (no config: block). Strict mode
	// (the default) MUST reject this — strategy-specific keys belong under
	// `config:` only. The relaxed companion still accepts it for the
	// one-shot migration sweep.
	input := `---
name: legacy-task
type: task
plugins:
  - standard
---
Body
`
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("strict Parse should reject legacy flat frontmatter (unknown root key `plugins`); got nil")
	}
	if !strings.Contains(err.Error(), "plugins") {
		t.Errorf("error should mention the offending root key; got: %v", err)
	}
}

func TestParseRelaxed_AcceptsLegacyFlatFormat(t *testing.T) {
	// Relaxed mode fall-through collects non-meta root fields into Config —
	// the legacy-flat shape — so the migration sweep can read pre-canonical
	// files before rewriting them.
	input := `---
name: legacy-task
type: task
plugins:
  - standard
---
Body
`
	task, err := ParseRelaxed([]byte(input))
	if err != nil {
		t.Fatalf("ParseRelaxed failed: %v", err)
	}
	plugins, ok := task.Config["plugins"].([]any)
	if !ok {
		t.Fatalf("Config[plugins] is not []any, got %T", task.Config["plugins"])
	}
	if len(plugins) != 1 || plugins[0] != "standard" {
		t.Errorf("Config[plugins] = %v, want [standard]", plugins)
	}
}

func TestParse_MissingName(t *testing.T) {
	input := `---
description: No name
---
Body
`
	_, err := Parse([]byte(input))
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestParse_MissingFrontmatter(t *testing.T) {
	_, err := Parse([]byte("# Just markdown"))
	if err == nil {
		t.Error("expected error for missing frontmatter")
	}
}

func TestParse_NoType(t *testing.T) {
	input := `---
name: no-type-task
config:
  capability: fast
---
Body
`
	_, err := Parse([]byte(input))
	if err == nil {
		t.Error("expected error for missing type")
	}
}

func TestParse_ConfigWithAllFieldTypes(t *testing.T) {
	input := `---
name: full-task
type: task
description: Full test
config:
  capability: smart
  plugins:
    - workspace
    - standard
  skills:
    - helper
  context:
    db_url: "postgres://localhost"
---
Body
`
	task, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if task.Config["capability"] != "smart" {
		t.Errorf("Config[capability] = %v", task.Config["capability"])
	}

	plugins, ok := task.Config["plugins"].([]any)
	if !ok {
		t.Fatalf("Config[plugins] is not []any, got %T", task.Config["plugins"])
	}
	if len(plugins) != 2 {
		t.Errorf("plugins count = %d, want 2", len(plugins))
	}

	ctx, ok := task.Config["context"].(map[string]any)
	if !ok {
		t.Fatalf("Config[context] is not map[string]any, got %T", task.Config["context"])
	}
	if ctx["db_url"] != "postgres://localhost" {
		t.Errorf("context[db_url] = %v", ctx["db_url"])
	}
}

func TestParse_WorkflowWithSteps(t *testing.T) {
	input := `---
name: my-workflow
type: workflow
config:
  steps:
    - name: ask
      task:
        type: human-gate
        message: "Question?"
    - name: do-work
      taskName: research-agent
      input:
        topic: "steps.ask.output"
---
Workflow description.
`
	task, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if task.Type != "workflow" {
		t.Errorf("Type = %q, want workflow", task.Type)
	}

	// Steps are in Config
	steps, ok := task.Config["steps"].([]any)
	if !ok {
		t.Fatalf("Config[steps] is not []any, got %T", task.Config["steps"])
	}
	if len(steps) != 2 {
		t.Fatalf("steps count = %d, want 2", len(steps))
	}

	step0, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("step[0] is not map, got %T", steps[0])
	}
	if step0["name"] != "ask" {
		t.Errorf("step[0].name = %v", step0["name"])
	}
}

func TestParse_Schemas(t *testing.T) {
	input := `---
name: typed-task
type: agent
config:
  capability: fast
input-schema:
  type: object
  properties:
    topic:
      type: string
  required:
    - topic
output-schema:
  type: object
  properties:
    summary:
      type: string
---
Body
`
	task, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if task.InputSchema == nil {
		t.Fatal("InputSchema is nil")
	}
	if task.OutputSchema == nil {
		t.Fatal("OutputSchema is nil")
	}
	if !strings.Contains(string(task.InputSchema), `"topic"`) {
		t.Errorf("InputSchema doesn't contain 'topic': %s", task.InputSchema)
	}
	if !strings.Contains(string(task.OutputSchema), `"summary"`) {
		t.Errorf("OutputSchema doesn't contain 'summary': %s", task.OutputSchema)
	}
}
