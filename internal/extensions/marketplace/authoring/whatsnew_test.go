// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleChangelog = `# Journal des changements — version 1.004

## En un coup d'œil

- **Un premier point.**

---

## Interface

### 3. Un écran plus clair

**Valeur :** ★★ Notable

**Ce que ça vous apporte :** on trouve tout du premier coup.

**Aujourd'hui** :
- un détail qui ne va pas dans « Quoi de neuf ».

## Réserves

### 9. Une réserve

**Ce que ça vous apporte :** rien pour le lecteur.
`

// « Quoi de neuf » is written from the change log: the glance, each domain's
// changes with what they bring — not the details, not the reserves.
func TestWhatsNewFromTheChangelog(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "1.004.md")
	if err := os.WriteFile(log, []byte(sampleChangelog), 0o644); err != nil {
		t.Fatal(err)
	}
	version, md, err := WhatsNewMarkdown(log)
	if err != nil {
		t.Fatal(err)
	}
	if version != "1.004" {
		t.Fatalf("version = %q", version)
	}
	doc := string(md)
	for _, want := range []string{"# Quoi de neuf dans 1.004", "Un premier point", "## Interface", "### Un écran plus clair", "on trouve tout du premier coup"} {
		if !strings.Contains(doc, want) {
			t.Errorf("« Quoi de neuf » lacks %q:\n%s", want, doc)
		}
	}
	for _, not := range []string{"Réserves", "une réserve", "un détail", "---"} {
		if strings.Contains(doc, not) {
			t.Errorf("« Quoi de neuf » should not hold %q:\n%s", not, doc)
		}
	}
}

// A letter written for the users (docs/fr/quoi-de-neuf.md) is the solution's
// « Quoi de neuf »: the build ships it as it is and generates none; without
// one, « Quoi de neuf » is made from the change log.
func TestAWrittenLetterIsTheWhatsNew(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "1.004.md")
	if err := os.WriteFile(log, []byte(sampleChangelog), 0o644); err != nil {
		t.Fatal(err)
	}
	read := func(docsDir string) string {
		out := t.TempDir()
		if _, err := publishDocuments(out, "demo", "v1", SolutionSpec{Docs: docsDir, WhatsNew: log}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(out, SolutionsDir, "demo", "v1", "docs", "fr", whatsNewFile))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(filepath.Join(docs, "fr"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "fr", "lisez-moi.md"), []byte("# Lisez-moi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := read(docs); !strings.Contains(got, "# Quoi de neuf dans 1.004") {
		t.Fatalf("without a letter, « Quoi de neuf » is made from the change log:\n%s", got)
	}
	letter := "# Quoi de neuf dans la version\n\nUne lettre écrite pour les utilisateurs.\n"
	if err := os.WriteFile(filepath.Join(docs, "fr", whatsNewFile), []byte(letter), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := read(docs); got != letter {
		t.Fatalf("the written letter is shipped as it is, got:\n%s", got)
	}
}
