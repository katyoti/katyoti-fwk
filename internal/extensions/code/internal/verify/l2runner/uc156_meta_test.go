// SPDX-License-Identifier: Apache-2.0

package l2runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestL2Scenarios_HaveUCMappingFrontmatter (UC-156) — meta-lint over every
// `.tasktest` in modules/tasks/tests/L2/. Each scenario MUST declare a
// `uc: UC-XXX` frontmatter field; the verify runner uses this to map
// scenarios to UC verdicts.
//
// Closes UC-156: "Simulator-backed scenario produces a journal that maps 1:1
// to UCs" — the mapping is enforced at the scenario-file level (a missing
// uc: field would silently drop a scenario from the verify board).
func TestL2Scenarios_HaveUCMappingFrontmatter(t *testing.T) {
	// Walk the L2 dir. The test runs from `internal/extensions/code/internal/verify/l2runner/`
	// so the relative path is up four levels then into modules/tasks/tests/L2.
	root := filepath.Join("..", "..", "..", "..", "..", "..", "modules", "tasks", "tests", "L2")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skipf("L2 fixtures dir not found at %s — skipping (test env mis-rooted)", root)
	}

	var bad []string
	var count int
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasSuffix(base, ".tasktest") {
			return nil
		}
		if base == "_template.tasktest" {
			return nil // template doesn't need a real uc: cell
		}

		count++
		data, err := os.ReadFile(path)
		if err != nil {
			bad = append(bad, path+" (read error: "+err.Error()+")")
			return nil
		}

		// Loose parse: look for a top-level `uc:` line that names a UC-XXX value.
		// Walks lines so we don't pull in a YAML dependency for a meta-lint.
		matched := false
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "uc:") {
				rest := strings.TrimSpace(strings.TrimPrefix(line, "uc:"))
				if strings.HasPrefix(rest, "UC-") {
					matched = true
				}
				break
			}
		}
		if !matched {
			bad = append(bad, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if count == 0 {
		t.Fatalf("UC-156: no .tasktest files found under %s — fixture tree missing", root)
	}
	if len(bad) > 0 {
		t.Errorf("UC-156: %d/%d .tasktest scenarios missing `uc: UC-XXX` frontmatter:\n  %s",
			len(bad), count, strings.Join(bad, "\n  "))
	}
}
