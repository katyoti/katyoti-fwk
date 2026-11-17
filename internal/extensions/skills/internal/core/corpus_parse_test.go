// SPDX-License-Identifier: Apache-2.0

package core

import (
	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCorpusParse_AllInRepoSkillsStrictGreen walks every in-repo skill
// markdown file and asserts ParseSkillMD (strict mode) accepts it.
// Closes the skill-migration-sweep success criterion that every
// deployed skill parses green under strict-mode validation post-sweep.
//
// Two filename conventions are covered:
//
//   - `SKILL.md` — the per-folder convention (`<dir>/SKILL.md`)
//   - `app/<sidecar>/embed/skills/<name>.md` — the flat-file convention
//     used by every shipped sidecar today
//
// Skipped folders:
//   - `_ktw_workspace/` — external clones, not part of the shipped repo
//   - `vendor/`, `node_modules/`, `.git/` — third-party / VCS
//
// Failure mode: a new skill file added to the repo that violates the
// canonical 9-key schema will fail this test, surfacing drift before it
// ships. Add the failing file to the migration-sweep checklist or fix
// the file's frontmatter.
func TestCorpusParse_AllInRepoSkillsStrictGreen(t *testing.T) {
	// Walk from the repo root. cwd during `go test` is the package dir,
	// so step up to the repo root by looking for go.mod.
	root := findRepoRoot(t)

	var skillFiles []string
	skipDirs := map[string]bool{
		"_ktw_workspace": true,
		"vendor":         true,
		"node_modules":   true,
		".git":           true,
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // a throwaway app rendered and removed by another test
			}
			return err
		}
		if info.IsDir() && (strings.HasPrefix(info.Name(), "_") || strings.HasPrefix(info.Name(), ".")) {
			return filepath.SkipDir // throwaway or hidden — never corpus
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() == "SKILL.md" {
			skillFiles = append(skillFiles, path)
			return nil
		}
		// Flat-file skill conventions:
		//   - app/<sidecar>/embed/skills/<name>.md (sidecar binary embeds)
		//   - internal/extensions/<pack>/local/embed/skills/<name>.md
		//     (method-pack in-process embeds, e.g. the methods pack —
		//     these are bundled into the CWM host AND served by the pack's
		//     sidecar, so they must hold the canonical frontmatter too)
		rel, _ := filepath.Rel(root, path)
		relSlash := filepath.ToSlash(rel)
		isSidecarSkill := strings.HasPrefix(relSlash, "app/") &&
			strings.Contains(relSlash, "/embed/skills/")
		isPackSkill := strings.HasPrefix(relSlash, "internal/extensions/") &&
			strings.Contains(relSlash, "/local/embed/skills/")
		if strings.HasSuffix(relSlash, ".md") && (isSidecarSkill || isPackSkill) {
			// Skip reference markdown — files under `references/`
			// (loaded via `skills-get-reference` from the skill
			// body) are plain markdown, not skill definitions. They
			// don't carry the frontmatter ParseSkillMD requires;
			// treating them as skills would force every reference
			// to grow useless metadata.
			if strings.Contains(relSlash, "/references/") {
				return nil
			}
			skillFiles = append(skillFiles, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}

	if len(skillFiles) == 0 {
		t.Skip("no in-repo SKILL.md files found")
	}

	t.Logf("parsing %d in-repo SKILL.md files in strict mode", len(skillFiles))

	for _, f := range skillFiles {
		f := f
		rel, _ := filepath.Rel(root, f)
		t.Run(rel, func(t *testing.T) {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			md, err := api.ParseSkillMD(data)
			if err != nil {
				t.Errorf("strict parse failed: %v\n(if this is a new legacy SKILL.md, add to the migration sweep instead of disabling this test)", err)
				return
			}
			if md.Frontmatter.Name == "" {
				t.Errorf("parsed meta has empty Name")
			}
		})
	}
}

// findRepoRoot walks up from cwd until it finds go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir || !strings.Contains(parent, string(filepath.Separator)) {
			t.Fatalf("repo root not found from %s", wd)
		}
		dir = parent
	}
}
