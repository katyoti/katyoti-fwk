// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"strings"
)

// skillmd_format.go — the SKILL.md shape, where both sides can see it.
//
// A skill file is frontmatter plus a body. Splitting the two is a fact about
// the FORMAT, not about either the producer that writes it or the consumer that
// reads it, so it lives with the format's other definitions rather than inside
// one of them — where the other could not reach it without an import cycle.

// SplitSkillMD separates a SKILL.md into its frontmatter and its body.
//
// Both are trimmed. An absent or unterminated frontmatter delimiter is an
// error: a file without one is not a skill, and guessing where the metadata
// ended would silently turn part of it into instructions.
func SplitSkillMD(content string) (frontmatter, body string, err error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("missing frontmatter delimiter (---)")
	}
	rest := strings.TrimLeft(content[3:], "\r\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("missing closing frontmatter delimiter (---)")
	}
	return strings.TrimSpace(rest[:idx]), strings.TrimSpace(rest[idx+4:]), nil
}
