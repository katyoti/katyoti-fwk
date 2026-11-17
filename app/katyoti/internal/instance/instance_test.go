// SPDX-License-Identifier: Apache-2.0

package instance

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// A process inside an instance reads the instance's file, and nothing of the
// caller's installation: no root, no KTW_DATA (it would pin every universe to
// one data folder), no universe, no workspace, no channel, no other file.
func TestEnvEntersTheInstance(t *testing.T) {
	inst := Instance{Name: "next", Folder: filepath.Join("C:", "inst", "next")}
	env := inst.Env([]string{
		"PATH=/bin", "KTW_KATYOTI_ROOT=/host/root", "KTW_DATA=/host/data", "KTW_CACHE=/host/cache",
		"XDG_DATA_HOME=/host/share", "KTW_WORKSPACE=/w", "KTW_UNIVERSE=u", "KTW_INFRA_CHANNEL=/c", "ktw_home=/h",
		"KTW_INSTALLATION=/host/katyoti.yml",
	})
	want := []string{"PATH=/bin", "KTW_INSTALLATION=" + inst.File()}
	slices.Sort(env)
	slices.Sort(want)
	if !slices.Equal(env, want) {
		t.Fatalf("env = %v\nwant %v", env, want)
	}
}

// An instance's roots are its file's; one made before it had a file gets one.
func TestAnInstanceHasItsFile(t *testing.T) {
	inst := Instance{Name: "next", Folder: t.TempDir()}
	if err := inst.EnsureFile(); err != nil {
		t.Fatal(err)
	}
	if inst.Root() != filepath.Join(inst.Folder, "root") || inst.Data() != filepath.Join(inst.Folder, "data") {
		t.Fatalf("roots %s %s", inst.Root(), inst.Data())
	}
}

// The registry reads back what it saved, and a name is found case-blind.
func TestRegistryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(RegistryEnv, filepath.Join(dir, "instances.yaml"))
	if r, err := Load(); err != nil || len(r.Instances) != 0 {
		t.Fatalf("empty registry = %+v, %v", r, err)
	}
	// The folder exists: one that is gone is forgotten.
	demo := filepath.Join(dir, "demo")
	if err := os.MkdirAll(demo, 0o755); err != nil {
		t.Fatal(err)
	}
	r := Registry{Instances: []Instance{{Name: "demo", Folder: demo}}}
	if err := r.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	i, err := got.Find("DEMO")
	if err != nil || i.Folder != demo {
		t.Fatalf("find = %+v, %v", i, err)
	}
	if _, err := got.Find("other"); err == nil || !strings.Contains(err.Error(), "demo") {
		t.Fatalf("an unknown name should list the known ones: %v", err)
	}
}

func TestValidName(t *testing.T) {
	for n, ok := range map[string]bool{"next": true, "v1_4-test": true, "": false, "a b": false, `a\b`: false, "../x": false} {
		if validName(n) != ok {
			t.Errorf("validName(%q) = %v", n, !ok)
		}
	}
}

// An installation is listed under the name its katyoti.yml gives it.
func TestAnInstanceShowsItsInstallationName(t *testing.T) {
	inst := Instance{Name: "acme-demo-v1-004", Folder: t.TempDir()}
	if err := cenv.SaveInstallation(cenv.NewInstallation(inst.Folder, "acme-demo v1.004", "acme-demo@v1.004")); err != nil {
		t.Fatal(err)
	}
	if got := inst.Title(); got != "acme-demo v1.004" {
		t.Fatalf("title = %q, want the katyoti.yml name", got)
	}
}
