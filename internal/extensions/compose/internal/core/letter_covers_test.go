// SPDX-License-Identifier: Apache-2.0

package core

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
)

// The marketplace build names every change log a solution's written « Quoi de
// neuf » covers - the build's --whats-new, its --covers and the solution's
// letter_covers - so the build checks its figures against all of them
// ; a letter covering --whats-new alone keeps
// the single-log check.
func TestTheBuildNamesEveryChangeLogALetterCovers(t *testing.T) {
	defDir := t.TempDir()
	whatsNew := filepath.Join(defDir, "changelog", "2026.10.md")
	katyoti := filepath.Join(defDir, "changelog", "1.006.md")
	sol := authoring.DefinitionSolution{Case: "archifactory", LetterCovers: []string{"changelog/1.006.md"}}

	got := letterCovers(defDir, sol, whatsNew, nil)
	if !slices.Equal(got, []string{whatsNew, katyoti}) {
		t.Errorf("letter_covers of the solution: %v; want %s then %s", got, whatsNew, katyoti)
	}
	if got := letterCovers(defDir, authoring.DefinitionSolution{Case: "x"}, whatsNew, []string{katyoti, whatsNew}); !slices.Equal(got, []string{whatsNew, katyoti}) {
		t.Errorf("--covers: %v; want %s then %s, each once", got, whatsNew, katyoti)
	}
	if got := letterCovers(defDir, authoring.DefinitionSolution{Case: "x"}, whatsNew, nil); got != nil {
		t.Errorf("a letter covering --whats-new alone: %v; want none (the single-log check)", got)
	}
}
