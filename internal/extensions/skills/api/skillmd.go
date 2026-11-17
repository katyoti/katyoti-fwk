// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// SkillMD is a parsed SKILL.md file: the frontmatter, typed, and the
// markdown body after it.
type SkillMD struct {
	// Frontmatter is the file's YAML header, decoded into the ONE struct the
	// skill contract is stated in (`mbapi.SkillBody`): the same fields a
	// skill entry carries as its data layer and the skill data-type (the
	// shape of that struct) validates.
	Frontmatter mbapi.SkillBody
	// Provider is NOT in the frontmatter — the loader records which provider
	// the file came from, after parsing.
	Provider string
	// Instructions is the markdown body after the frontmatter (L1).
	Instructions string
}

// Standard layout of a skill:
//
//	skill-name/
//	├── SKILL.md          # Required: frontmatter + instructions body
//	├── references/       # Optional: documentation served via GetReference
//	└── ...               # Any additional files
//
// Scripts shipped by a skill are addressed via the slot-method URI
// `<provider>:script-service/default/<skill>__<script>`, not via the
// frontmatter (decision 2026-05-26 — see scripts-as-slot-methods).

// ParseSkillMD parses a SKILL.md file in strict mode — unknown frontmatter
// keys cause a parse error. Use ParseSkillMDRelaxed for migration tooling
// that needs to read legacy files.
func ParseSkillMD(data []byte) (*SkillMD, error) {
	return parseSkillMD(data, true)
}

// ParseSkillMDRelaxed parses a SKILL.md file in relaxed mode — unknown
// frontmatter keys are silently ignored. Intended only for the
// skill-migration-sweep that reads legacy files before rewriting them.
// Removed once the sweep lands per NO LEGACY.
func ParseSkillMDRelaxed(data []byte) (*SkillMD, error) {
	return parseSkillMD(data, false)
}

func parseSkillMD(data []byte, strict bool) (*SkillMD, error) {
	frontmatter, body, err := SplitSkillMD(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse skill: %w", err)
	}

	var meta mbapi.SkillBody
	dec := yaml.NewDecoder(bytes.NewReader([]byte(frontmatter)))
	dec.KnownFields(strict)
	if err := dec.Decode(&meta); err != nil {
		return nil, fmt.Errorf("parse skill frontmatter: %w", err)
	}

	if err := validateMeta(&meta); err != nil {
		return nil, fmt.Errorf("parse skill: %w", err)
	}

	return &SkillMD{Frontmatter: meta, Instructions: strings.TrimSpace(body)}, nil
}

// validateMeta checks that skill frontmatter conforms to skill-authoring rules.
func validateMeta(m *mbapi.SkillBody) error {
	if m.Name == "" {
		return fmt.Errorf("missing required field 'name'")
	}
	if len(m.Name) > 64 {
		return fmt.Errorf("name %q exceeds 64 characters", m.Name)
	}
	for _, c := range m.Name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return fmt.Errorf("name %q: invalid character %q (allowed: lowercase, digits, hyphens)", m.Name, string(c))
		}
	}
	if m.Name[0] == '-' || m.Name[len(m.Name)-1] == '-' {
		return fmt.Errorf("name %q: must not start or end with a hyphen", m.Name)
	}
	if m.Description == "" {
		return fmt.Errorf("missing required field 'description' for skill %q", m.Name)
	}
	if len(m.Description) > 1024 {
		return fmt.Errorf("description for skill %q exceeds 1024 characters", m.Name)
	}
	return nil
}
