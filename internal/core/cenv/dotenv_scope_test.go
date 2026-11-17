// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// isolateDotenv unsets keys for the test (restored after), points the global
// root at global, empties the .env statuses and moves into cwd.
func isolateDotenv(t *testing.T, global, cwd string, keys ...string) {
	t.Helper()
	for _, k := range append(keys, GlobalKeysVar, DotenvKeysVar, "KTW_WORKSPACE", "KTW_UNIVERSE") {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	t.Setenv("KTW_KATYOTI_ROOT", global)
	saved := envFileStatuses
	envFileStatuses = nil
	t.Cleanup(func() { envFileStatuses = saved })
	wd, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

func writeEnv(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func noInstallation() *Installation { return nil }

// : a universe whose.env holds no connector credential never
// takes the global .env's - that is another scope's account; a key the
// global .env declares global (KTW_GLOBAL_KEYS, by name or by prefix) still
// fills, and the universe's own key wins.
func TestAUniverseNeverTakesTheGlobalEnvsUndeclaredKey(t *testing.T) {
	home, global, cwd := t.TempDir(), t.TempDir(), t.TempDir()
	isolateDotenv(t, global, cwd, "CONFLUENCE_API_KEY", "CONFLUENCE_USER_NAME", "ANTHROPIC_API_KEY", "CENV_F028_SHARED", "CENV_F028_OWN")
	writeEnv(t, home, "CENV_F028_OWN=universe\n")
	writeEnv(t, global, GlobalKeysVar+"=ANTHROPIC_*, CENV_F028_SHARED\n"+
		"CONFLUENCE_API_KEY=client-a\nCONFLUENCE_USER_NAME=a@client-a\n"+
		"ANTHROPIC_API_KEY=shared\nCENV_F028_SHARED=s\nCENV_F028_OWN=global\n")

	loadDotenvFor("katyoti-f028-test", home, noInstallation)

	for _, k := range []string{"CONFLUENCE_API_KEY", "CONFLUENCE_USER_NAME"} {
		if v, ok := os.LookupEnv(k); ok {
			t.Errorf("%s=%q filled from the global .env: another scope's credential", k, v)
		}
	}
	for k, want := range map[string]string{"ANTHROPIC_API_KEY": "shared", "CENV_F028_SHARED": "s", "CENV_F028_OWN": "universe"} {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	var withheld []string
	for _, s := range EnvFileStatuses() {
		if s.Path == filepath.Join(global, ".env") {
			withheld = s.Withheld
		}
	}
	if !slices.Contains(withheld, "CONFLUENCE_API_KEY") || slices.Contains(withheld, "ANTHROPIC_API_KEY") {
		t.Errorf("the global .env's withheld keys = %v, want CONFLUENCE_API_KEY and not ANTHROPIC_API_KEY", withheld)
	}
}

// : the working-folder.env of a project attached to the
// universe is outside its scope: it never fills a connector key, only a key
// declared global; the installation's own folder still reads every key.
func TestAnAttachedProjectsEnvNeverFillsAConnectorKey(t *testing.T) {
	_, b, _, bProject, bHome := twoInstallations(t)
	global := t.TempDir()
	writeEnv(t, global, GlobalKeysVar+"=CENV_F028_SHARED\n")
	writeEnv(t, bProject, "CONFLUENCE_API_KEY=from-the-project\nCENV_F028_SHARED=p\n")
	installed := func() *Installation { return b }

	isolateDotenv(t, global, bProject, "CONFLUENCE_API_KEY", "CENV_F028_SHARED")
	loadDotenvFor("katyoti-f028-test", bHome, installed)
	if v, ok := os.LookupEnv("CONFLUENCE_API_KEY"); ok {
		t.Errorf("CONFLUENCE_API_KEY=%q filled from an attached project's .env", v)
	}
	if got := os.Getenv("CENV_F028_SHARED"); got != "p" {
		t.Errorf("CENV_F028_SHARED = %q, want the project's value of a key declared global", got)
	}

	// The installation's own folder is a scope above every universe: its
	// .env fills only a key declared global; the universe's folder, every key.
	own := filepath.Join(b.Dir(), "root")
	writeEnv(t, own, "CONFLUENCE_API_KEY=the-installations\nCENV_F028_SHARED=i\n")
	isolateDotenv(t, global, own, "CONFLUENCE_API_KEY", "CENV_F028_SHARED")
	loadDotenvFor("katyoti-f028-test", bHome, installed)
	if v, ok := os.LookupEnv("CONFLUENCE_API_KEY"); ok {
		t.Errorf("CONFLUENCE_API_KEY=%q filled from the installation's folder .env", v)
	}
	if got := os.Getenv("CENV_F028_SHARED"); got != "i" {
		t.Errorf("CENV_F028_SHARED = %q in the installation's folder, want its value of a key declared global", got)
	}
	writeEnv(t, bHome, "CONFLUENCE_API_KEY=the-universes\n")
	isolateDotenv(t, global, bHome, "CONFLUENCE_API_KEY")
	loadDotenvFor("katyoti-f028-test", bHome, installed)
	if got := os.Getenv("CONFLUENCE_API_KEY"); got != "the-universes" {
		t.Errorf("CONFLUENCE_API_KEY = %q in the universe's own folder, want its value", got)
	}
}

// : an installation adopted in place (roots.config ".", the
// dev channel) has its folder as its config root, so its .env is the global
// .env - read before the universe's by applyInstallation, it still fills only
// the keys declared global; inst.Env's ${{env:KEY}} reads any key of it.
func TestAnInPlaceInstallationsEnvNeverFillsAConnectorKey(t *testing.T) {
	isolateEnv(t)
	root := t.TempDir()
	home := filepath.Join(root, "universes", "work", ".katyoti")
	writeEnv(t, home, "")
	writeEnv(t, root, GlobalKeysVar+"=CENV_F028_SHARED\n"+
		"CONFLUENCE_API_KEY=client-a\nCENV_F028_SHARED=s\nCENV_F028_TENANT=acme\n")
	isolateDotenv(t, root, root, "CONFLUENCE_API_KEY", "CENV_F028_SHARED", "CENV_F028_TENANT", "CENV_F028_FROM_FILE")
	inst := NewInstallation(root, "katyoti-dev", "")
	inst.Roots.Config = "."
	inst.Env = map[string]string{"CENV_F028_FROM_FILE": "tenant-${{env:CENV_F028_TENANT}}"}

	applyInstallation(inst, "")
	loadDotenvFor("katyoti-f028-test", home, func() *Installation { return inst })

	for _, k := range []string{"CONFLUENCE_API_KEY", "CENV_F028_TENANT"} {
		if v, ok := os.LookupEnv(k); ok {
			t.Errorf("%s=%q reached the universe through the installation folder's .env", k, v)
		}
	}
	for k, want := range map[string]string{"CENV_F028_SHARED": "s", "CENV_F028_FROM_FILE": "tenant-acme"} {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}
