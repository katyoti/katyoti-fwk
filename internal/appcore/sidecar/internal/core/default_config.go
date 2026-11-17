// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// ConfigDeps holds the aggregated infrastructure needs captured at init time.
// Populated by SidecarExt from PublishedServicesPoint.
type ConfigDeps struct {
	AppName     string
	EmbeddedFS  fs.FS               // the sidecar's embed.FS for reading defaults
	ConfigFiles []string            // from WithConfigFiles
	ConfigDirs  []string            // from WithConfigDirs
	Tools       []publish.ToolDef   // from WithTools
	APIKeys     []publish.APIKeyDef // from WithAPIKeys
	Auth        []publish.AuthDef   // from WithAuth
}

// AggregateConfigDeps reads all PublishedService entries from the EM and
// aggregates their infrastructure needs into a single ConfigDeps.
func AggregateConfigDeps(app *application.Application, appName string, efs fs.FS) ConfigDeps {
	deps := ConfigDeps{
		AppName:    appName,
		EmbeddedFS: efs,
	}
	for _, ps := range application.UseAppExtensions[publish.PublishedService](app) {
		deps.ConfigFiles = append(deps.ConfigFiles, ps.ConfigFiles...)
		deps.ConfigDirs = append(deps.ConfigDirs, ps.ConfigDirs...)
		deps.Tools = append(deps.Tools, ps.Tools...)
		deps.APIKeys = append(deps.APIKeys, ps.APIKeys...)
		deps.Auth = append(deps.Auth, ps.Auth...)
	}
	return deps
}

// DefaultConfigService is the generic ConfigService implementation.
// It operates on aggregated infrastructure data captured at construction time.
// All methods are non-interactive — the Cobra command layer handles prompts.
type DefaultConfigService struct {
	Deps ConfigDeps
}

// NewDefaultConfigService creates a DefaultConfigService.
func NewDefaultConfigService(deps ConfigDeps) *DefaultConfigService {
	return &DefaultConfigService{Deps: deps}
}

// Get reads a config file. With in.Default=false (the typical path)
// reads the live on-disk copy. With in.Default=true reads the bundled
// embedded default from the sidecar's embed.FS.
func (s *DefaultConfigService) Get(ctx context.Context, in api.ConfigGetInput) (api.ConfigGetOutput, error) {
	if in.Default {
		if s.Deps.EmbeddedFS == nil {
			return api.ConfigGetOutput{}, fmt.Errorf("no embedded filesystem available")
		}
		data, err := fs.ReadFile(s.Deps.EmbeddedFS, "embed/config/"+in.Name)
		if err != nil {
			return api.ConfigGetOutput{}, fmt.Errorf("default config %q not found: %w", in.Name, err)
		}
		return api.ConfigGetOutput{Name: in.Name, Content: string(data)}, nil
	}

	configDir := application.Env().ConfigDir()
	path := filepath.Join(configDir, in.Name)
	data, err := os.ReadFile(path)
	if err != nil {
		return api.ConfigGetOutput{}, fmt.Errorf("config %q: %w", in.Name, err)
	}
	return api.ConfigGetOutput{Name: in.Name, Path: path, Content: string(data)}, nil
}

// Validate checks proposed config content: non-empty, and syntactically
// valid YAML with a mapping at the top level (every config file is a
// mapping — a scalar or a list would silently decode to nothing).
//
// Schema validation is NOT done here: the shape lives in the contributed
// data-type (config_resource_provider.go) and is the schema-driven form's
// job (sidecar-config-schema-and-form).
func (s *DefaultConfigService) Validate(ctx context.Context, in api.ConfigValidateInput) (api.ConfigValidateOutput, error) {
	if strings.TrimSpace(in.Content) == "" {
		return api.ConfigValidateOutput{
			Valid:  false,
			Errors: []string{"content is empty"},
		}, nil
	}
	if _, err := marshal.UnmarshalMap([]byte(in.Content), marshal.YAML); err != nil {
		return api.ConfigValidateOutput{
			Valid:  false,
			Errors: []string{"invalid YAML: " + err.Error()},
		}, nil
	}
	return api.ConfigValidateOutput{Valid: true}, nil
}

// Set writes config to disk. Non-interactive, idempotent.
//
// With in.Name set, it is the raw-write path: Content replaces the whole
// file in.Name under the config dir, after Validate accepts it. Otherwise
// it materializes the bundled defaults that are missing. Per-key writes
// via in.Key / in.Value are reserved for future work.
func (s *DefaultConfigService) Set(ctx context.Context, in api.ConfigSetInput) (api.ConfigSetOutput, error) {
	if in.Key != "" {
		return api.ConfigSetOutput{}, fmt.Errorf("per-key writes (--key %q) not yet implemented; current Set only materializes bundled defaults", in.Key)
	}

	homeDir := application.Env().HomeDir()
	configDir := application.Env().ConfigDir()

	out := api.ConfigSetOutput{HomeDir: homeDir}

	if in.Name != "" {
		return s.setRaw(ctx, out, configDir, in)
	}

	// Create directories
	for _, dir := range s.Deps.ConfigDirs {
		full := filepath.Join(homeDir, dir)
		created := false
		if _, err := os.Stat(full); os.IsNotExist(err) {
			created = true
		}
		if err := os.MkdirAll(full, 0o755); err != nil {
			return out, fmt.Errorf("create directory %s: %w", full, err)
		}
		out.Dirs = append(out.Dirs, api.DirEntry{Path: full, Created: created})
	}

	// Copy default config files
	for _, name := range s.Deps.ConfigFiles {
		dst := filepath.Join(configDir, name)

		// Reset mode: remove existing file first
		if in.Reset {
			_ = os.Remove(dst)
		}

		// Skip if already exists (idempotent)
		if _, err := os.Stat(dst); err == nil {
			out.Files = append(out.Files, api.FileEntry{Name: name, Status: "skip"})
			continue
		}

		// Read from embedded defaults
		if s.Deps.EmbeddedFS == nil {
			out.Files = append(out.Files, api.FileEntry{Name: name, Status: "error", Error: "no embedded FS"})
			continue
		}
		data, err := fs.ReadFile(s.Deps.EmbeddedFS, "embed/config/"+name)
		if err != nil {
			out.Files = append(out.Files, api.FileEntry{Name: name, Status: "error", Error: err.Error()})
			continue
		}

		// Ensure parent dir exists
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			out.Files = append(out.Files, api.FileEntry{Name: name, Status: "error", Error: err.Error()})
			continue
		}

		if err := os.WriteFile(dst, data, 0o644); err != nil {
			out.Files = append(out.Files, api.FileEntry{Name: name, Status: "error", Error: err.Error()})
			continue
		}
		out.Files = append(out.Files, api.FileEntry{Name: name, Status: "created"})
	}

	// Check tools (report only, no install)
	for _, t := range s.Deps.Tools {
		td := api.NewToolDescriptor(t)
		if dep := publish.CheckTool(t, ctx); dep != nil {
			td.OK = dep.OK
			td.FoundVersion = dep.FoundVersion
			td.RequiredVersion = dep.RequiredVersion
		}
		out.Tools = append(out.Tools, td)
	}

	return out, nil
}

// setRaw is Set's raw-write path: validate in.Content, then replace the
// whole file in.Name under configDir atomically (temp file + rename, so a
// sidecar re-reading its config never sees a half-written file).
//
// in.Name is confined to the config dir: a bare file name or a relative
// path inside it. Anything absolute or climbing out is refused.
func (s *DefaultConfigService) setRaw(ctx context.Context, out api.ConfigSetOutput, configDir string, in api.ConfigSetInput) (api.ConfigSetOutput, error) {
	rel := filepath.Clean(in.Name)
	if filepath.IsAbs(rel) || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return out, fmt.Errorf("config name %q must be a file name inside the config dir", in.Name)
	}
	v, err := s.Validate(ctx, api.ConfigValidateInput{Name: in.Name, Content: in.Content})
	if err != nil {
		return out, err
	}
	if !v.Valid {
		return out, fmt.Errorf("config %q rejected: %s", in.Name, strings.Join(v.Errors, "; "))
	}

	dst := filepath.Join(configDir, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return out, fmt.Errorf("create config dir for %s: %w", in.Name, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".*.tmp")
	if err != nil {
		return out, fmt.Errorf("stage config %s: %w", in.Name, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(in.Content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return out, fmt.Errorf("write config %s: %w", in.Name, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return out, fmt.Errorf("write config %s: %w", in.Name, err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return out, fmt.Errorf("write config %s: %w", in.Name, err)
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		_ = os.Remove(tmpPath)
		return out, fmt.Errorf("write config %s: %w", in.Name, err)
	}
	out.Files = append(out.Files, api.FileEntry{Name: in.Name, Path: dst, Status: "written"})
	return out, nil
}

// Check produces a machine-readable status report.
func (s *DefaultConfigService) Check(ctx context.Context) (api.ConfigCheckReport, error) {
	return s.CheckIn(ctx, application.Env().ConfigDir(), application.Env().HomeDir())
}

// CheckIn is Check against a universe's config dir and home (its .env): the
// check of a member of another universe than this process's, read in process.
func (s *DefaultConfigService) CheckIn(ctx context.Context, configDir, homeDir string) (api.ConfigCheckReport, error) {
	report := api.ConfigCheckReport{}

	// === Tools (Category: program) ===
	for _, t := range s.Deps.Tools {
		td := api.NewToolDescriptor(t)
		if dep := publish.CheckTool(t, ctx); dep != nil {
			td.OK = dep.OK
			td.FoundVersion = dep.FoundVersion
			if dep.RequiredVersion != "" {
				// A program not found reports no version: the declared one
				// stays, what an installer plans for.
				td.RequiredVersion = dep.RequiredVersion
			}
		}
		report.Deps = append(report.Deps, td)
		report.Items = append(report.Items, programCheckItem(t, td))
	}

	// === Env keys (Category: env) ===
	envFileVars := loadEnvFile(filepath.Join(homeDir, ".env"))
	for _, k := range s.Deps.APIKeys {
		set := os.Getenv(k.EnvName) != ""
		if !set {
			if v, ok := envFileVars[k.EnvName]; ok && v != "" {
				set = true
			}
		}
		report.EnvKeys = append(report.EnvKeys, api.APIKeyDescriptor{
			EnvName:     k.EnvName,
			Label:       k.Label,
			Required:    k.Required,
			Set:         set,
			Description: k.Description,
		})
		report.Items = append(report.Items, envCheckItem(k, set))
	}

	// === Config files (Category: config) ===
	for _, name := range s.Deps.ConfigFiles {
		path := filepath.Join(configDir, name)
		_, err := os.Stat(path)
		hasDefault := false
		if s.Deps.EmbeddedFS != nil {
			_, defErr := fs.Stat(s.Deps.EmbeddedFS, "embed/config/"+name)
			hasDefault = defErr == nil
		}
		desc := api.ConfigFileDescriptor{
			Name:       name,
			Path:       path,
			Exists:     err == nil,
			HasDefault: hasDefault,
		}
		report.ConfigFiles = append(report.ConfigFiles, desc)
		report.Items = append(report.Items, configFileCheckItem(desc))
	}

	// === Auth (Category: auth) ===
	for _, a := range s.Deps.Auth {
		ok, known := false, false
		if a.Check != nil {
			if got, err := a.Check(ctx); err == nil {
				ok, known = got, true
			}
		}
		report.Auth = append(report.Auth, api.AuthDescriptor{
			Name:        a.Name,
			Label:       a.Label,
			Description: a.Description,
			LoginHint:   a.LoginHint,
			OK:          ok,
			Known:       known,
		})
		report.Items = append(report.Items, authCheckItem(a, ok, known))
	}

	// Derive Healthy + Summary from Items.
	report.Healthy = true
	okCount, warnCount, errCount := 0, 0, 0
	for _, it := range report.Items {
		switch it.Severity {
		case "error":
			errCount++
			report.Healthy = false
		case "warn":
			warnCount++
		default:
			okCount++
		}
	}
	report.Summary = fmt.Sprintf("%d OK · %d WARN · %d ERROR", okCount, warnCount, errCount)

	return report, nil
}

// programCheckItem maps a publish.ToolDef + its CheckTool result into
// a CheckItem with an actionable Hint. Replaces the standalone
// `program check` command logic (deleted in this milestone).
func programCheckItem(t publish.ToolDef, td api.ToolDescriptor) api.CheckItem {
	item := api.CheckItem{
		Category: "program",
		Name:     t.Name,
	}
	if td.OK {
		item.Status = "ok"
		item.Severity = "info"
		if td.FoundVersion != "" {
			item.Detail = "found " + td.FoundVersion
		}
		return item
	}
	switch {
	case td.FoundVersion != "":
		// Found, but not the validated version: it runs, its output may not
		// be what the sidecar expects.
		item.Status = "outdated"
		item.Severity = "warn"
		item.Detail = "found " + td.FoundVersion + "; validated with " + td.RequiredVersion
	case td.RequiredVersion != "":
		item.Status = "missing"
		item.Severity = "error"
		item.Detail = "required " + td.RequiredVersion + "; not found in PATH"
	default:
		item.Status = "missing"
		item.Severity = "error"
		item.Detail = "not found in PATH"
	}
	// An optional program the sidecar runs without is a warning, never an
	// error: its absence leaves the sidecar healthy.
	if t.Optional && item.Severity == "error" {
		item.Severity = "warn"
		item.Detail += " (optional)"
	}
	// Prefer the InstallSkill URI as the actionable hint — it points
	// at a skill document the operator (or LLM) can fetch for install
	// instructions. Fallback: generic "install + add to PATH".
	if t.InstallSkill != "" {
		item.Hint = "Read install instructions: `sc skills get-instructions " + t.InstallSkill + "`"
	} else {
		item.Hint = "Install `" + t.Name + "` and ensure it's on PATH"
	}
	return item
}

// envCheckItem maps an API-key declaration into a CheckItem.
func envCheckItem(k publish.APIKeyDef, set bool) api.CheckItem {
	item := api.CheckItem{
		Category: "env",
		Name:     k.EnvName,
	}
	if set {
		item.Status = "ok"
		item.Severity = "info"
		return item
	}
	item.Status = "missing"
	if k.Required {
		item.Severity = "error"
	} else {
		item.Severity = "warn"
	}
	item.Detail = "environment variable " + k.EnvName + " is not set"
	if k.Label != "" {
		item.Detail += " (" + k.Label + ")"
	}
	item.Hint = "Set " + k.EnvName + " in .env or run `sc config setup`"
	return item
}

// authCheckItem maps an auth declaration into a CheckItem. The login flow is
// surfaced so a generic configurator routes to it (category "auth"). When the
// auth state can't be determined (no Check), it's reported as a warning that
// points at the login hint rather than asserting a missing login.
func authCheckItem(a publish.AuthDef, ok, known bool) api.CheckItem {
	item := api.CheckItem{Category: "auth", Name: a.Name}
	login := a.LoginHint
	if login == "" {
		login = "the sidecar's auth/login command"
	}
	switch {
	case known && ok:
		item.Status = "ok"
		item.Severity = "info"
		if a.Label != "" {
			item.Detail = a.Label + " authenticated"
		}
	case known && !ok:
		item.Status = "missing"
		item.Severity = "warn"
		item.Detail = "not authenticated"
		item.Hint = "Authenticate: `" + login + "`"
	default: // unknown
		item.Status = "warning"
		item.Severity = "warn"
		item.Detail = "auth state unknown"
		item.Hint = "Authenticate if needed: `" + login + "`"
	}
	return item
}

// configFileCheckItem maps a config-file descriptor into a CheckItem.
func configFileCheckItem(desc api.ConfigFileDescriptor) api.CheckItem {
	item := api.CheckItem{
		Category: "config",
		Name:     desc.Name,
	}
	if desc.Exists {
		item.Status = "ok"
		item.Severity = "info"
		return item
	}
	item.Status = "missing"
	item.Severity = "warn"
	item.Detail = "config file not present at " + desc.Path
	if desc.HasDefault {
		item.Hint = "Run `sc config set " + desc.Name + "` to write the default template"
	} else {
		item.Hint = "Create " + desc.Path + " manually or run `sc config setup`"
	}
	return item
}

// Do runs the setup flow. Non-interactive — the Cobra command layer handles prompts.
// APIKeys in the input are pre-provided values (collected by the CLI before calling).
// Setup runs the interactive sidecar setup: writes bundled defaults
// (via Set), then reports API-key status (writes are still done by
// the Cobra command layer where TTY prompts can run).
func (s *DefaultConfigService) Setup(ctx context.Context, in api.ConfigSetupInput) (api.ConfigSetupOutput, error) {
	setOut, err := s.Set(ctx, api.ConfigSetInput{Reset: in.Reset})
	if err != nil {
		return api.ConfigSetupOutput{}, err
	}

	out := api.ConfigSetupOutput{
		HomeDir: setOut.HomeDir,
		Dirs:    setOut.Dirs,
		Files:   setOut.Files,
		Tools:   setOut.Tools,
	}

	// Step 2: Report API key status (actual writing handled by Cobra command layer)
	for _, k := range s.Deps.APIKeys {
		set := os.Getenv(k.EnvName) != ""
		status := "skipped"
		if set {
			status = "configured"
		}
		out.APIKeys = append(out.APIKeys, api.APIKeyEntry{
			EnvName: k.EnvName,
			Label:   k.Label,
			Status:  status,
		})
	}

	return out, nil
}

// --- Env file helpers (absorbed from appcore/setup_env.go) ---

func loadEnvFile(path string) map[string]string {
	result := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			result[key] = val
		}
	}
	return result
}
