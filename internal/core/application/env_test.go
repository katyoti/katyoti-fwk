// SPDX-License-Identifier: Apache-2.0

package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

func TestNewAppEnvironment(t *testing.T) {
	env := newAppEnvironment(t.TempDir())

	if env.HomeDir() == "" {
		t.Error("expected non-empty HomeDir")
	}
	if !filepath.IsAbs(env.HomeDir()) {
		t.Errorf("HomeDir not absolute: %q", env.HomeDir())
	}
	if !filepath.IsAbs(env.DataDir()) {
		t.Errorf("DataDir not absolute: %q", env.DataDir())
	}
	if !filepath.IsAbs(env.CacheDir()) {
		t.Errorf("CacheDir not absolute: %q", env.CacheDir())
	}
	if env.ConfigDir() != filepath.Join(env.HomeDir(), "config") {
		t.Errorf("ConfigDir = %q, want %q", env.ConfigDir(), filepath.Join(env.HomeDir(), "config"))
	}
	if !filepath.IsAbs(env.WorkspaceDir()) {
		t.Errorf("WorkspaceDir not absolute: %q", env.WorkspaceDir())
	}
	if !filepath.IsAbs(env.SessionsDir()) {
		t.Errorf("SessionsDir not absolute: %q", env.SessionsDir())
	}
	if !filepath.IsAbs(env.ServersDir()) {
		t.Errorf("ServersDir not absolute: %q", env.ServersDir())
	}
	if !filepath.IsAbs(env.LogDir()) {
		t.Errorf("LogDir not absolute: %q", env.LogDir())
	}
}

func TestNewAppEnvironment_3Roots_Independent(t *testing.T) {
	env := newAppEnvironment(t.TempDir())
	// Per-universe runtime (DataDir) + ephemeral cache (CacheDir) are NOT under
	// the project's HomeDir — different scopes (universe vs project).
	if env.DataDir() == env.HomeDir() {
		t.Error("DataDir should not equal HomeDir")
	}
	if env.CacheDir() == env.HomeDir() {
		t.Error("CacheDir should not equal HomeDir")
	}
}

func TestNewAppEnvironment_SubDirs(t *testing.T) {
	env := newAppEnvironment(t.TempDir())

	if got := env.GetHomeSubDir("skills"); got != filepath.Join(env.HomeDir(), "skills") {
		t.Errorf("GetHomeSubDir(skills) = %q", got)
	}
	if got := env.GetDataSubDir("kb"); got != filepath.Join(env.DataDir(), "kb") {
		t.Errorf("GetDataSubDir(kb) = %q", got)
	}
	if got := env.GetCacheSubDir("web"); got != filepath.Join(env.CacheDir(), "web") {
		t.Errorf("GetCacheSubDir(web) = %q", got)
	}
}

func TestNewAppEnvironment_SessionsUnderData(t *testing.T) {
	env := newAppEnvironment(t.TempDir())
	if env.SessionsDir() != filepath.Join(env.DataDir(), "sessions") {
		t.Errorf("SessionsDir = %q, want under DataDir", env.SessionsDir())
	}
}

func TestNewAppEnvironment_ServersUnderData(t *testing.T) {
	env := newAppEnvironment(t.TempDir())
	if env.ServersDir() != filepath.Join(env.DataDir(), "servers") {
		t.Errorf("ServersDir = %q, want under DataDir", env.ServersDir())
	}
}

// TestNewAppEnvironment_TenantAndSchedulerDirs pins the tenant stores' root:
// TenantDataDir is the universe DataDir until the tenant object exists, and
// the scheduler's store folder sits under it - also under a KTW_DATA override.
func TestNewAppEnvironment_TenantAndSchedulerDirs(t *testing.T) {
	check := func(t *testing.T, env AppEnvironment) {
		t.Helper()
		if env.TenantDataDir() != env.DataDir() {
			t.Errorf("TenantDataDir = %q, want DataDir %q", env.TenantDataDir(), env.DataDir())
		}
		if want := filepath.Join(env.TenantDataDir(), "scheduler"); env.SchedulerDir() != want {
			t.Errorf("SchedulerDir = %q, want %q", env.SchedulerDir(), want)
		}
		if !filepath.IsAbs(env.SchedulerDir()) {
			t.Errorf("SchedulerDir not absolute: %q", env.SchedulerDir())
		}
	}
	t.Run("default", func(t *testing.T) { check(t, newAppEnvironment(t.TempDir())) })
	t.Run("KTW_DATA", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("KTW_DATA", dir)
		env := newAppEnvironment(t.TempDir())
		check(t, env)
		if env.SchedulerDir() != filepath.Join(dir, "scheduler") {
			t.Errorf("SchedulerDir = %q, want under KTW_DATA %q", env.SchedulerDir(), dir)
		}
	})
}

func TestNewAppEnvironment_WithKTWWorkspace(t *testing.T) {
	t.Setenv("KTW_WORKSPACE", t.TempDir())
	env := newAppEnvironment(t.TempDir())
	if env.HomeDir() == "" {
		t.Error("expected non-empty HomeDir with KTW_WORKSPACE set")
	}
}

func TestNewAppEnvironment_WithKTWData(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KTW_DATA", dir)
	env := newAppEnvironment(t.TempDir())
	if env.DataDir() != dir {
		t.Errorf("DataDir = %q, want %q", env.DataDir(), dir)
	}
}

func TestNewAppEnvironment_WithKTWCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KTW_CACHE", dir)
	env := newAppEnvironment(t.TempDir())
	if env.CacheDir() != dir {
		t.Errorf("CacheDir = %q, want %q", env.CacheDir(), dir)
	}
}

// Universe-aware Env() tests (added by universe-config-scoping todo).

// DataDir defaults to <data-root>/universes/<name>/ when no override is set;
// <name> comes from <HomeDir>/config/Univers.yaml's `name` field — the
// authoritative universe identity (NOT the basename of HomeDir).
func TestNewAppEnvironment_DataDirUniverseScoped(t *testing.T) {
	// Post-cutover layout: workspace folder contains .katyoti/, and Univers.yaml
	// lives at <workspace>/.katyoti/config/Univers.yaml.
	workspaceRoot := filepath.Join(t.TempDir(), "ktw-universes", "acme")
	configDir := EnvOf(workspaceRoot).ConfigDir()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(configDir, "Univers.yaml"),
		[]byte("name: acme\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	cenv.ResetUniverseManifestCache()
	t.Setenv("KTW_WORKSPACE", workspaceRoot)
	t.Setenv("KTW_DATA", "")
	env := newAppEnvironment(workspaceRoot)
	if env.UniverseError() != nil {
		t.Fatalf("UniverseError = %v, want nil (Univers.yaml is valid)", env.UniverseError())
	}
	if env.UniverseName() != "acme" {
		t.Errorf("UniverseName = %q, want acme", env.UniverseName())
	}
	// Runtime data (sessions/servers/logs) is per-UNIVERSE (the hub is one-per-
	// universe) — <XDG>/universes/<manifest-name>/.
	if filepath.Base(env.DataDir()) != "acme" {
		t.Errorf("DataDir basename = %q, want acme (universe name)", filepath.Base(env.DataDir()))
	}
	if filepath.Base(filepath.Dir(env.DataDir())) != "universes" {
		t.Errorf("DataDir parent basename = %q, want universes", filepath.Base(filepath.Dir(env.DataDir())))
	}
}

// When Univers.yaml is missing, UniverseError must be non-nil and DataDir
// falls back to a "no-universe" placeholder so paths remain well-formed for
// diagnostics.
func TestNewAppEnvironment_MissingManifestErrors(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "ktw-universes", "broken")
	if err := os.MkdirAll(EnvOf(workspaceRoot).HomeDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	cenv.ResetUniverseManifestCache()
	t.Setenv("KTW_WORKSPACE", workspaceRoot)
	t.Setenv("KTW_DATA", "")
	env := newAppEnvironment(workspaceRoot)
	if env.UniverseError() == nil {
		t.Fatal("UniverseError = nil, want non-nil for missing Univers.yaml")
	}
	if !strings.Contains(env.UniverseError().Error(), "Univers.yaml") {
		t.Errorf("UniverseError message should mention Univers.yaml, got: %v", env.UniverseError())
	}
	if env.UniverseName() != "no-universe" {
		t.Errorf("UniverseName = %q, want no-universe placeholder", env.UniverseName())
	}
	// Per-universe runtime data falls back to the "no-universe" placeholder name.
	if filepath.Base(env.DataDir()) != "no-universe" {
		t.Errorf("DataDir basename = %q, want no-universe", filepath.Base(env.DataDir()))
	}
}

// KTW_DATA explicit override is honored as-is (no universe suffix appended)
// — used by test isolation and single-tenant deployments.
func TestNewAppEnvironment_KTWDataExplicitOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KTW_DATA", dir)
	env := newAppEnvironment(t.TempDir())
	if env.DataDir() != filepath.Clean(dir) {
		t.Errorf("DataDir = %q, want %q (KTW_DATA must be honored verbatim)", env.DataDir(), dir)
	}
}

// GlobalHomeDir is independent of HomeDir; honors KTW_KATYOTI_ROOT.
func TestNewAppEnvironment_GlobalHomeDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", tmp)
	env := newAppEnvironment(t.TempDir())
	if env.GlobalHomeDir() != filepath.Clean(tmp) {
		t.Errorf("GlobalHomeDir = %q, want %q", env.GlobalHomeDir(), tmp)
	}
}

// GetGlobalHomeSubDir returns a path under GlobalHomeDir.
func TestNewAppEnvironment_GetGlobalHomeSubDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", tmp)
	env := newAppEnvironment(t.TempDir())
	want := filepath.Join(filepath.Clean(tmp), "skills")
	if got := env.GetGlobalHomeSubDir("skills"); got != want {
		t.Errorf("GetGlobalHomeSubDir(skills) = %q, want %q", got, want)
	}
}

// HomeDir (universe) and GlobalHomeDir (cross-universe) are independent.
func TestNewAppEnvironment_HomeAndGlobalIndependent(t *testing.T) {
	rootTmp := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", rootTmp)
	// No KTW_HOME here: it was REMOVED with the workspace model and nothing
	// reads it, so setting it only made this test LOOK like it exercised a
	// variable that has no effect. The home dir comes from the workspace root
	// passed below.
	env := newAppEnvironment(t.TempDir())
	if env.HomeDir() == env.GlobalHomeDir() {
		t.Errorf("HomeDir (%q) must differ from GlobalHomeDir (%q)", env.HomeDir(), env.GlobalHomeDir())
	}
	if env.GetHomeSubDir("skills") == env.GetGlobalHomeSubDir("skills") {
		t.Error("universe skills dir and global skills dir must differ")
	}
}

// A project whose workspace.yaml names a universe resolves config/identity to
// the CENTRAL universe dir; the project dir is unchanged.
func TestNewAppEnvironment_CentralUniverse(t *testing.T) {
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".katyoti"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".katyoti", "project.yaml"), []byte("universe: acme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := newAppEnvironment(project)
	wantUniv := filepath.Join(env.GlobalHomeDir(), "universes", "acme", ".katyoti")
	if env.UniverseDir() != wantUniv {
		t.Fatalf("UniverseDir = %q, want %q", env.UniverseDir(), wantUniv)
	}
	if env.ConfigDir() != filepath.Join(wantUniv, "config") {
		t.Fatalf("ConfigDir = %q, want <UniverseDir>/config", env.ConfigDir())
	}
	if env.WorkspaceDir() != project {
		t.Fatalf("WorkspaceDir = %q, want the project folder %q", env.WorkspaceDir(), project)
	}
}

// A project with no workspace.yaml is its own universe — config resolves to
// <project>/.katyoti/config exactly as before the split (behavior-preserving).
func TestNewAppEnvironment_SelfContainedProject(t *testing.T) {
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	project := t.TempDir()
	env := newAppEnvironment(project)
	wantUniv := filepath.Join(project, ".katyoti")
	if env.UniverseDir() != wantUniv {
		t.Fatalf("UniverseDir = %q, want the project .katyoti %q", env.UniverseDir(), wantUniv)
	}
	if env.ConfigDir() != filepath.Join(wantUniv, "config") {
		t.Fatalf("ConfigDir = %q, want project-local config", env.ConfigDir())
	}
}

// OwnDataDirOf is a universe's data folder as that universe
// resolves it, never the calling process's KTW_DATA.
func TestOwnDataDirOfIgnoresKTWData(t *testing.T) {
	t.Setenv("KTW_DATA", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	own := t.TempDir()
	root := t.TempDir()
	cfg := filepath.Join(root, katyotiFolderName, "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "Settings.yaml"), []byte("data_dir: "+filepath.ToSlash(own)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := OwnDataDirOf(root); filepath.Clean(got) != filepath.Clean(own) {
		t.Errorf("OwnDataDirOf = %s, want the universe's data_dir %s", got, own)
	}

	bare := t.TempDir()
	want := filepath.Join(os.Getenv("XDG_DATA_HOME"), appName, "universes", "no-universe")
	if got := OwnDataDirOf(bare); filepath.Clean(got) != filepath.Clean(want) {
		t.Errorf("OwnDataDirOf(no settings) = %s, want the default %s", got, want)
	}
	if got := EnvOf(bare).DataDir(); filepath.Clean(got) != filepath.Clean(os.Getenv("KTW_DATA")) {
		t.Errorf("EnvOf still takes KTW_DATA: %s", got)
	}
}
