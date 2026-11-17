// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGUI_NoLiteralStringInTranslatedFolders runs the frontend's guard
// (frontend/tests/no-literal-labels.test.ts) from the Go test run: every .tsx
// and .ts under frontend/src - the Debug tab aside - shows no hard-coded word
// a person reads (a JSX text, a label-like prop, a notification's or a modal's
// title or message), parsed by TypeScript's own parser so code is never taken
// for text. A word that is data takes an `i18n-ignore` comment on its line
// (modules/gui/i18n.md). Skipped where bun is not installed, and where the
// frontend's dependencies are not (a fresh clone has no
// frontend/node_modules: the guard parses with the typescript package, so a
// CI that runs it installs the frontend dependencies first).
func TestGUI_NoLiteralStringInTranslatedFolders(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun not on PATH: the literal guard is a frontend test")
	}
	if _, err := os.Stat(filepath.Join("frontend", "node_modules", "typescript", "package.json")); err != nil {
		t.Skip("frontend dependencies not installed (no frontend/node_modules/typescript): run bun install in frontend to run the literal guard")
	}
	cmd := exec.Command(bun, "test", "tests/no-literal-labels.test.ts")
	cmd.Dir = filepath.Join("frontend")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("a screen holds a hard-coded word - move it to a locale (useT) or mark it i18n-ignore:\n%s", out)
	}
}
