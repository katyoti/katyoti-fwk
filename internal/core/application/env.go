// SPDX-License-Identifier: Apache-2.0

package application

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// appName is the directory name used in XDG paths and as the global config
// root (~/.config/katyoti/, ~/.local/share/katyoti/, ~/.cache/katyoti/).
// Owned by the application package — cenv embeds no literal of its own.
const appName = "katyoti"

// katyotiFolderName is the per-workspace config folder. A workspace is
// identified by the presence of this folder; HomeDir = <workspaceRoot>/.katyoti.
// This constant is the SINGLE source of truth for the ".katyoti" literal.
// Other packages access workspace paths via Env() or EnvOf(root).
const katyotiFolderName = ".katyoti"

// MachineDirs are the machine-level folders of the installation this process's
// environment names: the global config root (registries, control plane), the
// data root and the cache root.
type MachineDirs struct{ Root, Data, Cache string }

// Machine resolves the MachineDirs from the environment alone — no Env(), no
// preflight, no .env — so it is safe before a command bootstraps (a process
// deciding which installation it belongs to).
func Machine() MachineDirs {
	return MachineDirs{
		Root:  cenv.GlobalAgentsRoot(appName),
		Data:  cenv.ResolveDataDir(appName),
		Cache: cenv.ResolveCacheDir(appName),
	}
}

// Settings holds app-wide settings loaded from Settings.yaml.
type Settings struct {
	DataDir      string `yaml:"data_dir"      envkey:"KTW_DATA"`
	CacheDir     string `yaml:"cache_dir"     envkey:"KTW_CACHE"`
	WorkspaceDir string `yaml:"workspace"`
	MaxTurns     int    `yaml:"max_turns"`
}

// AppEnvironment provides resolved application paths for a single workspace.
//   - HomeDir / ConfigDir: per-workspace config root
//   - DataDir / CacheDir / GlobalHomeDir: derived from cenv resolvers
//
// Access via application.Env() for the ACTIVE workspace (process-wide
// singleton), or application.EnvOf(workspaceRoot) for any other workspace.
type AppEnvironment interface {
	// ── Per-workspace roots ──────────────────────────────────────────
	WorkspaceDir() string // the workspace folder (parent of `.katyoti/`)
	HomeDir() string      // <WorkspaceDir>/.katyoti — config root
	DataDir() string      // per-universe runtime: <XDG_DATA>/<appName>/universes/<name>/ (sessions/servers/logs)
	CacheDir() string     // cache root: <XDG_CACHE>/<appName> by default (flat)

	// ── Cross-workspace global root ──────────────────────────────────
	GlobalHomeDir() string                    // <XDG_CONFIG>/<appName>
	GetGlobalHomeSubDir(domain string) string // <GlobalHomeDir>/<domain>

	// ── Universe (config root; central per the central-universe model) ──
	// UniverseDir is the config + identity root: <GlobalHomeDir>/universes/<name>/
	// when the project's `workspace.yaml` names a universe, else the project's own
	// `.katyoti` folder (a self-contained, pre-split project IS its own universe).
	UniverseDir() string

	// ── Well-known config subdirs (under UniverseDir) ────────────────
	ConfigDir() string // <UniverseDir>/config

	// ── Well-known data subdirs (under DataDir) ──────────────────────
	SessionsDir() string // <DataDir>/sessions
	ServersDir() string  // <DataDir>/servers
	LogDir() string      // <DataDir>/logs

	// ── Tenant stores (under TenantDataDir) ──────────────────────────
	// TenantDataDir is the one root every tenant-keyed store asks its path
	// from. It is the universe's DataDir until the tenant object exists.
	TenantDataDir() string // <DataDir>
	SchedulerDir() string  // <TenantDataDir>/scheduler (the scheduler's store)

	// ── Dynamic subdirs ──────────────────────────────────────────────
	GetHomeSubDir(domain string) string  // <HomeDir>/<domain>
	GetDataSubDir(domain string) string  // <DataDir>/<domain>
	GetCacheSubDir(domain string) string // <CacheDir>/<domain>

	// ── Universe validation ──────────────────────────────────────────
	UniverseName() string
	UniverseError() error
}

// appEnvironment is the concrete implementation.
type appEnvironment struct {
	workspaceDir  string
	homeDir       string
	globalHomeDir string
	universeDir   string
	dataDir       string
	cacheDir      string
	configDir     string
	sessionsDir   string
	serversDir    string
	logDir        string
	tenantDataDir string
	schedulerDir  string
	universeName  string
	universeErr   error
}

func (e *appEnvironment) WorkspaceDir() string  { return e.workspaceDir }
func (e *appEnvironment) HomeDir() string       { return e.homeDir }
func (e *appEnvironment) GlobalHomeDir() string { return e.globalHomeDir }
func (e *appEnvironment) UniverseDir() string   { return e.universeDir }
func (e *appEnvironment) DataDir() string       { return e.dataDir }
func (e *appEnvironment) CacheDir() string      { return e.cacheDir }
func (e *appEnvironment) ConfigDir() string     { return e.configDir }
func (e *appEnvironment) SessionsDir() string   { return e.sessionsDir }
func (e *appEnvironment) ServersDir() string    { return e.serversDir }
func (e *appEnvironment) LogDir() string        { return e.logDir }
func (e *appEnvironment) TenantDataDir() string { return e.tenantDataDir }
func (e *appEnvironment) SchedulerDir() string  { return e.schedulerDir }
func (e *appEnvironment) UniverseName() string  { return e.universeName }
func (e *appEnvironment) UniverseError() error  { return e.universeErr }

func (e *appEnvironment) GetHomeSubDir(domain string) string {
	return filepath.Join(e.homeDir, domain)
}
func (e *appEnvironment) GetGlobalHomeSubDir(domain string) string {
	return filepath.Join(e.globalHomeDir, domain)
}
func (e *appEnvironment) GetDataSubDir(domain string) string {
	return filepath.Join(e.dataDir, domain)
}
func (e *appEnvironment) GetCacheSubDir(domain string) string {
	return filepath.Join(e.cacheDir, domain)
}

// readProjectUniverse reads the universe name a project points at, from
// `<homeDir>/project.yaml` (`universe: <name>`). Named `project.yaml` — NOT
// `workspace.yaml` — to avoid colliding with CWM's own `config/workspace.yaml`
// and to match the terminology (workspace = the CWM graph; this folder is the
// project). A bootstrap-layer read (like cenv's universes.yaml): it runs before
// the config service exists. Empty when the file is absent or names no universe
// (→ the project is its own self-contained universe).
func readProjectUniverse(homeDir string) string {
	data, err := os.ReadFile(filepath.Join(homeDir, "project.yaml"))
	if err != nil {
		return ""
	}
	var w struct {
		Universe string `yaml:"universe"`
	}
	if yaml.Unmarshal(data, &w) != nil {
		return ""
	}
	return w.Universe
}

// newAppEnvironment builds a fully-resolved AppEnvironment for the workspace
// rooted at `workspaceRoot`. The workspace folder is the parent of
// `<workspaceRoot>/.katyoti/`.
//
// Resolution order for DataDir / CacheDir:
//  1. KTW_DATA / KTW_CACHE env var (explicit override) — used as-is
//  2. Settings.yaml `data_dir` / `cache_dir` (universe override)
//  3. <XDG>/<appName>/universes/<universe-name>/ (per-universe default for data;
//     flat <XDG>/<appName>/ for cache)
//
// Universe identity (UniverseName, UniverseError) comes from
// `<HomeDir>/config/Univers.yaml` via cenv.LoadUniverseManifest. Missing or
// invalid manifest yields a "no-universe" name + a diagnostic error.
func newAppEnvironment(workspaceRoot string) AppEnvironment {
	return resolveAppEnvironment(workspaceRoot, os.Getenv("KTW_DATA"))
}

// OwnDataDirOf is the data folder of the universe rooted at workspaceRoot as
// that universe resolves it for itself: its Settings.yaml `data_dir`, else the
// per-universe default under the data root. The calling process's KTW_DATA is
// NOT taken: it names the CALLER's universe data folder (a sidecar inherits its
// hub's), so a process reaching ANOTHER universe's hub (the admin control
// plane) reads that hub's lock file where that hub writes it.
func OwnDataDirOf(workspaceRoot string) string {
	return resolveAppEnvironment(workspaceRoot, "").DataDir()
}

// resolveAppEnvironment builds the environment of workspaceRoot; dataOverride
// is the KTW_DATA override ("" none).
func resolveAppEnvironment(workspaceRoot, dataOverride string) AppEnvironment {
	homeDir := filepath.Join(workspaceRoot, katyotiFolderName)
	globalHomeDir := cenv.GlobalAgentsRoot(appName)

	// Universe (config) root. When the project's `workspace.yaml` names a
	// universe, config + identity live centrally at <GlobalHomeDir>/universes/
	// <name>/ (the central-universe model, modules/katyoti/layout.md §2). Absent → the project's
	// own `.katyoti` folder IS its universe (self-contained, pre-split) — so
	// existing setups resolve exactly as before.
	// A central universe is itself a `.katyoti` config root inside its folder
	// (`<GlobalHomeDir>/universes/<name>/.katyoti`) — the SAME convention as a
	// project's `.katyoti`, and what `ktw-univers create` / system universes
	// produce. So the central universe dir includes `.katyoti`.
	universeDir := homeDir
	if uname := readProjectUniverse(homeDir); uname != "" {
		// THE REGISTRY IS THE AUTHORITY on where a universe lives. Its
		// `home_path` exists so a universe can sit anywhere — commonly inside
		// the repository it configures. Composing the central path
		// unconditionally meant an attached project resolved to
		// `<GlobalHomeDir>/universes/<name>/` even when the registry said the
		// universe was somewhere else entirely, so config and identity were
		// read from a folder that did not exist.
		//
		// The convention remains the FALLBACK, for a name with no registry
		// entry — which is exactly the pre-split, self-contained layout.
		if home := cenv.ResolveUniverseHome(appName, uname); home != "" {
			universeDir = filepath.Join(home, katyotiFolderName)
		} else {
			universeDir = filepath.Join(globalHomeDir, "universes", uname, katyotiFolderName)
		}
	}
	configDir := filepath.Join(universeDir, "config")

	// Load Settings.yaml from configDir.
	var settings Settings
	cfgLoader := loader.NewFSLoader(configDir)
	data, err := cfgLoader.ReadFile("Settings.yaml")
	if err == nil && data != nil {
		format, _ := marshal.FormatFor("Settings.yaml")
		_ = config.DecodeBytes(data, format, &settings)
	}

	// Universe identity — Univers.yaml lives under the universe's config dir
	// (central when workspace.yaml names one; the project's own .katyoti/config
	// otherwise, unchanged from before).
	manifest, manifestErr := cenv.LoadUniverseManifest(universeDir)
	universeName := "no-universe"
	if manifest != nil {
		universeName = manifest.Name
	}

	// DataDir resolution — PER-UNIVERSE runtime state (sessions / servers / logs).
	// The hub is one-per-universe (universe-workspace-management.md), so its
	// runtime state is keyed by universe, NOT per-project. (Per-PROJECT authored
	// data — CWM graph, documents — lives under the project's own `.katyoti`,
	// resolved separately.) Overrides (KTW_DATA env / Settings.data_dir) still win.
	dataDir := dataOverride
	if dataDir == "" {
		dataDir = settings.DataDir
	}
	if dataDir == "" {
		dataDir = filepath.Join(cenv.DataRoot(appName), "universes", universeName)
	} else if !filepath.IsAbs(dataDir) {
		dataDir = filepath.Join(homeDir, dataDir)
	} else {
		dataDir = cenv.KeepTestsOut(filepath.Clean(dataDir))
	}

	// CacheDir resolution.
	cacheDir := settings.CacheDir
	if cacheDir == "" {
		cacheDir = cenv.ResolveCacheDir(appName)
	} else if !filepath.IsAbs(cacheDir) {
		cacheDir = filepath.Join(homeDir, cacheDir)
	}

	return &appEnvironment{
		workspaceDir:  workspaceRoot,
		homeDir:       homeDir,
		globalHomeDir: globalHomeDir,
		universeDir:   universeDir,
		dataDir:       dataDir,
		cacheDir:      cacheDir,
		configDir:     configDir,
		sessionsDir:   filepath.Join(dataDir, "sessions"),
		serversDir:    filepath.Join(dataDir, "servers"),
		logDir:        filepath.Join(dataDir, "logs"),
		tenantDataDir: dataDir,
		schedulerDir:  filepath.Join(dataDir, "scheduler"),
		universeName:  universeName,
		universeErr:   manifestErr,
	}
}
