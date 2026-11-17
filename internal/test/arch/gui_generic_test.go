// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// KaTyoti's GUI stays generic: what a pack is - a product, a client
// its studies, dossiers and cadrage, the connectors it reads - comes from the
// pack's contributions, never from ktw-gui, the display library or the default
// themes. Comments may name them (they say where a need came from); code,
// strings and locale texts may not.
var packNames = regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}_])(archi-?factory|dossiers?|cadrages?|stud(?:y|ies)|[ée]tudes?|confluence|leanix|dynatrace)(?:$|[^\p{L}\p{N}_])`)

var archDossier = regexp.MustCompile(`(?i)dossiers? d['’]architecture`)

// The roots the GUI is made of, a test or a demo page excepted.
func genericGUIFiles(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	var out []string
	walk := func(dir string, exts ...string) {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "dist", "build", "wailsjs", "tests", "testdata", "dev":
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(rel, "_test.go") || strings.Contains(rel, ".test.") {
				return nil
			}
			for _, e := range exts {
				if strings.HasSuffix(rel, e) {
					out = append(out, rel)
					break
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	walk("app/ktw-gui", ".go", ".ts", ".tsx", ".json", ".css")
	walk("app/ktw-display-lib/src", ".ts", ".tsx", ".json", ".css", ".yaml")
	walk("internal/extensions/slidedeck/contrib/themes", ".yaml", ".css")
	sort.Strings(out)
	return out
}

func TestGUINamesNoPack(t *testing.T) {
	root := repoRoot(t)
	files := genericGUIFiles(t)
	if len(files) < 100 {
		t.Fatalf("only %d GUI files found: the roots are wrong", len(files))
	}
	for _, rel := range files {
		if strings.HasSuffix(rel, "package.json") || strings.HasSuffix(rel, "package-lock.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		code := stripComments(string(raw), filepath.Ext(rel))
		french := strings.Contains(rel, "/locales/fr/")
		for i, line := range strings.Split(code, "\n") {
			for _, m := range packNames.FindAllStringSubmatch(line, -1) {
				// In French a dossier is any folder; only ArchiFactory's
				// dossier d'architecture is the pack's.
				if french && strings.HasPrefix(strings.ToLower(m[1]), "dossier") && !archDossier.MatchString(line) {
					continue
				}
				t.Errorf("%s:%d names %q - a pack's word belongs in its contributions: %s", rel, i+1, m[1], strings.TrimSpace(line))
			}
		}
	}
}

// Planification, the inbox and the scheduler are packs' views:
// ktw-gui gains no new such code, and today's - listed here - only shrinks;
// a file that no longer is one leaves the list.
var guiDomainCodeToday = []string{
	"app/ktw-gui/frontend/src/hooks/useInboxLauncher.ts",
	"app/ktw-gui/frontend/src/sections/scheduler/HistoryView.tsx",
	"app/ktw-gui/frontend/src/sections/scheduler/LiveView.tsx",
	"app/ktw-gui/frontend/src/sections/scheduler/QueuesView.tsx",
	"app/ktw-gui/frontend/src/sections/scheduler/RunDrawer.tsx",
	"app/ktw-gui/frontend/src/sections/scheduler/SchedulerConsole.tsx",
	"app/ktw-gui/frontend/src/sections/scheduler/model.ts",
	"app/ktw-gui/frontend/src/sections/scheduler/useScheduler.ts",
	"app/ktw-gui/frontend/src/sections/workspace/RequestsView.tsx",
}

var (
	domainPath = regexp.MustCompile(`(?i)(inbox|scheduler|planif|increment|backlog|kanban|requests?view|todos?)`)
	domainCode = regexp.MustCompile(`(?:requests|todos|increments|releases|plans|planning|inbox|scheduler)/[a-z-]+['"` + "`" + `]|Workspace(?:Inbox|Requests|Todos|Increment)|planification`)
)

func TestGUIGainsNoPlanificationCode(t *testing.T) {
	root := repoRoot(t)
	today := map[string]bool{}
	for _, f := range guiDomainCodeToday {
		today[f] = true
	}
	found := map[string]bool{}
	for _, rel := range genericGUIFiles(t) {
		if !strings.HasPrefix(rel, "app/ktw-gui/") || strings.HasSuffix(rel, ".json") || strings.HasSuffix(rel, ".css") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if domainPath.MatchString(filepath.Base(rel)) || domainPath.MatchString(filepath.Dir(rel)) || domainCode.MatchString(stripComments(string(raw), filepath.Ext(rel))) {
			found[rel] = true
			if !today[rel] {
				t.Errorf("%s is new planification, inbox or scheduler code in ktw-gui - it belongs to the pack that owns the domain", rel)
			}
		}
	}
	for _, f := range guiDomainCodeToday {
		if !found[f] {
			t.Errorf("%s is no longer planification, inbox or scheduler code: remove it from guiDomainCodeToday", f)
		}
	}
}

// stripComments blanks a source's comments, its strings kept, line numbers
// unchanged: // and /* */ in Go, TypeScript and CSS, # in YAML. Quotes are
// read line by line, so a regex literal holding one misleads a line at most.
func stripComments(src, ext string) string {
	lines := strings.Split(src, "\n")
	switch ext {
	case ".json":
		return src
	case ".yaml", ".yml":
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "#") {
				lines[i] = ""
			}
		}
		return strings.Join(lines, "\n")
	}
	inBlock := false
	for n, l := range lines {
		var b strings.Builder
		var quote byte
		for i := 0; i < len(l); i++ {
			c := l[i]
			switch {
			case inBlock:
				if c == '*' && i+1 < len(l) && l[i+1] == '/' {
					inBlock = false
					i++
				}
			case quote != 0:
				b.WriteByte(c)
				if c == '\\' && i+1 < len(l) {
					i++
					b.WriteByte(l[i])
				} else if c == quote {
					quote = 0
				}
			case c == '"' || c == '`' || (c == '\'' && ext != ".css"):
				quote = c
				b.WriteByte(c)
			case c == '/' && i+1 < len(l) && l[i+1] == '/' && ext != ".css":
				i = len(l)
			case c == '/' && i+1 < len(l) && l[i+1] == '*':
				inBlock = true
				i++
			default:
				b.WriteByte(c)
			}
		}
		lines[n] = b.String()
	}
	return strings.Join(lines, "\n")
}
