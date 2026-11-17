// SPDX-License-Identifier: Apache-2.0

// Package cenv provides typed environment variable access and .env file loading.
// It registers an "env" resolver into the variables package so that
// ${{env:KEY}} placeholders are resolved from the environment.
package cenv

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/variables"
)

func init() {
	variables.RegisterResolver("env", func(key string) (string, bool) {
		return os.LookupEnv(key)
	})
	// Workspace preflight (--workspace / --universe / KTW_WORKSPACE / fallback)
	// + .env loading are NOT auto-run here anymore — they need the app name,
	// which is owned by the application package. Application's Env()
	// initializer calls `cenv.Preflight(appName, os.Args[1:])` and
	// `cenv.LoadDotenv(appName, homeDir)` explicitly.
}

// ── .env loading ─────────────────────────────────────────────────────────

// EnvFileStatus tracks whether an .env candidate was found and loaded.
type EnvFileStatus struct {
	Path   string
	Loaded bool // true if file existed and was processed
	// Withheld are the keys the file holds that it did not set because they
	// are another scope's: a key of the global .env (or of an attached
	// project's working-folder .env) not declared global (GlobalKeysVar).
	Withheld []string
}

// envFileStatuses tracks all .env candidates checked during startup.
var envFileStatuses []EnvFileStatus

// EnvFileStatuses returns all .env candidates that were checked, with their load status.
func EnvFileStatuses() []EnvFileStatus { return envFileStatuses }

// LoadedEnvFiles returns only the .env files that were actually loaded.
func LoadedEnvFiles() []string {
	var result []string
	for _, s := range envFileStatuses {
		if s.Loaded {
			result = append(result, s.Path)
		}
	}
	return result
}

// recordEnvCandidate loads one .env candidate (once) and answers the
// variables it set.
func recordEnvCandidate(path string) []string {
	return recordEnvCandidateOf(path, nil)
}

// recordEnvCandidateOf is recordEnvCandidate setting only the keys allow
// accepts (every key when allow is nil); the others are recorded withheld.
func recordEnvCandidateOf(path string, allow func(string) bool) []string {
	// De-duplicate: Preflight may load the global + CWD .env early to drive
	// universe selection; loadDotenv re-walks the same paths. Skip duplicates
	// so the diagnostic list shows each candidate exactly once.
	for _, s := range envFileStatuses {
		if s.Path == path {
			return nil
		}
	}
	if _, err := os.Stat(path); err == nil {
		set, withheld, _ := loadEnvFiltered(path, allow)
		envFileStatuses = append(envFileStatuses, EnvFileStatus{Path: path, Loaded: true, Withheld: withheld})
		return set
	}
	envFileStatuses = append(envFileStatuses, EnvFileStatus{Path: path, Loaded: false})
	return nil
}

// DotenvKeysVar names the variable that lists the keys a process took from a
// universe's .env or the global .env (its own and those of the process that
// started it). A child that reads its keys itself is started without them
// (ChildEnviron): it reads the files as they are now, so a key saved after
// its parent started reaches it.
const DotenvKeysVar = "KTW_DOTENV_KEYS"

// markDotenvKeys adds keys to DotenvKeysVar.
func markDotenvKeys(keys []string) {
	if len(keys) == 0 {
		return
	}
	seen := map[string]bool{}
	var all []string
	for _, k := range append(dotenvKeys(), keys...) {
		if k != "" && !seen[k] {
			seen[k] = true
			all = append(all, k)
		}
	}
	_ = os.Setenv(DotenvKeysVar, strings.Join(all, ","))
}

// dotenvKeys are the keys DotenvKeysVar lists.
func dotenvKeys() []string {
	var out []string
	for _, k := range strings.Split(os.Getenv(DotenvKeysVar), ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// ChildEnviron is base (os.Environ() form) without the keys DotenvKeysVar
// lists, nor the list itself: the environment of a child that loads its
// universe's .env and the global .env itself (a member a hub starts), so it
// takes each key's current value from the files, never a copy its parent
// read when it started.
func ChildEnviron(base []string) []string {
	drop := map[string]bool{envKeyName(DotenvKeysVar): true}
	for _, k := range dotenvKeys() {
		drop[envKeyName(k)] = true
	}
	out := make([]string, 0, len(base))
	for _, kv := range base {
		name, _, _ := strings.Cut(kv, "=")
		if drop[envKeyName(name)] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// envKeyName is a variable's name as the platform compares it (Windows
// ignores case).
func envKeyName(k string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(k)
	}
	return k
}

// LoadDotenv loads .env files into the process environment per the 5-step
// chain stated in modules/framework/environment.md §2.
//
// Loading order (first-writer-wins — already-set vars are never overwritten):
//
//  1. Process env (highest)                           — already in os.Environ()
//  2. CWD/.env (early-pass, only when no workspace was selected otherwise)
//  3. <homeDir>/.env                                   (the universe's keys)
//  4. <GlobalAgentsRoot>/.env                          (cross-universe keys)
//  5. CWD/.env (full-pass)                             (project-local)
//
// The universe's .env comes BEFORE the global one: a key set in both scopes
// takes the universe's value. The global .env fills only the keys declared
// global (GlobalKeysVar): a universe never takes another scope's connector
// credential. Preflight reads only the selection keys (KTW_WORKSPACE /
// KTW_UNIVERSE) from the global .env, so it cannot make a global key win
// early. Callers (application package) pass the appName so the global .env
// path resolves correctly.
func LoadDotenv(appName string, homeDir string) {
	loadDotenvFor(appName, homeDir, CurrentInstallation)
}

// loadDotenvFor is LoadDotenv with the process's installation asked of inst
// at step 5.
func loadDotenvFor(appName, homeDir string, inst func() *Installation) {
	// Step 3: <homeDir>/.env (the universe).
	scoped := recordEnvCandidate(filepath.Join(homeDir, ".env"))

	// Step 4: <GlobalAgentsRoot>/.env (global) - only its declared keys.
	if root, err := globalAgentsRoot(appName); err == nil {
		global := filepath.Join(root, ".env")
		// The declaration first: it says which of the file's keys fill.
		declared, _ := loadEnv(global, map[string]bool{GlobalKeysVar: true})
		scoped = append(scoped, declared...)
		scoped = append(scoped, recordEnvCandidateOf(global, isGlobalKey)...)
	}
	// The keys of the two scoped files are listed: a child that reads them
	// itself is started without these copies (ChildEnviron).
	markDotenvKeys(scoped)

	// Step 5: CWD/.env (project-local, full-pass) - for an installation's
	// process, only in its own folder or a project its universe serves; an
	// attached project's folder is outside the universe, so its .env fills
	// only the keys declared global, never a connector credential.
	if cwd, err := os.Getwd(); err == nil {
		switch cwdEnvScope(inst(), cwd, homeDir) {
		case cwdEnvFull:
			recordEnvCandidate(filepath.Join(cwd, ".env"))
		case cwdEnvGlobalOnly:
			recordEnvCandidateOf(filepath.Join(cwd, ".env"), isGlobalKey)
		}
	}
}

// GlobalKeysVar names the variable that declares which keys the global .env
// gives every universe: a comma-separated list of names, an entry ending in
// "*" standing for every name it begins (ANTHROPIC_API_KEY,OPENAI_*). It is
// read from the process, else from the global .env itself. A key of the
// global .env it does not declare is another scope's - a connector
// credential of the account the global file was written for - and never
// fills a universe's missing key.
const GlobalKeysVar = "KTW_GLOBAL_KEYS"

// alwaysGlobalKeys are global without a declaration: the universe selection
// preflight reads from the global .env, and the declaration itself.
var alwaysGlobalKeys = []string{"KTW_WORKSPACE", "KTW_UNIVERSE", GlobalKeysVar}

// ReservedKeyPrefix begins the names of KaTyoti's own variables: the
// declaration (GlobalKeysVar), the universe selection (alwaysGlobalKeys),
// DotenvKeysVar and every KTW_* a process reads to find its hub, data,
// binaries or logs. None is a key a key set may import: written into a .env,
// it would change where a process works, or (GlobalKeysVar itself) declare
// every key of the global.env global.
const ReservedKeyPrefix = "KTW_"

// ReservedKey says whether name is one of KaTyoti's own variables
// (ReservedKeyPrefix, in any case), never a key a key set imports.
func ReservedKey(name string) bool {
	return strings.HasPrefix(strings.ToUpper(name), ReservedKeyPrefix)
}

// isGlobalKey says whether key is declared global (GlobalKeysVar).
func isGlobalKey(key string) bool {
	name := envKeyName(key)
	for _, k := range alwaysGlobalKeys {
		if envKeyName(k) == name {
			return true
		}
	}
	return GlobalKeyDeclared(os.Getenv(GlobalKeysVar), key)
}

// GlobalKeyDeclared says whether key is named by decl, a GlobalKeysVar value
// (comma-separated names, an entry ending in "*" standing for every name it
// begins) - the one grammar of the declaration, for its reader and its writers.
func GlobalKeyDeclared(decl, key string) bool {
	name := envKeyName(key)
	for _, entry := range strings.Split(decl, ",") {
		entry = envKeyName(strings.TrimSpace(entry))
		switch {
		case entry == "":
		case strings.HasSuffix(entry, "*"):
			if strings.HasPrefix(name, strings.TrimSuffix(entry, "*")) {
				return true
			}
		case entry == name:
			return true
		}
	}
	return false
}

// cwdEnv says how much of the current folder's .env may be read.
type cwdEnv int

const (
	cwdEnvNone       cwdEnv = iota // not read
	cwdEnvFull                     // every key
	cwdEnvGlobalOnly               // only the keys declared global
)

// cwdEnvAllowed says whether the current folder's .env may be read: always
// without an installation; with one, only inside the installation's folder or
// a project attached to the universe whose config root is homeDir - another
// folder's .env would hand it another installation's keys or workspace.
func cwdEnvAllowed(cwd, homeDir string) bool {
	return cwdEnvAllowedFor(CurrentInstallation(), cwd, homeDir)
}

func cwdEnvAllowedFor(inst *Installation, cwd, homeDir string) bool {
	return cwdEnvScope(inst, cwd, homeDir) != cwdEnvNone
}

// cwdEnvScope is cwdEnvAllowedFor saying how much: every key without an
// installation or inside the universe's own folder; only the keys declared
// global inside the installation's folder (a scope above every universe: in
// place, its folder is the config root and its .env the global one) or in a
// project attached to the universe (a folder outside its scope); none
// elsewhere.
func cwdEnvScope(inst *Installation, cwd, homeDir string) cwdEnv {
	if inst == nil {
		return cwdEnvFull
	}
	if homeDir != "" && within(absClean(homeDir), cwd) {
		return cwdEnvFull
	}
	if within(inst.Dir(), cwd) {
		return cwdEnvGlobalOnly
	}
	for _, p := range AttachedProjects(homeDir) {
		if within(absClean(p), cwd) {
			return cwdEnvGlobalOnly
		}
	}
	return cwdEnvNone
}

// within says whether path is dir or under it.
func within(dir, path string) bool {
	if dir == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// loadEnvKeys sets, from a .env file, only the named variables — preflight's
// read of the global .env, which picks the universe without loading every
// global key ahead of the universe's own.
func loadEnvKeys(path string, keys ...string) {
	only := map[string]bool{}
	for _, k := range keys {
		only[k] = true
	}
	_, _ = loadEnv(path, only)
}

// loadEnvFile reads a .env file and sets variables that are not already set.
// Returns nil silently if the file does not exist.
func loadEnvFile(path string) error {
	_, err := loadEnv(path, nil)
	return err
}

// loadEnv is loadEnvFile restricted, when only is given, to those keys; it
// answers the variables it set.
func loadEnv(path string, only map[string]bool) ([]string, error) {
	var allow func(string) bool
	if only != nil {
		allow = func(k string) bool { return only[k] }
	}
	set, _, err := loadEnvFiltered(path, allow)
	return set, err
}

// loadEnvFiltered is loadEnv setting only the keys allow accepts (every key
// when allow is nil); it answers the variables it set and, apart, the keys
// allow refused.
func loadEnvFiltered(path string, allow func(string) bool) (set, withheld []string, err error) {
	entries, err := parseEnvFile(path)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		key := e[0]
		if allow != nil && !allow(key) {
			withheld = append(withheld, key)
			continue
		}
		// Expand ${{domain:key}} placeholders in values.
		val := variables.ExpandVars(e[1])
		// Only set if not already defined (env takes precedence over .env).
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
			set = append(set, key)
		}
	}
	return set, withheld, nil
}

// EnvFileValues is the key/value pairs of the .env file at path (values
// unquoted, not expanded); a missing or unreadable file has none. It sets
// nothing in the process: a caller that must mask the values (the scenario
// evidence copy) reads them without loading them.
func EnvFileValues(path string) map[string]string {
	pairs, _ := parseEnvFile(path)
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]string, len(pairs))
	for _, kv := range pairs {
		out[kv[0]] = kv[1]
	}
	return out
}

// parseEnvFile reads a .env file's key/value pairs in order (values trimmed
// and unquoted, not expanded); a missing file has none.
func parseEnvFile(path string) ([][2]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // missing .env is not an error
		}
		return nil, fmt.Errorf("cenv: read %s: %w", path, err)
	}
	var out [][2]string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out = append(out, [2]string{strings.TrimSpace(key), stripQuotes(strings.TrimSpace(val))})
	}
	return out, nil
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// ── Path helpers ────────────────────────────────────────────────────────

// ResolveDir returns the configured path, the fallback if empty, or resolves
// relative paths against baseDir. Absolute paths are returned as-is.
func ResolveDir(baseDir, configured, fallback string) string {
	if configured == "" {
		return fallback
	}
	if filepath.IsAbs(configured) {
		return filepath.Clean(configured)
	}
	return filepath.Join(baseDir, configured)
}

// ResolveWorkspaceDir resolves the workspace folder. Source: KTW_WORKSPACE
// env var. Fallback when unset: `<XDG_CONFIG>/<appName>` (the host-context
// fallback used by `ktw-univers list` and similar utilities that don't
// require a real workspace).
//
// `appName` is owned by the application package and passed in — cenv does
// not embed any "katyoti" literal.
func ResolveWorkspaceDir(appName string) string {
	if v := os.Getenv("KTW_WORKSPACE"); v != "" {
		return filepath.Clean(v)
	}
	return KeepTestsOut(filepath.Join(xdgConfigHome(), appName))
}

// ResolveDataDir resolves the data root: KTW_DATA → XDG_DATA_HOME/<appName>
// (an installation's katyoti.yml sets XDG_DATA_HOME, installation.go). A test
// process never resolves it into the person's data root (KeepTestsOut).
func ResolveDataDir(appName string) string {
	CurrentInstallation()
	if v := os.Getenv("KTW_DATA"); v != "" {
		return KeepTestsOut(filepath.Clean(v))
	}
	return DataRoot(appName)
}

// DataRoot is the installation's data root, XDG_DATA_HOME/<appName>, without
// the KTW_DATA override: KTW_DATA names one universe's data folder, so a
// process resolving ANOTHER universe's folder must not take its own.
func DataRoot(appName string) string {
	CurrentInstallation()
	return KeepTestsOut(filepath.Join(xdgDataHome(), appName))
}

// ResolveCacheDir resolves the cache root: KTW_CACHE → XDG_CACHE_HOME/<appName>
// (an installation's katyoti.yml sets KTW_CACHE).
func ResolveCacheDir(appName string) string {
	CurrentInstallation()
	if v := os.Getenv("KTW_CACHE"); v != "" {
		return filepath.Clean(v)
	}
	return filepath.Join(xdgCacheHome(), appName)
}

// xdgConfigCandidates returns all config directories to check for .env files.
// On Windows, includes both %APPDATA% and ~/.config for cross-platform compat.
func xdgConfigCandidates() []string {
	seen := map[string]bool{}
	var result []string
	add := func(dir string) {
		if dir != "" && !seen[dir] {
			seen[dir] = true
			result = append(result, dir)
		}
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		add(v)
	}
	if runtime.GOOS == "windows" {
		if v := os.Getenv("APPDATA"); v != "" {
			add(v)
		}
	}
	home, _ := os.UserHomeDir()
	add(filepath.Join(home, ".config"))
	return result
}

// xdgConfigHome returns XDG_CONFIG_HOME or the platform default.
func xdgConfigHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		if v := os.Getenv("APPDATA"); v != "" {
			return v
		}
	}
	return filepath.Join(home, ".config")
}

// xdgDataHome returns XDG_DATA_HOME or the platform default.
func xdgDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return v
		}
	}
	return filepath.Join(home, ".local", "share")
}

// xdgCacheHome returns XDG_CACHE_HOME or the platform default.
func xdgCacheHome() string {
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "cache")
		}
	}
	return filepath.Join(home, ".cache")
}

// ── Typed getters ────────────────────────────────────────────────────────

// GetString returns the value of the named env var.
// If the var is not set:
//   - if a default is provided, returns that default
//   - otherwise returns an error
func GetString(key string, defaultVal ...string) (string, error) {
	if val, ok := os.LookupEnv(key); ok {
		return val, nil
	}
	if len(defaultVal) > 0 {
		return defaultVal[0], nil
	}
	return "", fmt.Errorf("cenv: required env var %s is not set", key)
}

// MustGetString is like GetString but panics on error.
func MustGetString(key string, defaultVal ...string) string {
	v, err := GetString(key, defaultVal...)
	if err != nil {
		panic(err)
	}
	return v
}

// GetBool returns the boolean value of the named env var.
func GetBool(key string, defaultVal ...bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		if len(defaultVal) > 0 {
			return defaultVal[0], nil
		}
		return false, fmt.Errorf("cenv: required env var %s is not set", key)
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("cenv: env var %s: %w", key, err)
	}
	return b, nil
}

// GetInt returns the integer value of the named env var.
func GetInt(key string, defaultVal ...int) (int, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		if len(defaultVal) > 0 {
			return defaultVal[0], nil
		}
		return 0, fmt.Errorf("cenv: required env var %s is not set", key)
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("cenv: env var %s: %w", key, err)
	}
	return n, nil
}

// GetFloat returns the float64 value of the named env var.
func GetFloat(key string, defaultVal ...float64) (float64, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		if len(defaultVal) > 0 {
			return defaultVal[0], nil
		}
		return 0, fmt.Errorf("cenv: required env var %s is not set", key)
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("cenv: env var %s: %w", key, err)
	}
	return f, nil
}

// GetPath returns the path value of the named env var.
// Absolute paths are returned as-is. Relative paths are resolved against
// baseDir if provided, or against the current working directory otherwise.
// An optional default value can be passed after baseDir.
func GetPath(key string, baseDir string, defaultVal ...string) (string, error) {
	raw, err := GetString(key, defaultVal...)
	if err != nil {
		return "", err
	}
	if raw == "" {
		return "", nil
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw), nil
	}
	if baseDir != "" {
		return filepath.Join(baseDir, raw), nil
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return raw, nil
	}
	return abs, nil
}

// GetList returns a comma-separated env var as a []string.
func GetList(key string, defaultVal ...[]string) ([]string, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		if len(defaultVal) > 0 {
			return defaultVal[0], nil
		}
		return nil, fmt.Errorf("cenv: required env var %s is not set", key)
	}
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts, nil
}
