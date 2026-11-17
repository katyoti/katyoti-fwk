// SPDX-License-Identifier: Apache-2.0

//go:build katyoti_e2e

package e2etest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A SEED BRINGS ITS PROJECT'S TYPE: a project published
// before CWM ever opened it - no project object, only the seed of one in its
// store, as a scenario case declaring its project's type leaves it - is
// packed with that seed, and `seed apply` (the path a solution's install
// takes) leaves it in the new project's store, so CWM mints the project
// object of that type at the first opening, never the default one.
func TestSeedApplyCarriesTheProjectObjectsSeed(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	_ = os.MkdirAll(dist, 0o755)
	catalog := filepath.Join(dist, "katyoti-marketplace.yaml")
	project := filepath.Join(t.TempDir(), "source-project")
	writeFile(t, filepath.Join(project, "modules", "archi", "objects", "archi.cwm.yaml"), "id: archi\ntype: documentation\nname: Archi\n")
	writeFile(t, filepath.Join(project, ".katyoti", "project-seed.yaml"), "title: ArchiFactory\ntype: dossier-architecture\n")
	pub := newInst(t)
	pub.run(t, false, composeBin, "build-seed", project, "--name", "typed-seed", "--version", "1.0.0", "--output", dist)

	tgt := newInst(t)
	tgt.run(t, false, katyotiBin, "marketplace", "add", "demo", catalog)
	writeFile(t, filepath.Join(tgt.workspace, ".katyoti", "config", "Univers.yaml"), "name: demo\n")
	dir := filepath.Join(t.TempDir(), "typed-project")
	out := tgt.run(t, false, katyotiBin, "seed", "apply", "typed-seed", "--universe", tgt.workspace, "--project", dir)
	if !strings.Contains(out, "laid out and attached") {
		t.Fatalf("seed apply should report the project: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "modules", "archi", "objects", "archi.cwm.yaml")); err != nil {
		t.Errorf("the seed's module is not in the project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "project.cwm.yaml")); err == nil {
		t.Error("a project object was laid out: CWM mints it, from the seed")
	}
	if _, err := os.Stat(filepath.Join(dir, "project-seed.yaml")); err == nil {
		t.Error("the project object's seed landed in the project's folder, not its store")
	}
	got := readFileString(t, filepath.Join(dir, ".katyoti", "project-seed.yaml"))
	if !strings.Contains(got, "type: dossier-architecture") || !strings.Contains(got, "title: ArchiFactory") {
		t.Errorf("the new project's store holds no seed of a dossier-architecture project object:\n%s", got)
	}
}
