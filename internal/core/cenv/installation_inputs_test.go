// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// : a process started from an installation uses only what that
// installation holds - its roots, registry, control plane, keys - and no
// inherited variable, current-folder .env or other installation.

// twoInstallations makes installations A and B side by side, each with a
// sidecar folder deep in its store, and a project attached to B's universe.
func twoInstallations(t *testing.T) (a, b *Installation, aExeDir, bProject, bHome string) {
	t.Helper()
	machine := t.TempDir()
	mk := func(name string) (*Installation, string) {
		dir := filepath.Join(machine, name)
		inst := NewInstallation(dir, name, "")
		exeDir := filepath.Join(dir, "root", "bin", "store", "sidecar", "1.0.0", "windows-amd64")
		if err := os.MkdirAll(exeDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := SaveInstallation(inst); err != nil {
			t.Fatal(err)
		}
		return inst, exeDir
	}
	a, aExeDir = mk("a")
	b, _ = mk("b")
	bHome = filepath.Join(b.ConfigRoot(), "universes", "work", ".katyoti")
	bProject = filepath.Join(machine, "projects", "b-project")
	for _, d := range []string{bHome, bProject} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bHome, AttachedProjectsFile), []byte("projects:\n  - "+filepath.ToSlash(bProject)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return a, b, aExeDir, bProject, bHome
}

// isolateEnv lets the test set every variable an installation reads or
// refuses, restored after it, and keeps the refused list to itself.
func isolateEnv(t *testing.T) {
	t.Helper()
	for _, k := range append(slices.Clone(installationInputs), append(installationDropped,
		InstallationEnv, "KTW_KATYOTI_ROOT", "XDG_DATA_HOME", "KTW_CACHE", "KTW_INSTANCE")...) {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	was := refusedInputs
	refusedInputs = nil
	t.Cleanup(func() { refusedInputs = was })
}

// bVariables are what a process of B hands its children.
func bVariables(t *testing.T, b *Installation) {
	t.Helper()
	for k, v := range map[string]string{
		InstallationEnv:    b.Path(),
		"KTW_KATYOTI_ROOT": b.ConfigRoot(),
		"KTW_WORKSPACE":    filepath.Join(b.ConfigRoot(), "universes", "work"),
		"KTW_UNIVERSE":     "work",
		"KTW_NATS_URL":     "nats://127.0.0.1:4299",
		"KTW_HUB_BINARY":   filepath.Join(b.ConfigRoot(), "control-plane", "ktw-hub.exe"),
		"KTW_DATA":         filepath.Join(b.DataRoot(), "work"),
		DotenvKeysVar:      "B_SECRET",
	} {
		t.Setenv(k, v)
	}
}

func TestAnExecutableOfOneInstallationNeverAdoptsAnother(t *testing.T) {
	a, b, aExeDir, _, _ := twoInstallations(t)
	if got := findInstallationFrom(aExeDir, b.Path()); !samePath(got, a.Path()) {
		t.Fatalf("A's sidecar started by B found %q, want A's own %s", got, a.Path())
	}
	// No installation above the executable (a development build, a test): the
	// one that started it, as before.
	if got := findInstallationFrom(t.TempDir(), b.Path()); !samePath(got, b.Path()) {
		t.Fatalf("an executable of no installation found %q, want the inherited %s", got, b.Path())
	}
	if got := findInstallationFrom(t.TempDir(), ""); got != "" {
		t.Fatalf("an executable of no installation, started by nobody, found %q", got)
	}
}

func TestAnInstallationTakesNoLocationFromAnother(t *testing.T) {
	isolateEnv(t)
	a, b, _, _, _ := twoInstallations(t)
	bVariables(t, b)
	applyInstallation(a, b.Path())

	// Only A's roots, registry and control plane.
	if got := os.Getenv("KTW_KATYOTI_ROOT"); !samePath(got, a.ConfigRoot()) {
		t.Errorf("config root = %q, want A's %s", got, a.ConfigRoot())
	}
	// The universe registry and the control plane sit under the config root
	// (defaultRegistryPath reads KTW_KATYOTI_ROOT; it is not called here, as it
	// would pin this test process to a temporary installation).
	if got := os.Getenv("XDG_DATA_HOME"); !samePath(got, a.DataRoot()) {
		t.Errorf("data root = %q, want A's %s", got, a.DataRoot())
	}
	for _, k := range append(slices.Clone(installationInputs), installationDropped...) {
		if v, ok := os.LookupEnv(k); ok {
			t.Errorf("%s=%s came from installation B", k, v)
		}
	}
	// Each one refused is reported, with why.
	got := map[string]string{}
	for _, r := range RefusedInputs() {
		got[r.Name] = r.Why
	}
	for _, k := range []string{"KTW_WORKSPACE", "KTW_UNIVERSE", "KTW_NATS_URL", "KTW_HUB_BINARY", DotenvKeysVar} {
		if !strings.Contains(got[k], "another installation") {
			t.Errorf("%s refused as %q, want it named as another installation's", k, got[k])
		}
	}
	if got["KTW_DATA"] == "" {
		t.Error("KTW_DATA dropped without being reported")
	}
}

func TestAProcessOfTheSameInstallationKeepsWhatItsParentSet(t *testing.T) {
	isolateEnv(t)
	a, _, _, _, _ := twoInstallations(t)
	ws := filepath.Join(a.ConfigRoot(), "universes", "work")
	t.Setenv("KTW_WORKSPACE", ws)
	t.Setenv("KTW_UNIVERSE", "work")
	applyInstallation(a, a.Path())
	if os.Getenv("KTW_WORKSPACE") != ws || os.Getenv("KTW_UNIVERSE") != "work" {
		t.Fatalf("a child of A lost the universe its parent chose: KTW_WORKSPACE=%q KTW_UNIVERSE=%q",
			os.Getenv("KTW_WORKSPACE"), os.Getenv("KTW_UNIVERSE"))
	}
	if len(RefusedInputs()) != 0 {
		t.Fatalf("a child of A refused %v", RefusedInputs())
	}
}

func TestAStartFromAPersonsShellTakesNoLocation(t *testing.T) {
	isolateEnv(t)
	a, _, _, _, _ := twoInstallations(t)
	t.Setenv("KTW_WORKSPACE", `D:\Projets\ktw-agents\.katyoti`)
	applyInstallation(a, "")
	if v, ok := os.LookupEnv("KTW_WORKSPACE"); ok {
		t.Fatalf("KTW_WORKSPACE=%s taken from a shell", v)
	}
	if r := RefusedInputs(); len(r) != 1 || !strings.Contains(r[0].Why, "not set by the installation a") {
		t.Fatalf("refused = %+v", r)
	}
}

func TestTheCurrentFoldersEnvIsReadOnlyInsideTheInstallation(t *testing.T) {
	a, b, _, bProject, bHome := twoInstallations(t)
	aHome := filepath.Join(a.ConfigRoot(), "universes", "work", ".katyoti")
	cases := []struct {
		name string
		inst *Installation
		cwd  string
		home string
		want bool
	}{
		{"no installation reads any folder", nil, b.Dir(), aHome, true},
		{"A in its own folder", a, filepath.Join(a.Dir(), "root"), aHome, true},
		{"A in B's folder", a, b.Dir(), aHome, false},
		{"A in a project of B's universe", a, bProject, aHome, false},
		{"B in a project its universe serves", b, filepath.Join(bProject, "sub"), bHome, true},
		{"B in a sibling of that project", b, bProject + "-other", bHome, false},
	}
	for _, c := range cases {
		if got := cwdEnvAllowedFor(c.inst, c.cwd, c.home); got != c.want {
			t.Errorf("%s: cwd .env allowed = %v, want %v", c.name, got, c.want)
		}
	}
}

// : the lock is the installation's, kept in its katyoti.yml.
func TestTheLockIsKeptInTheInstallationFile(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstallation(dir, "workshop", "katyoti@2026.11.0-dev.1")
	inst.Locked = true
	if err := SaveInstallation(inst); err != nil {
		t.Fatal(err)
	}
	got, err := LoadInstallation(dir)
	if err != nil || !got.Locked {
		t.Fatalf("locked not read back: %+v, %v", got, err)
	}
	got.Locked = false
	if err := SaveInstallation(got); err != nil {
		t.Fatal(err)
	}
	if again, _ := LoadInstallation(dir); again.Locked {
		t.Fatal("unlocked not read back")
	}
}

// : the dev channel's root is adopted in place -
// its roots stay where they are, the programs under it (control plane, store)
// belong to it, and a build folder elsewhere (the repository's bin/, run by the
// tests with their own roots) does not.
func TestTheDevRootIsAdoptedInPlace(t *testing.T) {
	isolateEnv(t)
	root := filepath.Join(t.TempDir(), "katyoti")
	cache := filepath.Join(t.TempDir(), "cache", "katyoti")
	dataBefore := xdgDataHome()
	if _, err := AdoptRoot(root, "katyoti-dev", cache); err == nil {
		t.Fatal("a root that does not exist was adopted")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	inst, err := AdoptRoot(root, "katyoti-dev", cache)
	if err != nil {
		t.Fatal(err)
	}
	got, err := LoadInstallation(root)
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(got.ConfigRoot(), root) || !samePath(got.DataRoot(), dataBefore) || !samePath(got.CacheRoot(), cache) {
		t.Fatalf("roots moved: config %s data %s cache %s; want %s, %s, %s", got.ConfigRoot(), got.DataRoot(), got.CacheRoot(), root, dataBefore, cache)
	}
	// Applied, it sets what the resolvers read to the same folders.
	applyInstallation(got, "")
	if !samePath(os.Getenv("KTW_KATYOTI_ROOT"), root) || !samePath(os.Getenv("XDG_DATA_HOME"), dataBefore) || !samePath(os.Getenv("KTW_CACHE"), cache) {
		t.Fatalf("applied: KTW_KATYOTI_ROOT=%s XDG_DATA_HOME=%s KTW_CACHE=%s", os.Getenv("KTW_KATYOTI_ROOT"), os.Getenv("XDG_DATA_HOME"), os.Getenv("KTW_CACHE"))
	}
	// Its control plane and its store belong to it; a build folder does not.
	for _, d := range []string{filepath.Join(root, "control-plane"), filepath.Join(root, "bin", "store", "ktw-x", "v1"), filepath.Join(root, "bin", "dev")} {
		if f := findInstallationFrom(d, ""); !samePath(f, inst.Path()) {
			t.Errorf("a program in %s found %q, want the dev installation", d, f)
		}
	}
	if f := findInstallationFrom(filepath.Join(t.TempDir(), "repo", "bin"), ""); f != "" {
		t.Errorf("the repository's bin/ found an installation: %s", f)
	}
	if _, err := AdoptRoot(root, "again", cache); err == nil {
		t.Fatal("a root adopted twice")
	}
}
