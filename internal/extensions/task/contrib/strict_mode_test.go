// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"strings"
	"testing"
)

// Verifies the strict-mode + relaxed-mode parser invariants for TASK.md
// locked alongside the canonical TaskBody DSL (2026-05-26).

func TestParse_StrictRejectsLegacyAsciiIcon(t *testing.T) {
	legacy := []byte(`---
name: my-task
type: agent
description: Test legacy icon
ascii_icon: "🔍"
---
Body
`)
	_, err := Parse(legacy)
	if err == nil {
		t.Fatal("strict mode should reject legacy ascii_icon root key; got nil")
	}
	if !strings.Contains(err.Error(), "ascii_icon") {
		t.Errorf("error should mention the offending key; got: %v", err)
	}
}

func TestParseRelaxed_AcceptsLegacyAsciiIcon(t *testing.T) {
	legacy := []byte(`---
name: my-task
type: agent
description: Test legacy icon
ascii_icon: "🔍"
---
Body
`)
	task, err := ParseRelaxed(legacy)
	if err != nil {
		t.Fatalf("relaxed mode should accept legacy keys; got: %v", err)
	}
	if task.Name != "my-task" {
		t.Errorf("Name = %q, want my-task", task.Name)
	}
}

func TestParse_AcceptsCanonicalShape(t *testing.T) {
	canonical := []byte(`---
name: doc-create
type: agent
description: Test canonical shape
icon: "📄"
iconName: file-text
hashtags: [docs, templates]
category: writing
capability: smart
config:
  mode: worker
  plugins: [standard]
---
Body
`)
	task, err := Parse(canonical)
	if err != nil {
		t.Fatalf("strict mode should accept canonical shape; got: %v", err)
	}
	if task.Icon != "📄" {
		t.Errorf("Icon = %q, want emoji", task.Icon)
	}
	if task.IconName != "file-text" {
		t.Errorf("IconName = %q, want Tabler name", task.IconName)
	}
	if task.Config["capability"] != "smart" {
		t.Errorf("Config[capability] = %v, want 'smart' (lifted from root)", task.Config["capability"])
	}
}

func TestParse_StrictRejectsUnknownRootKey(t *testing.T) {
	// A key that's neither in the canonical meta set nor a strategy
	// extension under `config:` must fail strict mode with a clear
	// pointer to where it belongs.
	bogus := []byte(`---
name: my-task
type: agent
random_field: oops
config:
  mode: worker
---
Body
`)
	_, err := Parse(bogus)
	if err == nil {
		t.Fatal("strict mode should reject unknown root key; got nil")
	}
	if !strings.Contains(err.Error(), "random_field") {
		t.Errorf("error should mention the offending key; got: %v", err)
	}
	if !strings.Contains(err.Error(), "config:") {
		t.Errorf("error should hint that strategy-specific keys belong under config:; got: %v", err)
	}
}
