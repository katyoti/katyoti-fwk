// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// A STALE INSTALL IS NEVER REUSED: an install records what it
// was made from; one with no stamp, one made from another definition of the
// case, or one whose binaries came from another build of its source folder is
// refused by a start, a run and a replay - each saying what to do. A change of
// its stages needs no new install, and refreshing its binaries makes it fresh.
func TestAStaleCaseInstallIsRefused(t *testing.T) {
	src := t.TempDir()
	c := &api.Case{Name: "duo", Universe: api.CaseUniverse{Sidecars: []string{"ktw-a"}},
		Stages: []api.Stage{{ID: "s1"}}}
	e := NewEngine(c, t.TempDir(), src, nil)
	for _, b := range e.caseBinaries() {
		write(t, filepath.Join(src, exe(b)), "build 1 of "+b)
	}
	if err := os.MkdirAll(e.RT.Bin(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := saveState(e.RT, State{Installed: true}); err != nil {
		t.Fatal(err)
	}
	refused := func(what, want string) {
		t.Helper()
		err := e.CheckFresh()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: %v; want refused naming %q", what, err, want)
		}
		if _, err := e.Run(context.Background(), RunOptions{}); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: run answers %v; want refused naming %q", what, err, want)
		}
		if err := e.Start(context.Background()); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: start answers %v; want refused naming %q", what, err, want)
		}
	}
	fresh := func(what string) {
		t.Helper()
		if err := e.CheckFresh(); err != nil {
			t.Fatalf("%s: %v; want the install fresh", what, err)
		}
	}

	if err := e.copyBinaries(); err != nil {
		t.Fatal(err)
	}
	refused("an install with no stamp", "no install stamp")

	if err := e.stampInstall(); err != nil {
		t.Fatal(err)
	}
	fresh("as installed")
	c.Stages = append(c.Stages, api.Stage{ID: "s2"})
	fresh("a stage added")

	c.Universe.Sidecars = []string{"ktw-a", "ktw-b"}
	refused("another definition", "install it again")
	c.Universe.Sidecars = []string{"ktw-a"}
	fresh("the definition back")

	later := time.Now().Add(time.Minute)
	write(t, filepath.Join(src, exe("ktw-a")), "build 2 of ktw-a, longer")
	if err := os.Chtimes(filepath.Join(src, exe("ktw-a")), later, later); err != nil {
		t.Fatal(err)
	}
	refused("another build of a member", "ktw-a changed")

	if err := e.copyBinaries(); err != nil {
		t.Fatal(err)
	}
	if err := e.stampBinaries(); err != nil {
		t.Fatal(err)
	}
	fresh("its binaries refreshed")

	// An installed universe a clean machine's case runs on is not checked.
	e.Universe = "katyoti"
	c.Universe.Sidecars = nil
	fresh("an installed universe")
}
