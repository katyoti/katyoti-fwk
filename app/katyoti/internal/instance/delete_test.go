// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// deleteWorld registers two installations in a temporary registry: "old",
// whose data root lies outside its folder, and "new"; it answers both.
func deleteWorld(t *testing.T) (old, current Instance, dataRoot string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(RegistryEnv, filepath.Join(dir, "instances.yaml"))
	old = Instance{Name: "old", Folder: filepath.Join(dir, "Programs", "old")}
	current = Instance{Name: "new", Folder: filepath.Join(dir, "Programs", "new")}
	dataRoot = filepath.Join(dir, "elsewhere", "old-data")
	inst := cenv.NewInstallation(old.Folder, "acme-demo 1.004", "")
	inst.Roots.Data = dataRoot
	for _, f := range []string{inst.Path(), filepath.Join(inst.ConfigRoot(), "universes.yaml"), filepath.Join(dataRoot, "universes", "u", "log.txt"), filepath.Join(inst.CacheRoot(), "blob")} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("0123456789"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := cenv.SaveInstallation(inst); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(current.Folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := current.EnsureFile(); err != nil {
		t.Fatal(err)
	}
	if err := (Registry{Instances: []Instance{old, current}}).Save(); err != nil {
		t.Fatal(err)
	}
	return old, current, dataRoot
}

// : an earlier installation is deleted - its folder, its data
// root wherever it lies, its shortcuts, its registry entry - only with the
// token of the plan that lists them; without it nothing is touched.
func TestAnInstallationIsDeletedOnlyOnceAcknowledged(t *testing.T) {
	old, current, dataRoot := deleteWorld(t)
	var removed []string
	was := controlplane.RemoveShortcuts
	controlplane.RemoveShortcuts = func(name string) { removed = append(removed, name) }
	t.Cleanup(func() { controlplane.RemoveShortcuts = was })

	reg, _ := Load()
	plan, err := PlanDelete(reg, "old", current.Folder, true)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range plan.Folders {
		paths = append(paths, f.Path)
		if f.Bytes == 0 || f.LastUsed.IsZero() {
			t.Errorf("%s listed without its size or last use: %+v", f.Path, f.Usage)
		}
	}
	if len(plan.Folders) != 2 || !strings.Contains(strings.Join(paths, "|"), dataRoot) || plan.Shortcuts != "acme-demo 1.004" {
		t.Fatalf("plan = %+v, want the folder and the outside data root, shortcuts « acme-demo 1.004 »", plan)
	}

	// Not acknowledged (no token, another token): nothing changes.
	for _, ack := range []string{"", "old-00000000"} {
		if err := plan.Apply(reg, ack); err == nil || !strings.Contains(err.Error(), plan.Token) {
			t.Fatalf("apply with %q = %v, want it refused naming the token", ack, err)
		}
	}
	if _, err := os.Stat(dataRoot); err != nil || len(removed) != 0 {
		t.Fatalf("an unacknowledged deletion touched something: %v, shortcuts %v", err, removed)
	}

	if err := plan.Apply(reg, plan.Token); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{old.Folder, dataRoot} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s survived the deletion (%v)", p, err)
		}
	}
	if len(removed) != 1 || removed[0] != "acme-demo 1.004" {
		t.Errorf("shortcuts removed = %v", removed)
	}
	after, _ := Load()
	if _, err := after.Find("old"); err == nil {
		t.Error("the deleted installation is still registered")
	}
	if _, err := os.Stat(current.Folder); err != nil {
		t.Errorf("the other installation was touched: %v", err)
	}
}

// The installation running, the last one (unless asked) and one whose
// projects folder lies inside it are refused before any plan.
func TestADeletionKeepsWhatItMustNotTake(t *testing.T) {
	old, current, _ := deleteWorld(t)
	reg, _ := Load()
	if _, err := PlanDelete(reg, "new", current.Folder, true); err == nil || !strings.Contains(err.Error(), "runs from") {
		t.Errorf("deleting the running installation = %v, want it refused", err)
	}
	last := Registry{Instances: []Instance{old}}
	if _, err := PlanDelete(last, "old", "", true); err == nil || !strings.Contains(err.Error(), "last installation") {
		t.Errorf("deleting the last installation = %v, want it kept", err)
	}
	if _, err := PlanDelete(last, "old", "", false); err != nil {
		t.Errorf("deleting the last installation with --last = %v", err)
	}

	inst, err := cenv.LoadInstallation(old.File())
	if err != nil {
		t.Fatal(err)
	}
	inst.Settings = map[string]any{cenv.ProjectsFolderSetting: filepath.Join(old.Folder, "projects")}
	if err := cenv.SaveInstallation(inst); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanDelete(reg, "old", current.Folder, true); err == nil || !strings.Contains(err.Error(), "projects folder") {
		t.Errorf("deleting an installation holding its projects folder = %v, want it refused", err)
	}

	// A data root shared with another installation is never deleted.
	inst.Settings = nil
	if err := cenv.SaveInstallation(inst); err != nil {
		t.Fatal(err)
	}
	shared := cenv.NewInstallation(current.Folder, "new", "")
	shared.Roots.Data = inst.DataRoot()
	if err := cenv.SaveInstallation(shared); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanDelete(reg, "old", "", true); err == nil || !strings.Contains(err.Error(), "shared") {
		t.Errorf("deleting an installation whose data root another uses = %v, want it refused", err)
	}
}

// The list shows each installation's size and last use, over all its roots.
func TestAnInstallationShowsItsSizeAndLastUse(t *testing.T) {
	old, _, _ := deleteWorld(t)
	u := old.Usage()
	if u.Bytes < 30 || u.LastUsed.IsZero() {
		t.Fatalf("usage = %+v, want the bytes of its folder and its outside data root", u)
	}
	if humanBytes(1536) != "1.5 KB" || lastUse(u.LastUsed) == "never" {
		t.Fatalf("shown as %s / %s", humanBytes(1536), lastUse(u.LastUsed))
	}
}
