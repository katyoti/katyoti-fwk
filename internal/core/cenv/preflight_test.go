// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"testing"
)

// withCleanUniverseEnv unsets all workspace/universe-related env vars for the
// duration of the test, restoring originals at end.
func withCleanUniverseEnv(t *testing.T) func() {
	t.Helper()
	keys := []string{"KTW_WORKSPACE", "KTW_UNIVERSE", "KTW_KATYOTI_ROOT", "KTW_HOME", GlobalKeysVar}
	saved := make(map[string]string, len(keys))
	for _, k := range keys {
		saved[k] = os.Getenv(k)
		_ = os.Unsetenv(k)
	}
	return func() {
		for k, v := range saved {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}
}

func TestScanWorkspaceFlags_WorkspaceOnly(t *testing.T) {
	ws, universe, err := scanWorkspaceFlags([]string{"--workspace", "/path/to/uni"})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if ws != "/path/to/uni" {
		t.Errorf("workspace = %q, want /path/to/uni", ws)
	}
	if universe != "" {
		t.Errorf("universe = %q, want empty", universe)
	}
}

func TestScanWorkspaceFlags_WorkspaceEqualsForm(t *testing.T) {
	ws, _, err := scanWorkspaceFlags([]string{"--workspace=/path/to/uni"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ws != "/path/to/uni" {
		t.Errorf("workspace = %q, want /path/to/uni", ws)
	}
}

func TestScanWorkspaceFlags_UniverseOnly(t *testing.T) {
	_, universe, err := scanWorkspaceFlags([]string{"--universe", "acme"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if universe != "acme" {
		t.Errorf("universe = %q, want acme", universe)
	}
}

func TestScanWorkspaceFlags_MutualExclusion(t *testing.T) {
	_, _, err := scanWorkspaceFlags([]string{"--workspace", "/a", "--universe", "b"})
	if err == nil {
		t.Fatalf("expected mutex error, got nil")
	}
}

func TestScanWorkspaceFlags_TrailingFlagIgnored(t *testing.T) {
	// "--workspace" with no value should be silently ignored (Cobra surfaces the proper error later).
	ws, _, err := scanWorkspaceFlags([]string{"--workspace"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ws != "" {
		t.Errorf("workspace = %q, want empty", ws)
	}
}

func TestPreflight_FlagWorkspaceWins(t *testing.T) {
	defer withCleanUniverseEnv(t)()
	tmp := t.TempDir()
	got := Preflight("testapp", []string{"--workspace", tmp})
	if got != filepath.Clean(tmp) {
		t.Errorf("got %q, want %q", got, tmp)
	}
	if os.Getenv("KTW_WORKSPACE") != filepath.Clean(tmp) {
		t.Errorf("KTW_WORKSPACE not set correctly")
	}
	info := PreflightInfo()
	if info.Source != "flag-workspace" {
		t.Errorf("Source = %q, want flag-workspace", info.Source)
	}
}

func TestPreflight_FlagUniverseResolvesToWorkspace(t *testing.T) {
	defer withCleanUniverseEnv(t)()
	tmp := t.TempDir()
	universeHome := filepath.Join(tmp, "homes", "acme")
	if err := os.MkdirAll(universeHome, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := filepath.Join(tmp, "universes.yaml")
	body := "universes:\n  - name: acme\n    home_path: " + universeHome + "\n"
	if err := os.WriteFile(registry, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Setenv("KTW_KATYOTI_ROOT", tmp)

	got := Preflight("testapp", []string{"--universe", "acme"})
	if got != filepath.Clean(universeHome) {
		t.Errorf("got %q, want %q", got, universeHome)
	}
	if PreflightInfo().Source != "flag-universe" {
		t.Errorf("Source = %q, want flag-universe", PreflightInfo().Source)
	}
}

func TestPreflight_UniverseNameNotFound(t *testing.T) {
	defer withCleanUniverseEnv(t)()
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "universes.yaml"), []byte("universes: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Setenv("KTW_KATYOTI_ROOT", tmp)

	got := Preflight("testapp", []string{"--universe", "missing"})
	if got == "" {
		t.Errorf("expected fallback path, got empty")
	}
	info := PreflightInfo()
	if info.Source != "fallback" {
		t.Errorf("Source = %q, want fallback", info.Source)
	}
	if info.Err == nil {
		t.Errorf("expected Err to be set, got nil")
	}
}

func TestPreflight_KTWWorkspaceEnvVar(t *testing.T) {
	defer withCleanUniverseEnv(t)()
	tmp := t.TempDir()
	_ = os.Setenv("KTW_WORKSPACE", tmp)
	got := Preflight("testapp", []string{})
	if got != filepath.Clean(tmp) {
		t.Errorf("got %q, want %q", got, tmp)
	}
	if PreflightInfo().Source != "env-workspace" {
		t.Errorf("Source = %q, want env-workspace", PreflightInfo().Source)
	}
}

func TestPreflight_FallbackWhenNothingSet(t *testing.T) {
	defer withCleanUniverseEnv(t)()
	got := Preflight("testapp", []string{})
	if got == "" {
		t.Errorf("expected fallback path, got empty")
	}
	if PreflightInfo().Source != "fallback" {
		t.Errorf("Source = %q, want fallback", PreflightInfo().Source)
	}
	// Fallback path ends with /<katyoti-root>/universes/default
	if filepath.Base(got) != "default" {
		t.Errorf("fallback basename = %q, want default", filepath.Base(got))
	}
}

func TestPreflight_FlagWinsOverEnv(t *testing.T) {
	defer withCleanUniverseEnv(t)()
	tmp := t.TempDir()
	flagWS := filepath.Join(tmp, "from-flag")
	envWS := filepath.Join(tmp, "from-env")
	_ = os.MkdirAll(flagWS, 0o755)
	_ = os.MkdirAll(envWS, 0o755)
	_ = os.Setenv("KTW_WORKSPACE", envWS)

	got := Preflight("testapp", []string{"--workspace", flagWS})
	if got != filepath.Clean(flagWS) {
		t.Errorf("got %q, want %q (flag should win over env)", got, flagWS)
	}
}

func TestGlobalAgentsRoot_HonorsOverride(t *testing.T) {
	defer withCleanUniverseEnv(t)()
	tmp := t.TempDir()
	_ = os.Setenv("KTW_KATYOTI_ROOT", tmp)
	if got := GlobalAgentsRoot("testapp"); got != filepath.Clean(tmp) {
		t.Errorf("GlobalAgentsRoot = %q, want %q", got, tmp)
	}
}

// --home, the flag the binaries declare, selects the universe home — both
// forms — and excludes --universe like --workspace does.
func TestScanWorkspaceFlags_HomeIsTheUniverseHome(t *testing.T) {
	for _, args := range [][]string{{"--home", "/path/to/uni"}, {"--home=/path/to/uni"}} {
		ws, universe, err := scanWorkspaceFlags(args)
		if err != nil || ws != "/path/to/uni" || universe != "" {
			t.Errorf("%v: home %q universe %q err %v; want /path/to/uni", args, ws, universe, err)
		}
	}
	if _, _, err := scanWorkspaceFlags([]string{"--home", "/a", "--universe", "b"}); err == nil {
		t.Error("--home with --universe: want the mutex error")
	}
}

func TestPreflight_FlagHomeWins(t *testing.T) {
	defer withCleanUniverseEnv(t)()
	tmp := t.TempDir()
	if got := Preflight("testapp", []string{"--home", tmp}); got != filepath.Clean(tmp) || os.Getenv("KTW_WORKSPACE") != filepath.Clean(tmp) {
		t.Errorf("Preflight(--home) = %q, KTW_WORKSPACE %q; want %q", got, os.Getenv("KTW_WORKSPACE"), tmp)
	}
}

// A key set in the universe's .env and in the global one takes the universe's
// value — preflight reads only the selection keys from the global .env, so it
// cannot load a global key ahead of the universe's own.
func TestDotenv_TheUniverseWinsOverGlobal(t *testing.T) {
	defer withCleanUniverseEnv(t)()
	root, home := t.TempDir(), t.TempDir()
	_ = os.Setenv("KTW_KATYOTI_ROOT", root)
	_ = os.WriteFile(filepath.Join(root, ".env"), []byte(GlobalKeysVar+"=KTW_TEST_*\nKTW_TEST_SCOPED=global\nKTW_TEST_GLOBAL_ONLY=g\n"), 0o600)
	_ = os.WriteFile(filepath.Join(home, ".env"), []byte("KTW_TEST_SCOPED=universe\n"), 0o600)
	for _, k := range []string{"KTW_TEST_SCOPED", "KTW_TEST_GLOBAL_ONLY"} {
		_ = os.Unsetenv(k)
		t.Cleanup(func() { _ = os.Unsetenv(k) })
	}
	saved := envFileStatuses
	envFileStatuses = nil
	t.Cleanup(func() { envFileStatuses = saved })

	Preflight("testapp", []string{})
	LoadDotenv("testapp", home)

	if got := os.Getenv("KTW_TEST_SCOPED"); got != "universe" {
		t.Errorf("KTW_TEST_SCOPED = %q, want the universe's value", got)
	}
	if got := os.Getenv("KTW_TEST_GLOBAL_ONLY"); got != "g" {
		t.Errorf("KTW_TEST_GLOBAL_ONLY = %q, want the global value", got)
	}
}
