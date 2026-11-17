// SPDX-License-Identifier: Apache-2.0

package core

import (
	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
	"strings"
	"testing"
)

// Verifies the strict-mode + relaxed-mode parser invariants of the
// frontmatter contract.

func TestParseSkillMD_StrictRejectsLegacyKeys(t *testing.T) {
	// `ascii_icon` is legacy (renamed to `icon` for emoji); strict mode
	// must reject it. `advertised` is part of the canonical 9-key schema
	// and is accepted.
	legacy := []byte(`---
name: doc-create
description: Test legacy
ascii_icon: "📄"
---
Body
`)
	_, err := api.ParseSkillMD(legacy)
	if err == nil {
		t.Fatal("strict mode should reject legacy ascii_icon key; got nil")
	}
	if !strings.Contains(err.Error(), "ascii_icon") {
		t.Errorf("error should mention the offending unknown key; got: %v", err)
	}
}

func TestParseSkillMDRelaxed_AcceptsLegacyKeys(t *testing.T) {
	legacy := []byte(`---
name: doc-create
description: Test legacy
ascii_icon: "📄"
---
Body
`)
	md, err := api.ParseSkillMDRelaxed(legacy)
	if err != nil {
		t.Fatalf("relaxed mode should accept legacy keys; got: %v", err)
	}
	if md.Frontmatter.Name != "doc-create" {
		t.Errorf("Name = %q, want doc-create", md.Frontmatter.Name)
	}
	if md.Frontmatter.Icon != "" {
		t.Errorf("Icon should be empty (ascii_icon ignored, not mapped); got %q", md.Frontmatter.Icon)
	}
}

func TestParseSkillMD_AcceptsCanonical9Keys(t *testing.T) {
	canonical := []byte(`---
name: doc-create
description: Test canonical 9-key shape
requires:
  - workspace
  - skill:template-engine
provides:
  - task
  - cli
category: writing
hashtags: [docs, templates]
icon: "📄"
iconName: file-text
advertised: true
---
Body
`)
	md, err := api.ParseSkillMD(canonical)
	if err != nil {
		t.Fatalf("strict mode should accept the canonical 9 keys; got: %v", err)
	}
	if md.Frontmatter.Icon != "📄" {
		t.Errorf("Icon = %q, want emoji", md.Frontmatter.Icon)
	}
	if md.Frontmatter.IconName != "file-text" {
		t.Errorf("IconName = %q, want Tabler name", md.Frontmatter.IconName)
	}
	if len(md.Frontmatter.Provides) != 2 {
		t.Errorf("Provides = %v, want [task cli]", md.Frontmatter.Provides)
	}
	if !md.Frontmatter.IsAdvertised() {
		t.Errorf("IsAdvertised = false, want true (advertised: true)")
	}
}

func TestParseSkillMD_AdvertisedDefaultsTrue(t *testing.T) {
	// Omitting `advertised:` should default to advertised (visible).
	noAdvertised := []byte(`---
name: skill-a
description: No advertised field
---
Body
`)
	md, err := api.ParseSkillMD(noAdvertised)
	if err != nil {
		t.Fatalf("strict parse failed: %v", err)
	}
	if !md.Frontmatter.IsAdvertised() {
		t.Errorf("IsAdvertised default = false, want true")
	}
}

func TestParseSkillMD_AdvertisedFalseHides(t *testing.T) {
	hidden := []byte(`---
name: skill-b
description: Internal helper
advertised: false
---
Body
`)
	md, err := api.ParseSkillMD(hidden)
	if err != nil {
		t.Fatalf("strict parse failed: %v", err)
	}
	if md.Frontmatter.IsAdvertised() {
		t.Errorf("IsAdvertised = true with advertised:false, want false")
	}
}
