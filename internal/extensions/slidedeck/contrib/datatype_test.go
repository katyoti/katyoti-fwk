// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// capDeck is the `deck` field-group's data-type, rendered from
// deckapi.DeckObjectFields — the one source of the contract.
var capDeck = datatypegen.Source{
	Name:    "cap-deck",
	Version: "1.0.0",
	Root:    "CapDeck",
	Origin:  "deckapi.DeckObjectFields (internal/extensions/slidedeck/api/deck.go)",
	Description: "Field-group for the `deck` capability: a slide-deck object's settings —\n" +
		"aspect, theme, default transition, author, watermark and production mark.\n" +
		"The title is the object's name and the slides are the body.\n",
	AST: func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[deckapi.DeckObjectFields]() },
}

// TestCapDeckIsCurrent refuses a data-type that no longer says what
// DeckObjectFields says. KTW_DATATYPE_UPDATE=1 rewrites it.
func TestCapDeckIsCurrent(t *testing.T) {
	file := filepath.Join("embed", "cwm", "data-types", "cap-deck.dsl.yaml")
	want, err := datatypegen.RenderAs(capDeck, datatypegen.Header{
		Tool:       "the slidedeck contrib test",
		Regenerate: "`KTW_DATATYPE_UPDATE=1 go test ./internal/extensions/slidedeck/contrib/`",
		URI:        "ktw-docs:/data-type/cap-deck",
	})
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("KTW_DATATYPE_UPDATE") == "1" {
		if err := os.WriteFile(file, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("%v — run with KTW_DATATYPE_UPDATE=1", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
		t.Errorf("%s is stale: %s changed — run with KTW_DATATYPE_UPDATE=1", file, capDeck.Origin)
	}
}
