// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every method the GUI's Go host calls is named by its URI, every segment
// kebab (the URI rule: `ktw-agents:cwm/object-list`, never
// `ktw-agents:cwm/ObjectList`) - the name the method is declared, and its
// timeout read, under. The frontend sources have the same guard
// (frontend/tests/method-paths-kebab.test.ts).

// A quoted method URI (`provider:service/method`, nested paths and an
// `(instance)` included) with the quote, a colon or a blank after it.
var quotedMethodPath = regexp.MustCompile("[\"`](ktw-[a-z0-9-]+:[a-zA-Z][A-Za-z0-9-]*(?:\\([^)\"`]*\\))?(?:/[A-Za-z0-9-]+)+)[\"`:\\s]")

// pascalMethodPaths returns the quoted method paths of text that carry an
// upper-case letter in their service or method path.
func pascalMethodPaths(text string) []string {
	var found []string
	for _, m := range quotedMethodPath.FindAllStringSubmatch(text, -1) {
		if strings.ToLower(m[1]) != m[1] {
			found = append(found, m[1])
		}
	}
	return found
}

func TestTheGuardSeesAPascalCaseMethodPath(t *testing.T) {
	cases := map[string][]string{
		`sender.Call(ctx, "ktw-agents:cwm/ObjectList", in)`:    {"ktw-agents:cwm/ObjectList"},
		"case `ktw-hub:schema/GetJSONSchema`:":                 {"ktw-hub:schema/GetJSONSchema"},
		`fmt.Errorf("ktw-agents:object-pane/Detail: %w", err)`: {"ktw-agents:object-pane/Detail"},
		`sender.Call(ctx, "ktw-agents:cwm/object-list", in)`:   nil,
		`"ktw-agents:/panel-type/progress"`:                    nil,
		`"https://example.com/Some/Path"`:                      nil,
	}
	for text, want := range cases {
		got := pascalMethodPaths(text)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("pascalMethodPaths(%s) = %v, want %v", text, got, want)
		}
	}
}

func TestEveryGoMethodPathOfTheGUIIsKebab(t *testing.T) {
	var offenders []string
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != "." && (d.Name() == "frontend" || d.Name() == "build" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || p == "method_paths_kebab_test.go" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, path := range pascalMethodPaths(string(b)) {
			offenders = append(offenders, filepath.ToSlash(p)+": "+path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Errorf("method paths not kebab (name them as declared, every segment kebab):\n%s", strings.Join(offenders, "\n"))
	}
}

// The live scripts (frontend/tests/live/*.sh) call methods unquoted
// (`call ktw-agents:cwm/object-list '{}'`): a method URI starting a word.
var scriptMethodPath = regexp.MustCompile(`(?:^|[\s"'=(])(ktw-[a-z0-9-]+:[a-zA-Z][A-Za-z0-9-]*(?:\([^)\s"']*\))?(?:/[A-Za-z0-9-]+)+)`)

func pascalScriptMethodPaths(text string) []string {
	var found []string
	for _, m := range scriptMethodPath.FindAllStringSubmatch(text, -1) {
		if strings.ToLower(m[1]) != m[1] {
			found = append(found, m[1])
		}
	}
	return found
}

func TestTheScriptGuardSeesAPascalCaseMethodPath(t *testing.T) {
	cases := map[string][]string{
		`x="$(call ktw-agents:cwm/ObjectList '{}')"`:     {"ktw-agents:cwm/ObjectList"},
		`call ktw-agents:increments/Unship "{\"id\":1}"`: {"ktw-agents:increments/Unship"},
		`call ktw-agents:cwm/object-list '{}'`:           nil,
		`open ktw-compose:/skill/test-gui`:               nil,
		`"key":"notes/Cartographie Acme.md"`:             nil,
	}
	for text, want := range cases {
		got := pascalScriptMethodPaths(text)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("pascalScriptMethodPaths(%s) = %v, want %v", text, got, want)
		}
	}
}

func TestEveryMethodPathOfTheGUILiveScriptsIsKebab(t *testing.T) {
	liveDir := filepath.Join("frontend", "tests", "live")
	if _, err := os.Stat(liveDir); errors.Is(err, fs.ErrNotExist) {
		// The open-source export does not ship the live scripts.
		t.Skip("frontend/tests/live is not in this tree")
	}
	scripts, err := filepath.Glob(filepath.Join(liveDir, "*.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) == 0 {
		t.Fatal("no live script found under frontend/tests/live")
	}
	var offenders []string
	for _, p := range scripts {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range pascalScriptMethodPaths(string(b)) {
			offenders = append(offenders, filepath.ToSlash(p)+": "+path)
		}
	}
	if len(offenders) > 0 {
		t.Errorf("live scripts call methods not kebab (name them as declared, every segment kebab):\n%s", strings.Join(offenders, "\n"))
	}
}
