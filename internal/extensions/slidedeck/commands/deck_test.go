// SPDX-License-Identifier: Apache-2.0

package commands_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/commands"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// lint runs `deck lint <deck> [flags]` on a copy of a corpus deck, in an
// application that publishes the built-in layouts, and answers its output and
// its error.
func lint(t *testing.T, corpus string, flags ...string) (string, error) {
	t.Helper()
	ctx, _ := cwmapp.New(t, contrib.ContributeLayouts("ktw-test"))
	src, err := os.ReadFile(filepath.Join("..", "api", "testdata", "corpus", corpus))
	if err != nil {
		t.Fatal(err)
	}
	deck := filepath.Join(t.TempDir(), corpus)
	if err := os.WriteFile(deck, src, 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := commands.NewDeckCommand()
	cmd.SetContext(ctx)
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"lint", deck}, flags...))
	err = cmd.Execute()
	return out.String(), err
}

// `deck lint` on a broken deck reports each error with its line and fails;
// on a clean one it says ok with the number of slides and succeeds.
func TestDeckLintReportsEachErrorWithItsLine(t *testing.T) {
	out, err := lint(t, "lint-errors.md", "--json")
	if err == nil {
		t.Fatalf("a deck with errors passed the lint:\n%s", out)
	}
	var findings []deckapi.Finding
	if jerr := json.Unmarshal([]byte(out[:strings.LastIndex(out, "]")+1]), &findings); jerr != nil {
		t.Fatalf("the --json output is not the findings: %v\n%s", jerr, out)
	}
	errors := 0
	for _, f := range findings {
		if f.Severity == deckapi.SeverityError {
			errors++
			if f.Line <= 0 {
				t.Errorf("an error without its line: %+v", f)
			}
		}
	}
	if errors == 0 {
		t.Fatalf("no error reported on lint-errors.md: %+v", findings)
	}

	out, err = lint(t, "code-fence.md")
	if err != nil {
		t.Fatalf("a clean deck failed the lint: %v\n%s", err, out)
	}
	if !strings.Contains(out, ": ok (") || !strings.Contains(out, "slides)") {
		t.Errorf("a clean deck does not say ok with its slides: %q", out)
	}
}
