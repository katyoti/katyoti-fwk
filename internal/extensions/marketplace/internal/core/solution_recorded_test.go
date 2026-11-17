// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// Each universe a solution installs records the solution in its Univers.yaml
// (`solution: <name>@<version>`), and its summary carries it: what the
// universe needs is found again from it (the Ready to start screen). A member
// needs a program this machine lacks, and the install runs on an engine that
// cannot install one: none is installed, it is only planned.
func TestEachUniverseRecordsTheSolutionThatInstalledIt(t *testing.T) {
	s := solutionFixture(t, `      - version: "1"
        includes:
          - {name: notes, version: "1.0"}
          - {name: cards, version: "1.0"}
        universes:
          - name: review
            sidecars: [{name: cards}]
          - name: studio
            main: true
            sidecars: [{name: notes}]
`)
	notes, err := store.ContentFolderPath("notes", "1.0")
	if err != nil {
		t.Fatal(err)
	}
	decl := "name: notes\nconfig:\n  tools:\n    - name: prog\n      commands: [ktw-test-no-such-program]\n" +
		"      recipes:\n        - platform: " + runtime.GOOS + "\n          kind: script\n          script: install/prog.sh\n"
	if err := os.WriteFile(filepath.Join(notes, "ktw-host.yaml"), []byte(decl), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := s.InstallSolution(context.Background(), mktapi.InstallSolutionInput{Ref: mktapi.ItemRef{Name: "duo"}, UniverseHome: "duo", SolutionName: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Prereqs == nil || len(out.Prereqs.Programs) != 1 || out.Prereqs.Programs[0].Name != "prog" {
		t.Fatalf("prereqs %+v: want prog planned", out.Prereqs)
	}
	if out.Main != "duo" {
		t.Errorf("main %q, want duo: the universe named after the solution", out.Main)
	}
	reg := registered(t)
	if len(reg) != 2 {
		t.Fatalf("registry %v: want duo and duo-review", reg)
	}
	for name, home := range reg {
		m, err := orchcatalog.LoadManifest(home)
		if err != nil {
			t.Fatal(err)
		}
		if m.Solution != "duo@1" {
			t.Errorf("%s records solution %q, want duo@1", name, m.Solution)
		}
		if sum := m.ToUniverseSummary(name, home, false); sum.Solution != "duo@1" {
			t.Errorf("%s summary solution %q, want duo@1", name, sum.Solution)
		}
	}
}
