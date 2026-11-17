// SPDX-License-Identifier: Apache-2.0

package guide

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
)

// repoRoot walks up from the test's folder to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test")
		}
		dir = parent
	}
}

var (
	reRegistryEntry = regexp.MustCompile(`(?m)^\s*'([a-z0-9.:-]+)':\s*\{\s*file:\s*'([^']+)'(.*)$`)
	reRegistryLabel = regexp.MustCompile(`\blabel:\s*'([^']+)'`)
)

// registryAnchor is one entry of the GUI's anchor registry: the file placing
// it, the i18n key of the text it shows, and whether that text is a
// contribution's instead.
type registryAnchor struct {
	file        string
	label       string
	contributed bool
}

// readRegistry parses frontend/src/guide/anchors.ts (one entry per line).
func readRegistry(t *testing.T, root string) map[string]registryAnchor {
	t.Helper()
	reg, err := os.ReadFile(filepath.Join(root, "app", "ktw-gui", "frontend", "src", "guide", "anchors.ts"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]registryAnchor{}
	for _, m := range reRegistryEntry.FindAllStringSubmatch(string(reg), -1) {
		a := registryAnchor{file: m[2], contributed: strings.Contains(m[3], "contributed: true")}
		if l := reRegistryLabel.FindStringSubmatch(m[3]); l != nil {
			a.label = l[1]
		}
		out[m[1]] = a
	}
	if len(out) < 20 {
		t.Fatalf("read %d anchors from the registry — the parse is wrong, not the registry", len(out))
	}
	return out
}

// TestRecipeTargetsAreRegisteredAnchors holds the seam between the recipes
// and the GUI (modules/guide, OD-007): every target of every recipe a binary
// ships names an anchor of the GUI's registry (frontend/src/guide/anchors.ts),
// and every anchor of the registry is placed by the file it names — so a
// renamed or removed control fails here, not in front of a person.
func TestRecipeTargetsAreRegisteredAnchors(t *testing.T) {
	root := repoRoot(t)
	front := filepath.Join(root, "app", "ktw-gui", "frontend", "src")
	anchors := readRegistry(t, root)
	// Every registered anchor is placed by its file: as a literal, or as the
	// value an expression gives (data-guide={cond ? 'id' : undefined}).
	for id, a := range anchors {
		file := a.file
		src, err := os.ReadFile(filepath.Join(front, filepath.FromSlash(file)))
		if err != nil {
			t.Errorf("anchor %s: file %s: %v", id, file, err)
			continue
		}
		placed := strings.Contains(string(src), `data-guide="`+id+`"`) ||
			regexp.MustCompile(`data-guide=\{[^}]*'`+regexp.QuoteMeta(id)+`'`).Match(src)
		if !placed && strings.HasPrefix(id, "objtab.") {
			placed = strings.Contains(string(src), `'objtab.' + `) // one family: objtab.<entry id>
		}
		if !placed {
			t.Errorf("anchor %s is registered but %s does not place data-guide=%q", id, file, id)
		}
	}
	// Every recipe target names a registered anchor.
	recipes := 0
	for _, dir := range []string{filepath.Join(root, "app"), filepath.Join(root, "internal", "test", "guide")} {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "frontend") {
				return filepath.SkipDir
			}
			slash := filepath.ToSlash(p)
			if d.IsDir() || !strings.Contains(slash, "/embed/guide/") || !strings.HasSuffix(slash, ".yaml") || strings.HasSuffix(slash, ".i18n.yaml") {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			r, err := guideapi.ParseRecipe(raw)
			if err != nil {
				t.Errorf("%s: %v", slash, err)
				return nil
			}
			recipes++
			if strings.Contains(slash, "internal/test/guide/") {
				return nil // the fixture sidecar's controls are not the GUI's
			}
			for _, s := range r.Steps {
				if s.Target == nil {
					continue
				}
				if _, ok := anchors[s.Target.Guide]; !ok && !strings.HasPrefix(s.Target.Guide, "objtab.") {
					t.Errorf("%s step %s: target %q is not an anchor of the GUI registry", slash, s.ID, s.Target.Guide)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if recipes < 14 {
		t.Fatalf("found %d recipes — the walk is wrong", recipes)
	}
}

// TestEveryPlacedAnchorIsRegistered is the other half of the seam
// : every anchor the GUI places - `data-guide="<id>"`, or the
// ids an expression gives - is in the registry, so a recipe can name it and
// a renamed control cannot drift away unseen.
func TestEveryPlacedAnchorIsRegistered(t *testing.T) {
	root := repoRoot(t)
	front := filepath.Join(root, "app", "ktw-gui", "frontend", "src")
	anchors := readRegistry(t, root)
	literal := regexp.MustCompile(`data-guide="([^"]+)"`)
	expr := regexp.MustCompile(`data-guide=\{([^}]*)\}`)
	// The ids an expression gives: a bare '<id>', or what a ternary yields
	// (`cond ? '<id>' : undefined`) - not what it compares.
	quoted := regexp.MustCompile(`(?:^\s*|[?:]\s*)'([a-z][a-z0-9.:-]*)'`)
	placed := 0
	err := filepath.WalkDir(front, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".tsx") {
			return err
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(front, p)
		check := func(id string) {
			placed++
			if _, ok := anchors[id]; !ok {
				t.Errorf("%s places data-guide=%q, which the registry (guide/anchors.ts) does not hold", filepath.ToSlash(rel), id)
			}
		}
		for _, m := range literal.FindAllStringSubmatch(string(src), -1) {
			check(m[1])
		}
		for _, m := range expr.FindAllStringSubmatch(string(src), -1) {
			if strings.Contains(m[1], "'objtab.' +") {
				continue // one family: objtab.<entry id>
			}
			for _, q := range quoted.FindAllStringSubmatch(m[1], -1) {
				check(q[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if placed < 40 {
		t.Fatalf("found %d placed anchors - the walk is wrong", placed)
	}
}

// openPlaces and openScreens are what a recipe step's `open` may name
// (addresses, without the universe; app/ktw-gui/frontend/src/address.ts).
var openPlaces = map[string]bool{"object": true, "home": true, "screen": true, "rail": true}
var openScreens = map[string]bool{"inbox": true, "marketplace": true, "debug": true, "guide": true, "settings": true}

// openProblem says why a step's `open` is no address the GUI opens; "" when
// it is one.
func openProblem(open string) string {
	segs := strings.Split(strings.Trim(open, "/"), "/")
	switch {
	case !openPlaces[segs[0]]:
		return "no place " + segs[0] + " (object, home, screen, rail)"
	case segs[0] == "object" && (len(segs) < 2 || segs[1] == ""):
		return "an object address names the object"
	case segs[0] == "screen" && (len(segs) < 2 || !openScreens[segs[1]]):
		return "no screen there (inbox, marketplace, debug, guide, settings)"
	}
	return ""
}

// TestRecipeStepsOpenAddresses: a step's `open` is an
// address the player can open - a known place, a known screen, an object
// named - checked here like a target, so a recipe cannot open what is not.
func TestRecipeStepsOpenAddresses(t *testing.T) {
	for open, ok := range map[string]bool{
		"object/m.F-001/links": true, "home/actions": true, "screen/marketplace/sidecars": true, "rail/inbox": true,
		"garden/x": false, "screen/garden": false, "object": false,
	} {
		if got := openProblem(open) == ""; got != ok {
			t.Errorf("open %q accepted = %v; want %v", open, got, ok)
		}
	}
	root := repoRoot(t)
	err := filepath.WalkDir(filepath.Join(root, "app"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "frontend") {
			return filepath.SkipDir
		}
		slash := filepath.ToSlash(p)
		if d.IsDir() || !strings.Contains(slash, "/embed/guide/") || !strings.HasSuffix(slash, ".yaml") || strings.HasSuffix(slash, ".i18n.yaml") {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		r, err := guideapi.ParseRecipe(raw)
		if err != nil {
			return nil // the anchor test reports it
		}
		for _, s := range r.Steps {
			if s.Open != "" {
				if why := openProblem(s.Open); why != "" {
					t.Errorf("%s step %s: open %q: %s", slash, s.ID, s.Open, why)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestRecipeNextNamesShippedGuides: a recipe's `next` (« Pour
// aller plus loin ») names a guide a binary ships - by its id (the same
// provider) or its URI - or an address the GUI opens, so the last step never
// offers what is not there.
func TestRecipeNextNamesShippedGuides(t *testing.T) {
	root := repoRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "app", "*", "embed", "guide", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	shipped := map[string]bool{} // <provider>:/guide-recipe/<id>
	recipes := map[string]guideapi.Recipe{}
	for _, f := range files {
		if strings.HasSuffix(f, ".i18n.yaml") {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		r, err := guideapi.ParseRecipe(raw)
		if err != nil {
			continue
		}
		provider := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(f))))
		uri := provider + ":/guide-recipe/" + strings.TrimSuffix(filepath.Base(f), ".yaml")
		shipped[uri] = true
		recipes[uri] = r
	}
	withNext := 0
	for uri, r := range recipes {
		provider := uri[:strings.Index(uri, ":")]
		for _, e := range r.Next {
			if withNext++; strings.Contains(e, "/") && !strings.Contains(e, ":/guide-recipe/") {
				if why := openProblem(e); why != "" {
					t.Errorf("%s next %q: %s", uri, e, why)
				}
				continue
			}
			target := e
			if !strings.Contains(e, ":/guide-recipe/") {
				target = provider + ":/guide-recipe/" + e
			}
			if !shipped[target] {
				t.Errorf("%s next %q: no shipped guide %s", uri, e, target)
			}
		}
	}
	if withNext == 0 {
		t.Fatal("no recipe names what to read next - the walk is wrong")
	}
}
