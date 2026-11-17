// SPDX-License-Identifier: Apache-2.0

//go:build katyoti_e2e

package e2etest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A SEED'S PROJECT IS CREATED ON THIS PC: a
// project the marketplace lays out from a seed - `seed apply`, a solution's
// install - is recorded in the installation's creation record
// (created-projects.yaml beside the universes registry) through the
// orchestrator, so it may later join a universe beside other members; a
// project folder that already held files, kept as it was by a solution's
// install, is never recorded: it may be a clone.
func TestASeedsProjectIsRecordedAsCreatedOnThisPC(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	_ = os.MkdirAll(dist, 0o755)
	archive, sha := contentPack(t, dist, "flowpack")
	contentCat := filepath.Join(dist, "katyoti-marketplace.yaml")
	writeFile(t, contentCat, fmt.Sprintf(`apiVersion: 1
marketplace: {name: content}
items:
  - name: flowpack
    kind: content
    versions: [{version: "1.0.0", assets: [{url: "%s", sha256: "%s"}]}]
`, fileURL(archive), sha))

	// A universe snapshotted with its project: a seed pinned by a release.
	src := newInst(t)
	writeFile(t, filepath.Join(src.workspace, ".katyoti", "config", "Univers.yaml"), "name: demo\ndisplay_name: Demo\n")
	src.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	src.run(t, false, katyotiBin, "install", "flowpack")
	src.run(t, false, katyotiBin, "universe", "wire", "flowpack")
	project := filepath.Join(t.TempDir(), "demo-src-project")
	writeFile(t, filepath.Join(project, "README.md"), "# The demo\n")
	writeFile(t, filepath.Join(project, "modules", "demo", "objects", "demo.cwm.yaml"), "id: demo\ntype: module\nname: Demo\n")
	releases := filepath.Join(dist, "releases.yaml")
	src.run(t, false, composeBin, "snapshot-universe", src.workspace, "--version", "v1", "--marketplace", releases,
		"--name", "demo-rel", "--project", project)

	// seed apply into a fresh folder: recorded.
	tgt := newInst(t)
	tgt.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	tgt.run(t, false, katyotiBin, "marketplace", "add", "releases", releases)
	writeFile(t, filepath.Join(tgt.workspace, ".katyoti", "config", "Univers.yaml"), "name: demo\n")
	tgt.run(t, false, katyotiBin, "install", "flowpack")
	tgt.run(t, false, katyotiBin, "universe", "wire", "flowpack")
	applied := filepath.Join(t.TempDir(), "applied-project")
	out := tgt.run(t, false, katyotiBin, "seed", "apply", "demo-rel-seed", "--universe", tgt.workspace, "--project", applied)
	if !strings.Contains(out, "laid out and attached") || strings.Contains(out, "not recorded") {
		t.Fatalf("seed apply should lay the project out and record it: %s", out)
	}
	if !createdRecordLists(t, tgt, applied) {
		t.Errorf("the project seed apply laid out is not recorded as created on this PC:\n%s", createdRecord(t, tgt))
	}

	// A solution's install into a fresh projects folder: recorded.
	fresh := newInst(t).withHome(t.TempDir())
	fresh.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	fresh.run(t, false, katyotiBin, "marketplace", "add", "releases", releases)
	fresh.run(t, false, katyotiBin, "solution", "install", "demo-rel@v1", "--universe", filepath.Join(t.TempDir(), "demo"), "--non-interactive")
	laid := filepath.Join(fresh.home, "Documents", "Katyoti", "demo")
	if !createdRecordLists(t, fresh, laid) {
		t.Errorf("the project the solution's install laid out is not recorded:\n%s", createdRecord(t, fresh))
	}

	// A solution's install over a folder holding files: kept, never recorded.
	kept := newInst(t).withHome(t.TempDir())
	mine := filepath.Join(kept.home, "Documents", "Katyoti", "demo")
	writeFile(t, filepath.Join(mine, "README.md"), "# Mine, maybe a clone\n")
	kept.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	kept.run(t, false, katyotiBin, "marketplace", "add", "releases", releases)
	out = kept.run(t, false, katyotiBin, "solution", "install", "demo-rel@v1", "--universe", filepath.Join(t.TempDir(), "demo"), "--non-interactive")
	if !strings.Contains(out, "kept as it was") {
		t.Fatalf("a project folder holding files should be kept: %s", out)
	}
	if createdRecordLists(t, kept, mine) {
		t.Errorf("a kept project folder was recorded as created on this PC:\n%s", createdRecord(t, kept))
	}
}

// createdRecord is the instance's creation record, beside its universes
// registry; "" when there is none.
func createdRecord(t *testing.T, in inst) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(in.root, "created-projects.yaml"))
	if err != nil {
		return ""
	}
	return string(data)
}

// createdRecordLists reports whether the instance's creation record lists dir.
func createdRecordLists(t *testing.T, in inst, dir string) bool {
	t.Helper()
	rec := strings.ToLower(filepath.ToSlash(createdRecord(t, in)))
	return strings.Contains(strings.ReplaceAll(rec, `\`, "/"), strings.ToLower(filepath.ToSlash(dir)))
}
