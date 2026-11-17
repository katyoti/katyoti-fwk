// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallationRootsAreRelativeToItsFile(t *testing.T) {
	dir := t.TempDir()
	inst := NewInstallation(dir, "acme 1.004", "acme@1.004")
	inst.Settings = map[string]any{"default_universe": "demo"}
	if err := SaveInstallation(inst); err != nil {
		t.Fatal(err)
	}
	got, err := LoadInstallation(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "acme 1.004" || got.Solution != "acme@1.004" || got.Settings["default_universe"] != "demo" {
		t.Fatalf("read back %+v", got)
	}
	if got.ConfigRoot() != filepath.Join(dir, "root") || got.DataRoot() != filepath.Join(dir, "data") || got.CacheRoot() != filepath.Join(dir, "cache") {
		t.Fatalf("roots %s %s %s", got.ConfigRoot(), got.DataRoot(), got.CacheRoot())
	}
}

func TestAnInstallationHasAName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, InstallationFile), []byte("roots: {config: root}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadInstallation(dir); err == nil {
		t.Fatal("an installation file with no name should be refused")
	}
}

func TestTheFileIsFoundAboveTheExecutable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, InstallationFile), []byte("name: a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(dir, "root", "bin", "store", "ktw-kb", "1.0.0", "windows-amd64")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := walkUpFor(deep, InstallationFile, installationWalk); got != filepath.Join(dir, InstallationFile) {
		t.Fatalf("from a sidecar's folder: %q", got)
	}
	if got := walkUpFor(deep, InstallationFile, 2); got != "" {
		t.Fatalf("beyond the walk: %q", got)
	}
}

// The installation's roots win over inherited ones; KTW_DATA is dropped; a
// variable of env: reads the .env beside the file.
func TestAnInstallationSetsItsRootsOverInheritedOnes(t *testing.T) {
	dir := t.TempDir()
	for k, v := range map[string]string{"KTW_KATYOTI_ROOT": "/another/root", "KTW_DATA": "/another/data",
		"XDG_DATA_HOME": "", "KTW_CACHE": "", "KTW_INFRA_CHANNEL": "/old/channel", InstallationEnv: "", "KTW_INSTANCE": "",
		"KTW_E2E_TENANT": "", "KTW_E2E_FROM_DOTENV": ""} {
		t.Setenv(k, v)
		if v == "" {
			_ = os.Unsetenv(k) // restored by t.Setenv
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("KTW_E2E_FROM_DOTENV=acme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inst := NewInstallation(dir, "b", "")
	inst.Env = map[string]string{"KTW_E2E_TENANT": "tenant-${{env:KTW_E2E_FROM_DOTENV}}"}
	applyInstallation(inst, "")
	for k, want := range map[string]string{"KTW_KATYOTI_ROOT": filepath.Join(dir, "root"), "XDG_DATA_HOME": filepath.Join(dir, "data"),
		"KTW_CACHE": filepath.Join(dir, "cache"), InstallationEnv: filepath.Join(dir, InstallationFile), "KTW_INSTANCE": "b",
		"KTW_E2E_TENANT": "tenant-acme"} {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	for _, k := range []string{"KTW_DATA", "KTW_INFRA_CHANNEL"} {
		if _, ok := os.LookupEnv(k); ok {
			t.Errorf("%s inherited into an installation", k)
		}
	}
}

// : a test run from a program of an installation does not take
// the installation it inherited (its roots would win over the test's own); a
// process told to stay out of every installation, with a root of its own (a
// scenario case's sandbox), takes none, even above its executable.
func TestATestAndASandboxTakeNoInstallation(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, InstallationFile)
	if err := os.WriteFile(file, []byte("name: dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(InstallationEnv, file)
	t.Setenv(NoInstallationEnv, "")
	if got := installationPath("", true); got != "" {
		t.Errorf("a test took the inherited installation %s", got)
	}
	if got := installationPath("", false); got != file {
		t.Errorf("a program of the installation's children: %q, want %q", got, file)
	}
	bin := filepath.Join(dir, "bin", "dev")
	t.Setenv(NoInstallationEnv, "1")
	t.Setenv("KTW_KATYOTI_ROOT", "")
	if got := installationPath(bin, false); got != file {
		t.Errorf("without a root of its own the program keeps its installation: %q", got)
	}
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(t.TempDir(), "sandbox"))
	if got := installationPath(bin, false); got != "" {
		t.Errorf("a sandboxed program took the installation %s", got)
	}
}

func TestAProgramInNoInstallationIsFound(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, InstallationFile), []byte("name: dev\nroots: {config: .}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if InstallationAbove(filepath.Join(dir, "bin", "dev", "ktw-a.exe")) == "" {
		t.Error("a program of bin/dev belongs to the installation")
	}
	if got := InstallationAbove(filepath.Join(t.TempDir(), "checkout", "bin", "ktw-a.exe")); got != "" {
		t.Errorf("a checkout's bin/ is in no installation, got %s", got)
	}
	if inst := RootInstallation(dir); inst == nil || inst.Name != "dev" {
		t.Errorf("an adopted root is its installation's: %+v", inst)
	}
	if RootInstallation(t.TempDir()) != nil {
		t.Error("a plain root has no installation")
	}
	if !Within(filepath.Join(dir, "bin"), dir) || Within(dir+"x", dir) {
		t.Error("Within")
	}
}
