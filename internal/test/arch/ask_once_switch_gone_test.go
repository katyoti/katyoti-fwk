// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// « À faire » is the one screen for everyone (the person's
// decision): IN-039's switch is retired - nothing reads it, no code keeps the
// way things were asked with it off - and the GUI's built-in Inbox panel is
// deleted: the rail's Inbox opens the page the backend names. None of it comes
// back: no code, script or walk names the switch or the panel.
var askOnceSwitch = regexp.MustCompile(`AskOnceOn|SetAskOnce|AskOnceEnv|KTW_IN039_DISPLAY|In039DisplayEnv|useDisplaySwitch\(\s*['"]in039['"]\s*\)|InboxPanel`)

func TestIN039sSwitchAndTheBuiltInInboxStayGone(t *testing.T) {
	root := repoRoot(t)
	checked := 0
	walk := func(dir string, keep func(rel string) bool) {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "dist" || d.Name() == "wailsjs" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(rel, "_test.go") || strings.HasSuffix(rel, ".test.ts") || !keep(rel) {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			checked++
			if m := askOnceSwitch.FindString(string(raw)); m != "" {
				t.Errorf("%s names %q - the ask-once switch and the built-in Inbox are retired: « À faire » is the one screen", rel, m)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	code := func(rel string) bool {
		return strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, ".ts") || strings.HasSuffix(rel, ".tsx") || strings.HasSuffix(rel, ".sh")
	}
	walk("internal", code)
	walk("app/ktw-gui", code)
	walk("scripts", code)
	if checked < 2000 {
		t.Fatalf("checked %d files: the roots are wrong", checked)
	}
}
