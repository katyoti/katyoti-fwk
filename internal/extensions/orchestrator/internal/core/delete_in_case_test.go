// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// A PROCESS INSIDE A SCENARIO CASE ARCHIVES ONLY THE CASE'S UNIVERSES
// : the person's « archifactory » universe was archived while a
// case standing for the archifactory solution was installed. Whatever registry
// such a process resolves - the case's own, or the person's reached by a
// resolution that left the sandbox - a universe whose folder lies outside the
// case is never moved to .archived nor dropped from the registry; the case's
// own universe still is. Falsified: with the guard removed from DeleteUniverse,
// the person's universe is archived.
func TestAProcessInsideACaseNeverArchivesAUniverseOutsideIt(t *testing.T) {
	person := t.TempDir() // a temporary person's installation root
	registry, home := seedUniverse(t, person, "archifactory")
	caseDir := filepath.Join(t.TempDir(), "archifactory-study")
	t.Setenv(cenv.ScenarioSandboxEnv, caseDir)
	svc := &Service{registry: registry}

	out, err := svc.DeleteUniverse(context.Background(), api.DeleteUniverseInput{Name: "archifactory"})
	if err == nil || out.Archived {
		t.Fatalf("inside a case, the person's universe was archived (%+v, %v); want it refused", out, err)
	}
	if !strings.Contains(err.Error(), "outside the scenario case") {
		t.Errorf("the refusal does not say why: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".katyoti", "config", "Univers.yaml")); err != nil {
		t.Errorf("the person's universe folder moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(person, "universes", ".archived")); !os.IsNotExist(err) {
		t.Errorf("an .archived folder was made beside the person's universes (%v)", err)
	}
	if _, err := registry.Find("archifactory"); err != nil {
		t.Errorf("the person's universe left its registry: %v", err)
	}

	// The case's own universe, inside its folder, is archived as before.
	caseRegistry, caseHome := seedUniverse(t, caseDir, "scn-archifactory-study")
	caseSvc := &Service{registry: caseRegistry}
	if out, err := caseSvc.DeleteUniverse(context.Background(), api.DeleteUniverseInput{Name: "scn-archifactory-study"}); err != nil || !out.Archived {
		t.Fatalf("the case's own universe was not archived: %+v, %v", out, err)
	}
	if _, err := os.Stat(caseHome); !os.IsNotExist(err) {
		t.Errorf("the case's universe folder is still there (%v)", err)
	}
}
