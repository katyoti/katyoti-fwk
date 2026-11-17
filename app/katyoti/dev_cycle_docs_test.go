// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// : the skills and pages describe the developer cycle as it
// works. The GUI is launched from the dev installation (`katyoti gui`), never
// from bin/ - a bin/ program belongs to no installation and is refused its hub
// - and no session starts or stops the workshop's hub from
// bin/. A line may still name these forms to warn against them.

// staleCycle are the instructions the developer cycle no longer has.
var staleCycle = []string{
	"bin/ktw-gui",
	"bin/katyoti.exe universe stop",
	"bin/katyoti universe stop",
	"--hub keep todos modes",
}

// warns is a line that names a stale form to say not to use it.
func warns(line string) bool {
	l := strings.ToLower(line)
	for _, w := range []string{"never", "refused", "no way in", "not ", "instead of"} {
		if strings.Contains(l, w) {
			return true
		}
	}
	return false
}

func TestTheSkillsAndPagesDescribeTheDevCycleAsItWorks(t *testing.T) {
	root := repoRootOf(t)
	var files []string
	for _, glob := range []string{
		".claude/skills/*/SKILL.md",
		"app/*/embed/skills/*.md", "app/*/embed/skills/*/*.md",
		"CLAUDE.md", "CONTRIBUTING.md", "USER_GUIDE.md", "DEV_GUIDE.md",
		"modules/*/*.md",
	} {
		m, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(glob)))
		files = append(files, m...)
	}
	if len(files) < 50 {
		t.Fatalf("only %d skills and pages found - the globs are wrong, not the docs", len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			for _, stale := range staleCycle {
				if strings.Contains(line, stale) && !warns(line) {
					rel, _ := filepath.Rel(root, f)
					t.Errorf("%s:%d tells to use %q - the developer cycle launches with `katyoti gui` and starts hubs from the installation (modules/katyoti/developer-cycle.md)", filepath.ToSlash(rel), i+1, stale)
				}
			}
		}
	}
}

func repoRootOf(t *testing.T) string {
	wd, _ := os.Getwd()
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			t.Fatal("no go.mod above the test")
		}
	}
}
