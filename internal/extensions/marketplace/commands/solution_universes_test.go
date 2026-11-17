// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// setup opens on — and makes the default — the main universe the solution
// install names; an install that names none keeps the universe it went into.
func TestSetupOpensOnTheMainUniverse(t *testing.T) {
	res := mktapi.InstallSolutionOutput{Main: "archi", Universes: []mktapi.InstalledUniverse{{Name: "archi", Main: true}, {Name: "archi-review"}}}
	if got := mainUniverse(res, "fallback"); got != "archi" {
		t.Errorf("main = %q, want archi", got)
	}
	if got := mainUniverse(mktapi.InstallSolutionOutput{}, "archi"); got != "archi" {
		t.Errorf("main = %q, want archi", got)
	}
}

// `solution show` lists a solution of several universes universe by universe,
// the main one first and marked; a solution of one lists as it always did.
func TestSolutionShow_ListsEachUniverse(t *testing.T) {
	v := mktapi.ItemVersion{
		Includes:   []mktapi.PinnedRef{{Name: "notes", Version: "1"}, {Name: "cards", Version: "2"}},
		ConfigSets: []mktapi.PinnedRef{{Name: "cs", Version: "1"}},
		Universes: []mktapi.SolutionUniverse{
			{Name: "review", Sidecars: []mktapi.SolutionSidecar{{Name: "cards"}}},
			{Name: "studio", Main: true, Sidecars: []mktapi.SolutionSidecar{{Name: "notes"}}},
		},
	}
	var b bytes.Buffer
	printUniverses(&b, v)
	want := "  universe    studio  (main: opens after install)\n    sidecar     notes@1\n    config-set  cs@1\n" +
		"  universe    review\n    sidecar     cards@2\n"
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
	b.Reset()
	v.Universes = []mktapi.SolutionUniverse{{Main: true}}
	printUniverses(&b, v)
	if got := b.String(); got != "  sidecar     notes@1\n  sidecar     cards@2\n  config-set  cs@1\n" || strings.Contains(got, "universe") {
		t.Errorf("one universe:\n%s", got)
	}
}
