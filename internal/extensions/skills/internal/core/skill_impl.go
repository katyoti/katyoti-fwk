// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"path"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
)

type loaderSkill struct {
	skill  *api.SkillMD
	loader loader.Loader
	dir    string
}

func NewSkillFromBytes(ldr loader.Loader, dir string, data []byte) (api.Skill, error) {
	s, err := api.ParseSkillMD(data)
	if err != nil {
		return nil, err
	}
	return &loaderSkill{skill: s, loader: ldr, dir: dir}, nil
}

func (s *loaderSkill) Title() api.SkillInfo {
	return api.SkillInfo{
		Name:        s.skill.Frontmatter.Name,
		Provider:    s.skill.Provider,
		Description: s.skill.Frontmatter.Description,
		Requires:    s.skill.Frontmatter.Requires,
		Icon:        s.skill.Frontmatter.Icon,
		IconName:    s.skill.Frontmatter.IconName,
		Hashtags:    s.skill.Frontmatter.Hashtags,
		Category:    s.skill.Frontmatter.Category,
		Provides:    s.skill.Frontmatter.Provides,
	}
}

func (s *loaderSkill) Instructions() string {
	return s.skill.Instructions
}

func (s *loaderSkill) Reference(p string) (string, error) {
	if err := validatePath(p); err != nil {
		return "", err
	}
	lower := strings.ToLower(path.Clean(p))
	if lower == "skill.md" {
		return "", fmt.Errorf("path %q: SKILL.md is not a reference (use ResourceService.Get on the skill URI instead)", p)
	}
	data, err := s.loader.ReadFile(path.Join(s.dir, p))
	if err != nil {
		return "", fmt.Errorf("reference %q not found: %w", p, err)
	}
	if data == nil {
		return "", fmt.Errorf("reference %q not found in skill %q", p, s.skill.Frontmatter.Name)
	}
	return string(data), nil
}

// A skill runs no scripts: standalone executables are `scripts` slot
// methods of their sidecar (see `internal/extensions/scripts/`), and the
// frontmatter carries no script list.

func validatePath(p string) error {
	cleaned := path.Clean(p)
	if strings.HasPrefix(cleaned, "..") || strings.HasPrefix(cleaned, "/") || strings.Contains(cleaned, "\\") {
		return fmt.Errorf("invalid path %q: must be relative within skill directory", p)
	}
	return nil
}
