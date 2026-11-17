// SPDX-License-Identifier: Apache-2.0

package contentlint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const reviewWorkflow = `name: review
label: Review
description: Drafted, reviewed, approved.
initial: draft
states:
  - {name: draft, description: "Being written"}
  - {name: done, description: "Finished", terminal: true}
transitions:
  - {name: finish, from: [draft], to: done}
`

// A pack on disk: what it misses in a language is listed with the source
// text; the scaffold writes a `~` stub per missing text, keeping what the
// file says; the stubs still read as missing until filled, and once filled
// the entry misses nothing.
func TestPackTranslations_MissingThenScaffoldThenFilled(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "embed", "cwm", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(dir, "review.yaml")
	if err := os.WriteFile(entry, []byte(reviewWorkflow), 0o644); err != nil {
		t.Fatal(err)
	}
	i18n := filepath.Join(dir, "review.i18n.yaml")
	if err := os.WriteFile(i18n, []byte("fr:\n  label: Revue\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := PackTranslations(root, "fr")
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	e := entries[0]
	if e.Kind != "workflow" || e.SourceLang != "en" || len(e.Findings) != 0 {
		t.Fatalf("entry = %+v", e)
	}
	var missing []string
	for _, g := range e.Missing {
		missing = append(missing, g.Path+"="+g.Source)
	}
	got := strings.Join(missing, "|")
	if strings.Contains(got, "label=") || !strings.Contains(got, "description=Drafted, reviewed, approved.") || !strings.Contains(got, "states.draft.") {
		t.Fatalf("missing = %s", got)
	}

	added, err := ScaffoldTranslations(e, "fr")
	if err != nil || added != len(e.Missing) {
		t.Fatalf("scaffold added %d of %d: %v", added, len(e.Missing), err)
	}
	raw, _ := os.ReadFile(i18n)
	if !strings.Contains(string(raw), "label: Revue") || !strings.Contains(string(raw), "# Drafted, reviewed, approved.") {
		t.Fatalf("scaffolded file:\n%s", raw)
	}
	again, _ := PackTranslations(root, "fr")
	if len(again[0].Missing) != len(e.Missing) || len(again[0].Findings) != 0 {
		t.Fatalf("after the scaffold: %+v (stubs must still read as missing, and be valid)", again[0])
	}
	if n, _ := ScaffoldTranslations(again[0], "fr"); n != 0 {
		t.Errorf("a second scaffold added %d stubs", n)
	}

	// Fill every stub: nothing missing.
	filled := strings.ReplaceAll(string(raw), ": ~", ": Texte")
	if err := os.WriteFile(i18n, []byte(filled), 0o644); err != nil {
		t.Fatal(err)
	}
	done, _ := PackTranslations(root, "fr")
	if len(done[0].Missing) != 0 || len(done[0].Findings) != 0 {
		t.Fatalf("filled: %+v\n%s", done[0], filled)
	}
	// English is the source: nothing to translate.
	if en, _ := PackTranslations(root, "en"); len(en[0].Missing) != 0 {
		t.Errorf("en: %+v", en[0].Missing)
	}
}

// A data type on disk with a menu and no French: the lint lists the menu's
// label, prompt and confirm missing, never a text that is only one `${...}`
// template.
func TestPackTranslations_DataTypeMenuTextsMissing(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "embed", "cwm", "data-types")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dsl := `types:
  Row:
    fields:
      id: {type: string}
    extensions:
      - name: default
        menu:
          - {id: hold, label: Hold, prompt: "Why hold it?", confirm: "Hold it?"}
          - {id: next, label: "${item.next_label}"}
`
	if err := os.WriteFile(filepath.Join(dir, "rows.dsl.yaml"), []byte(dsl), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := PackTranslations(root, "fr")
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	var missing []string
	for _, g := range entries[0].Missing {
		missing = append(missing, g.Path+"="+g.Source)
	}
	got := strings.Join(missing, "|")
	want := "types.Row.extensions.default.menu.hold.confirm=Hold it?|types.Row.extensions.default.menu.hold.label=Hold|types.Row.extensions.default.menu.hold.prompt=Why hold it?"
	if got != want {
		t.Fatalf("missing = %s\nwant      %s\n%+v", got, want, entries[0])
	}
}
