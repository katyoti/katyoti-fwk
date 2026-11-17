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

// The one object card (rule): an object is drawn
// as a card by the display library's `object-card` node, filled from its CWM
// row through render.ObjectCard / render.ObjectCards. A Go page producer
// passes rows; it never assembles a card node of its own.
var (
	cardNodeLiteral       = regexp.MustCompile(`"kind":\s*"card"`)
	objectCardNodeLiteral = regexp.MustCompile(`"kind":\s*"object-card"`)
)

// cardBuildersToMove are the producers that assembled cards before the object
// card existed: each moves onto it as its screen is rebuilt (IN-1157's S0-S6),
// and leaves this list then. The list only shrinks: a new card builder is
// refused, one gone from it must leave it. The private extensions' builders
// are listed in private_test.go.
var cardBuildersToMove = map[string]bool{
	"internal/extensions/inbox/internal/waiting/chat.go":                   true,
	"internal/extensions/planification/api/increment_card.go":              true,
	"internal/extensions/planification/internal/core/deck.go":              true,
	"internal/extensions/planification/internal/core/person_queue_page.go": true,
	"internal/kts/protocol/terminal/chatpanel/chatpanel.go":                true,
}

// The render package owns the card helpers (render.Card, render.ObjectCard).
const cardHelperDir = "internal/core/render/"

func goProducers(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	var out []string
	for _, dir := range []string{"internal", "app"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "testdata", "frontend", "wailsjs":
					return filepath.SkipDir
				}
				return nil
			}
			rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
			if strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") {
				out = append(out, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	sort.Strings(out)
	return out
}

func TestNoCardNodeBuiltOutsideTheObjectCard(t *testing.T) {
	root := repoRoot(t)
	files := goProducers(t)
	if len(files) < 500 {
		t.Fatalf("only %d Go files found: the roots are wrong", len(files))
	}
	builders := map[string]bool{}
	for _, rel := range files {
		if strings.HasPrefix(rel, cardHelperDir) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		code := stripComments(string(raw), ".go")
		if objectCardNodeLiteral.MatchString(code) {
			t.Errorf("%s builds an object-card node itself: pass rows to render.ObjectCard / render.ObjectCards", rel)
		}
		if cardNodeLiteral.MatchString(code) {
			builders[rel] = true
			if !cardBuildersToMove[rel] {
				t.Errorf("%s builds a card node of its own: draw the object with render.ObjectCards from its rows", rel)
			}
		}
	}
	for rel := range cardBuildersToMove {
		if !builders[rel] {
			t.Errorf("%s builds no card node any more: take it off cardBuildersToMove", rel)
		}
	}
}
