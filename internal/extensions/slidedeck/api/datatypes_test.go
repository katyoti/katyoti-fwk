// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

const regen = "run `KTW_REGEN_DATATYPES=1 go test ./internal/extensions/slidedeck/api/ -run TestDeck`"

// deckDataTypes are the declared grammar, rendered from the Go types into the
// ktw-docs pack (app/ktw-docs/embed/data-types). The deck kinds' own shapes
// (deck-theme, deck-asset, slide-group) are their leaf's (deckkind),
// published with the kinds; deckTheme is rendered for TypeScript only.
var deckDataTypes = []datatypegen.Source{
	{
		Name: "deck", Version: "1.0.0", Root: "DeckSettings",
		Origin:      "deckapi.DeckSettings (internal/extensions/slidedeck/api/deck.go)",
		Description: "A slide deck's frontmatter: the settings of the whole deck. The deck\nitself is markdown; see modules/slidedeck/.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[deckapi.DeckSettings]() },
	},
	{
		Name: "deck-slide", Version: "1.0.0", Root: "SlideSettings",
		Origin:      "deckapi.SlideSettings (internal/extensions/slidedeck/api/deck.go)",
		Description: "What one slide may declare on its first line as\n`<!-- slide: key=value … -->`.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[deckapi.SlideSettings]() },
	},
	{
		Name: "deck-layout", Version: "1.0.0", Root: "Layout",
		Origin:      "deckapi.LayoutBody (internal/extensions/slidedeck/api/deck.go)",
		Description: "A slide layout: when to use it and its named slots. Chosen by a slide as\n`layout=<name>`; styled by the deck component.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[deckapi.LayoutBody]() },
	},
	{
		Name: "deck-review", Version: "1.0.0", Root: "DeckReview",
		Origin:      "deckapi.DeckReview (internal/extensions/slidedeck/api/review.go)",
		Description: "A deck object's review: comment threads on its slides and the reviews marked\ndone. The object's `review` named file, beside it in decks/.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[deckapi.DeckReview]() },
	},
}

// deckTheme is the deck-theme shape the component styles a deck from.
var deckTheme = datatypegen.Source{
	Name: "deck-theme", Version: "1.0.0", Root: "Theme",
	Origin:      "deckapi.ThemeBody (internal/extensions/slidedeck/api/theme.go)",
	Description: "A deck theme: design tokens and fonts and the chrome every slide carries and\nthe looks of each layout and named icons. Chosen by a deck as `theme: <name>`.\n",
	AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[deckapi.ThemeBody]() },
}

// deckTSTypes are what the TypeScript parser and component import
// (app/ktw-display-lib/src/ui-builders/deck/generated) — never a hand copy.
var deckTSTypes = []datatypegen.Source{
	deckDataTypes[0], deckDataTypes[1], deckDataTypes[2], deckTheme, deckDataTypes[3],
	{
		Name: "deck-model", Root: "Deck",
		Origin:      "deckapi.Deck (internal/extensions/slidedeck/api/deck.go)",
		Description: "A parsed deck: the model the component renders and the lint checks.",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[deckapi.Deck]() },
	},
	{
		Name: "deck-finding", Root: "Finding",
		Origin:      "deckapi.Finding (internal/extensions/slidedeck/api/lint.go)",
		Description: "One thing wrong with a deck at a source line.",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[deckapi.Finding]() },
	},
}

var repo = filepath.Join("..", "..", "..", "..")

// TestDeckDataTypesAreCurrent refuses a stale pack data-type.
func TestDeckDataTypesAreCurrent(t *testing.T) {
	dir := filepath.Join(repo, "app", "ktw-docs", "embed", "data-types")
	for _, src := range deckDataTypes {
		want, err := datatypegen.RenderAs(src, datatypegen.Header{
			Tool: "TestDeckDataTypesAreCurrent", Regenerate: regen, URI: "ktw-docs:/data-type/" + src.Name,
		})
		if err != nil {
			t.Fatalf("%s: %v", src.Name, err)
		}
		current(t, filepath.Join(dir, src.Name+".dsl.yaml"), want)
	}
}

// TestDeckTSTypesAreCurrent refuses a stale TypeScript interface.
func TestDeckTSTypesAreCurrent(t *testing.T) {
	dir := filepath.Join(repo, "app", "ktw-display-lib", "src", "ui-builders", "deck", "generated")
	for _, src := range deckTSTypes {
		want, err := datatypegen.RenderTSAs(src, datatypegen.Header{Tool: "TestDeckTSTypesAreCurrent", Regenerate: regen})
		if err != nil {
			t.Fatalf("%s: %v", src.Name, err)
		}
		current(t, filepath.Join(dir, src.Name+".ts"), want)
	}
}

func current(t *testing.T, path string, want []byte) {
	t.Helper()
	if os.Getenv("KTW_REGEN_DATATYPES") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (regenerate with KTW_REGEN_DATATYPES=1)", err)
	}
	if strings.ReplaceAll(string(got), "\r\n", "\n") != string(want) {
		t.Errorf("%s is stale — the Go type is the source; regenerate with KTW_REGEN_DATATYPES=1", path)
	}
}
