// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// THE LEAK CHECK FAILS THE VERB ON A PERSON'S UNIVERSE ARCHIVED OR A DATA
// FOLDER MADE: on a temporary person's installation, a verb
// during which their « archifactory » universe is moved to .archived and
// dropped from universes.yaml fails, naming the registry and the universe's
// config; one that makes or removes a universe data folder in the person's
// data root (what tests left there: plan-test, sol, …) fails too. A log
// written in an existing data folder is the person's hub's, not a leak.
// Falsified: without the data folders in the snapshot, the made folder passes.
func TestTheLeakCheckFailsOnAPersonsUniverseArchivedOrADataFolderMade(t *testing.T) {
	base := t.TempDir()
	host := application.MachineDirs{Root: filepath.Join(base, "root"), Data: filepath.Join(base, "data"), Cache: filepath.Join(base, "cache")}
	write := func(rel, s string) {
		p := filepath.Join(base, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("root/universes.yaml", "universes:\n  - name: archifactory\n")
	write("root/universes/archifactory/.katyoti/config/workspace.yaml", "x: 1")
	write("data/universes/archifactory/logs/hub.log", "a")

	// The person's universe archived, as on 2026-10-24.
	g := WatchLeaks(host)
	_ = os.MkdirAll(filepath.Join(base, "root", "universes", ".archived"), 0o755)
	if err := os.Rename(filepath.Join(base, "root", "universes", "archifactory"), filepath.Join(base, "root", "universes", ".archived", "archifactory")); err != nil {
		t.Fatal(err)
	}
	write("root/universes.yaml", "universes: []\n")
	err := g.Check()
	if err == nil || !strings.Contains(err.Error(), "universes.yaml") || !strings.Contains(err.Error(), filepath.Join("archifactory", ".katyoti", "config", "workspace.yaml")) {
		t.Fatalf("the person's universe archived: %v; want a leak naming the registry and the universe's config", err)
	}

	// A data folder made, or removed, in the person's data root.
	g = WatchLeaks(host)
	_ = os.MkdirAll(filepath.Join(base, "data", "universes", "plan-test"), 0o755)
	if err := g.Check(); err == nil || !strings.Contains(err.Error(), filepath.Join("data", "universes", "plan-test")) {
		t.Errorf("a data folder made: %v; want a leak naming it", err)
	}
	g = WatchLeaks(host)
	_ = os.RemoveAll(filepath.Join(base, "data", "universes", "plan-test"))
	if err := g.Check(); err == nil {
		t.Errorf("a data folder removed passed the check")
	}

	// The person's hub writing its log in its own folder is not a leak.
	g = WatchLeaks(host)
	write("data/universes/archifactory/logs/hub.log", "a longer log line")
	if err := g.Check(); err != nil {
		t.Errorf("a log in an existing data folder: %v; want no leak", err)
	}
}
