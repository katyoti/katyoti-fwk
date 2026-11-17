// SPDX-License-Identifier: Apache-2.0

package display_ui_component

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kind=deck: the markdown is the value (inline or a {markdown} object or a
// file), and the result carries the lint's findings for the agent. With no
// catalog in a unit test the lint runs without layouts, so these findings are
// the grammar's own (frontmatter, settings).
func TestDeckKind(t *testing.T) {
	ok, err := executeDisplayUIComponent(context.Background(), deckInput(nil))
	if err == nil || !strings.Contains(err.Error(), "kind='deck' requires") {
		t.Fatalf("empty deck must be refused, got %v %+v", err, ok)
	}

	good := "---\ntitle: T\n---\n\n# One\n\n---\n\n## Two\n"
	out, err := executeDisplayUIComponent(context.Background(), deckInput(good))
	if err != nil || len(out.Findings) != 0 || !strings.Contains(out.Message, "(2 slides)") {
		t.Fatalf("valid deck: %v %+v", err, out)
	}

	bad := "## no frontmatter\n\n---\n\n<!-- slide: steps=all -->\n## Two\n"
	out, err = executeDisplayUIComponent(context.Background(), deckInput(bad))
	if err != nil {
		t.Fatal(err)
	}
	rules := map[string]int{}
	for _, f := range out.Findings {
		rules[f.Rule] = f.Line
	}
	if rules["frontmatter-missing"] != 1 || rules["slide-value-invalid"] != 5 {
		t.Fatalf("findings with lines expected, got %+v", out.Findings)
	}
	if !strings.Contains(out.Message, "fix each at its line") {
		t.Fatalf("message must ask for a fix: %q", out.Message)
	}

	obj := deckInput(nil)
	obj.Data = map[string]any{"markdown": good}
	if out, err := executeDisplayUIComponent(context.Background(), obj); err != nil || len(out.Findings) != 0 {
		t.Fatalf("{markdown} value: %v %+v", err, out)
	}

	file := filepath.Join(t.TempDir(), "q4.deck.md")
	if err := os.WriteFile(file, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	byFile := deckInput(nil)
	byFile.File = file
	if out, err := executeDisplayUIComponent(context.Background(), byFile); err != nil || !strings.Contains(out.Message, "(2 slides)") {
		t.Fatalf("file deck: %v %+v", err, out)
	}
}

func deckInput(v any) DisplayUIComponentInput {
	in := DisplayUIComponentInput{ArtifactID: "d1"}
	in.Kind = "deck"
	in.Data = v
	return in
}
