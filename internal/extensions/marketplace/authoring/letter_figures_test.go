// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const katyotiLog = `# KaTyoti 1.006

## Planification & méthode

### 1. A board

**Valeur :** ★★★ Majeur · **Maturité :** Construit et testé

### 2. Tabs

**Valeur :** ★★ Notable · **Maturité :** Construit et testé

## Interface

### 3. A label

**Valeur :** ★ Amélioration · **Maturité :** Construit et testé

### 4. A crash

**Valeur :** 🛠 Correction · **Maturité :** Corrigé et testé
`

const archiLog = `# ArchiFactory 2026.10

## Dossier d'architecture

### 5. Studies

**Valeur :** ★★ Important · **Maturité :** Construit et testé

### 6. Tooling

**Valeur :** 🛠 Outillage · **Maturité :** Disponible

### 7. Untold
`

const letter = `# Quoi de neuf dans ArchiFactory 2026.10

## En chiffres

- **7** changements depuis la 2026.09
- **5** demandes traitées
- **1** changements majeurs, **2** notables, **1** améliorations, **2** corrections (1 non classés)

| Thème | Changements |
|---|---:|
| Méthode | 2 |
| Interface | 2 |
| Études | 3 |
| **Total** | **7** |

## Ce qui change
`

// « totals and values »: a written letter's « En chiffres » is
// checked against the change logs it covers - changes, requests when known,
// each value and the theme table's sum - and the build fails naming each
// figure that differs.
func TestAWrittenLettersFiguresMatchItsChangeLogs(t *testing.T) {
	dir := t.TempDir()
	k := filepath.Join(dir, "1.006.md")
	a := filepath.Join(dir, "2026.10.md")
	if err := os.WriteFile(k, []byte(katyotiLog), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(a, []byte(archiLog), 0o644); err != nil {
		t.Fatal(err)
	}
	covers := []string{a, k}

	if err := CheckLetterFigures(letter, covers, 5); err != nil {
		t.Fatalf("a letter matching its change logs: %v", err)
	}
	for name, off := range map[string]struct{ from, to string }{
		"changes":       {"**7** changements depuis", "**8** changements depuis"},
		"requests":      {"**5** demandes", "**6** demandes"},
		"majeurs":       {"**1** changements majeurs", "**2** changements majeurs"},
		"notables":      {"**2** notables", "**3** notables"},
		"améliorations": {"**1** améliorations", "**0** améliorations"},
		"corrections":   {"**2** corrections", "**1** corrections"},
		"non classés":   {"(1 non classés)", "(2 non classés)"},
		"theme table":   {"| Études | 3 |", "| Études | 4 |"},
	} {
		bad := strings.Replace(letter, off.from, off.to, 1)
		if bad == letter {
			t.Fatalf("%s: the test letter does not hold %q", name, off.from)
		}
		err := CheckLetterFigures(bad, covers, 5)
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s off by one: %v; want refused naming it", name, err)
		}
	}
	// The requests are checked only when known; a letter without « En
	// chiffres » is not checked.
	if err := CheckLetterFigures(strings.Replace(letter, "**5** demandes", "**9** demandes", 1), covers, -1); err != nil {
		t.Errorf("requests unknown, yet checked: %v", err)
	}
	if err := CheckLetterFigures("# Quoi de neuf\n\nNothing counted here.\n", covers, 5); err != nil {
		t.Errorf("a letter without figures: %v", err)
	}
}

// The build checks it but never refuses: a written letter whose figures
// drift from its change logs is published as written (the difference
// reported); CheckLetterFigures still names each figure that differs.
func TestTheBuildPublishesALetterThatDrifts(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "1.006.md")
	if err := os.WriteFile(log, []byte(katyotiLog), 0o644); err != nil {
		t.Fatal(err)
	}
	docs := filepath.Join(dir, "docs")
	if err := os.MkdirAll(filepath.Join(docs, "fr"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(md string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(docs, "fr", "quoi-de-neuf.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	matching := "# Quoi de neuf\n\n## En chiffres\n\n- **4** changements\n- **1** changements majeurs, **1** notables, **1** améliorations, **1** corrections\n"
	write(matching)
	if _, err := publishDocuments(filepath.Join(dir, "out"), "archifactory", "2026.10", SolutionSpec{Docs: docs, WhatsNew: log}); err != nil {
		t.Fatalf("a letter matching its change log: %v", err)
	}
	drifting := strings.Replace(matching, "**4** changements", "**5** changements", 1)
	write(drifting)
	if _, err := publishDocuments(filepath.Join(dir, "out2"), "archifactory", "2026.10", SolutionSpec{Docs: docs, WhatsNew: log}); err != nil {
		t.Fatalf("a letter misstating its changes: %v; want it published as written", err)
	}
	if err := CheckLetterFigures(drifting, []string{log}, -1); err == nil || !strings.Contains(err.Error(), "changes") {
		t.Fatalf("the drift is not named: %v", err)
	}
}
