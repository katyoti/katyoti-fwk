// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
)

// A SOLUTION APPLIED INSIDE A SCENARIO CASE TOUCHES ONLY THE CASE'S UNIVERSES
// : a case standing for the archifactory solution never
// archives nor creates a universe named « archifactory » - the person's
// installation may hold one. scn-<case> and scn-<case>-<other> are the case's.
// Falsified: with caseOwnsUniverse answering nil, « archifactory » is archived.
func TestASolutionAppliedInACaseTouchesOnlyTheCasesUniverses(t *testing.T) {
	s, _ := fixtureService(t)
	fake := newFakeOrch()
	s.orchOverride = fake
	fake.universes["archifactory"] = nil
	t.Setenv(cenv.ScenarioSandboxEnv, filepath.Join(t.TempDir(), "archifactory-study"))

	_, err := s.ApplyUniverse(context.Background(), api.ApplyUniverseInput{Spec: api.SolutionSpec{
		Name: "archifactory", Sidecars: []api.SolutionSidecar{{Name: "ktw-target"}}, Replace: true,
	}})
	if err == nil || !strings.Contains(err.Error(), "not the scenario case's") {
		t.Fatalf("inside a case, a solution named like the person's universe was applied: %v", err)
	}
	if len(fake.archived) != 0 {
		t.Fatalf("inside a case, %v was archived", fake.archived)
	}

	for _, name := range []string{"scn-archifactory-study", "scn-archifactory-study-lab"} {
		if _, err := s.ApplyUniverse(context.Background(), api.ApplyUniverseInput{Spec: api.SolutionSpec{
			Name: name, Sidecars: []api.SolutionSidecar{{Name: "ktw-target"}},
		}}); err != nil {
			t.Fatalf("the case's own universe %s was refused: %v", name, err)
		}
	}
}
