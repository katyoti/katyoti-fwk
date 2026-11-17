// SPDX-License-Identifier: Apache-2.0

package deck_export

import (
	"path/filepath"
	"strings"
	"testing"

	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	deckdisplay "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/display"
)

// The export goes where asked, else beside the deck's file, else under
// decks/ by the deck's title — always with the format's extension.
func TestOutputPath(t *testing.T) {
	deck := deckapi.Parse("---\ntitle: \"Les présentations: Acme\"\n---\n\n## A\n")
	src := filepath.Join("ws", "decks", "talk.deck.md")
	if got := outputPath("", src, deck, deckdisplay.FormatPptx); got != filepath.Join("ws", "decks", "talk.pptx") {
		t.Errorf("beside the file: %s", got)
	}
	if got := outputPath("", src, deck, deckdisplay.FormatHTML); got != filepath.Join("ws", "decks", "talk.html") {
		t.Errorf("html beside the file: %s", got)
	}
	got := outputPath("", "", deck, deckdisplay.FormatPptxPictures)
	if !strings.HasSuffix(got, filepath.Join("decks", "Les présentations- Acme.pptx")) {
		t.Errorf("by the title: %s", got)
	}
	abs := filepath.Join(t.TempDir(), "out")
	if got := outputPath(abs, src, deck, deckdisplay.FormatHTML); got != abs+".html" {
		t.Errorf("as asked, with the extension: %s", got)
	}
}

// The tool refuses an unknown format and a deck with no slide before any call.
func TestExecuteRefuses(t *testing.T) {
	if _, err := executeDeckExport(t.Context(), DeckExportInput{Markdown: "## A", Format: "pdf"}); err == nil || !strings.Contains(err.Error(), "format") {
		t.Errorf("unknown format: %v", err)
	}
	if _, err := executeDeckExport(t.Context(), DeckExportInput{Markdown: "---\ntitle: T\n---\n", Format: "html"}); err == nil || !strings.Contains(err.Error(), "no slide") {
		t.Errorf("empty deck: %v", err)
	}
	if _, err := executeDeckExport(t.Context(), DeckExportInput{Format: "html"}); err == nil || !strings.Contains(err.Error(), "file or its markdown") {
		t.Errorf("no deck: %v", err)
	}
}
