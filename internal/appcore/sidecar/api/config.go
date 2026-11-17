// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// SidecarConfigService provides the sidecar config lifecycle.
// Every sidecar publishes this service via SidecarExt(efs).
// CLI verbs are grouped under `<sidecar> sc config <verb>` — exactly 5
// verbs per the command-surface decision: `check`, `get`, `set`
// `validate`, `setup`.
//
// History:
//   - `Describe` was removed 2026-05-10 — its data moved to
//     `ResourceService.List(type=config-file|config-program|env)`.
//   - `GetDefault` was folded into `Get` (flag `Default bool`) on
//     2026-05-23 per the ms-sidecar-norm verb cleanup.
//   - `Materialize` was renamed to `Set` on 2026-05-23.
//   - `Do` was renamed to `Setup` on 2026-05-23.
type SidecarConfigService interface {
	// Get reads a config file. With Default=false (the typical case),
	// reads the live on-disk file. With Default=true, reads the bundled
	// default from the sidecar's embed.FS — useful for "show defaults",
	// "reset to factory", "diff live vs default".
	Get(ctx context.Context, in ConfigGetInput) (ConfigGetOutput, error)

	// Validate checks a proposed config blob without writing to disk.
	// Useful for pre-flight checks, installer UI validation, CI checks.
	Validate(ctx context.Context, in ConfigValidateInput) (ConfigValidateOutput, error)

	// Set writes a proposed config to disk. Non-interactive, idempotent.
	// The machine-callable path for installers and scripts.
	Set(ctx context.Context, in ConfigSetInput) (ConfigSetOutput, error)

	// Check produces a machine-readable status report of config/tool/env state.
	// The canonical health-check entry — subsumes the old standalone
	// `check` and `program check` verbs.
	Check(ctx context.Context) (ConfigCheckReport, error)

	// Setup runs the interactive setup: prompts for missing values, then
	// writes. Internally: Check() → prompt gaps → Validate() → Set().
	Setup(ctx context.Context, in ConfigSetupInput) (ConfigSetupOutput, error)
}

// SidecarConfigServiceDef is the service definition for SidecarConfigService.
var SidecarConfigServiceDef = meta.ServiceDef{
	Name:    "config",
	Short:   "Sidecar configuration lifecycle",
	Long:    "Manages sidecar configuration: check status, setup, read/validate/write config files.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Get":      meta.Method("Read a config file (live by default, embedded default with --default)"),
		"Validate": meta.Method("Validate a proposed config without writing"),
		"Set":      meta.Method("Write config to disk: materialize bundled defaults, or (Name+Content) write one file whole after validating it"),
		"Check":    meta.Method("Report sidecar status (deps, env, config files) with actionable hints"),
		"Setup":    meta.Method("Interactive setup (prompts + writes)"),
	},
}

// --- I/O types ---

// ConfigDescribeOutput is the full shape of the sidecar's config needs.
type ConfigDescribeOutput struct {
	ConfigFiles []ConfigFileDescriptor `json:"config_files"`
	Tools       []ToolDescriptor       `json:"tools"`
	APIKeys     []APIKeyDescriptor     `json:"api_keys"`
}

// ConfigFileDescriptor describes a config file the sidecar manages.
type ConfigFileDescriptor struct {
	Name       string `json:"name"        description:"File name (e.g. DocService.yaml)"`
	Path       string `json:"path"        description:"Resolved path on disk"`
	Exists     bool   `json:"exists"      description:"Whether the file currently exists"`
	HasDefault bool   `json:"has_default" description:"Whether a bundled default is available"`
}

// ToolDescriptor describes an external program dependency: its declaration
// (required or optional, the install skill, the structured install recipes per
// platform) and what the check found.
type ToolDescriptor struct {
	Name     string   `json:"name"`
	Commands []string `json:"commands,omitempty"`
	// PythonPackage is the Python package the program is: checked as that
	// package, never as a command on PATH (publish.ToolDef.PythonPackage).
	PythonPackage   string                  `json:"python_package,omitempty"`
	Required        bool                    `json:"required"`
	RequiredVersion string                  `json:"required_version,omitempty"`
	FoundVersion    string                  `json:"found_version,omitempty"`
	OK              bool                    `json:"ok"`
	InstallSkill    string                  `json:"install_skill,omitempty"`
	Recipes         []publish.InstallRecipe `json:"recipes,omitempty"`
}

// NewToolDescriptor is a tool's declaration as a descriptor, before any check.
func NewToolDescriptor(t publish.ToolDef) ToolDescriptor {
	return ToolDescriptor{
		Name:            t.Name,
		Commands:        t.Commands,
		PythonPackage:   t.PythonPackage,
		Required:        !t.Optional,
		RequiredVersion: t.Version,
		InstallSkill:    t.InstallSkill,
		Recipes:         t.Recipes,
	}
}

// APIKeyDescriptor describes a required or optional env var.
type APIKeyDescriptor struct {
	EnvName     string `json:"env_name"`
	Label       string `json:"label"`
	Required    bool   `json:"required"`
	Set         bool   `json:"set"`
	Description string `json:"description"`
}

// AuthDescriptor describes a login flow the sidecar owns (category "auth").
type AuthDescriptor struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	LoginHint   string `json:"login_hint,omitempty"`
	OK          bool   `json:"ok"`    // authenticated (only meaningful when Known)
	Known       bool   `json:"known"` // false = auth state could not be determined
}

// ConfigGetInput specifies which config file to read.
// When Default=true, returns the bundled default from the sidecar's
// embed.FS; when false (the typical case) returns the live on-disk
// file. Folded together so callers + CLI users don't juggle two
// almost-identical verbs.
type ConfigGetInput struct {
	Name    string `json:"name"              cli:"arg"     description:"Config file name (e.g. DocService.yaml)"`
	Default bool   `json:"default,omitempty" cli:"default" description:"Read the bundled embedded default instead of the on-disk file"`
}

// ConfigGetOutput contains the resolved config file content.
// Path is empty when Default=true was requested (no on-disk file).
type ConfigGetOutput struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Content string `json:"content"`
}

// ConfigValidateInput contains a proposed config to validate.
type ConfigValidateInput struct {
	Name    string `json:"name"    cli:"arg"  description:"Config file name"`
	Content string `json:"content"            description:"Proposed config content"`
}

// ConfigValidateOutput is the validation result.
type ConfigValidateOutput struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

// ConfigSetInput specifies a config-write operation. Two paths:
//
//   - Name == "": materialize every bundled default that is missing
//     (idempotent; Reset recreates them first).
//   - Name != "": write Content as the WHOLE file Name (raw write). The
//     content is validated first (see Validate) and nothing is written when
//     it is invalid. This is the path a text editor takes; a schema-driven
//     form will produce the same call once it exists
//     (sidecar-config-raw-edit → sidecar-config-schema-and-form).
//
// Per-key writes via Key / Value remain reserved (placeholders today —
// strategies will validate against the parsed schema before writing).
type ConfigSetInput struct {
	Reset   bool   `json:"reset,omitempty"   cli:"reset"   description:"Delete and recreate config files before writing"`
	Name    string `json:"name,omitempty"    cli:"name"    description:"Config file to write as a whole (raw write path, with Content)"`
	Content string `json:"content,omitempty" cli:"content" description:"Proposed full content for Name; validated before writing"`
	Key     string `json:"key,omitempty"     cli:"key"     description:"(RFU) Specific key path to write (e.g. 'auth.client_id')"`
	Value   string `json:"value,omitempty"   cli:"value"   description:"(RFU) Value to write at the given key"`
}

// ConfigSetOutput is the result of a Set call.
type ConfigSetOutput struct {
	HomeDir string           `json:"home_dir"`
	Dirs    []DirEntry       `json:"dirs,omitempty"`
	Files   []FileEntry      `json:"files,omitempty"`
	Tools   []ToolDescriptor `json:"tools,omitempty"`
}

// DirEntry describes a directory created during setup.
type DirEntry struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

// FileEntry describes a config file materialized during setup, or written
// by the raw-write path of Set.
type FileEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"` // resolved on-disk path (raw-write path)
	Status string `json:"status"`         // "created", "written", "skip", "error"
	Error  string `json:"error,omitempty"`
}

// ConfigCheckReport is the machine-readable status report.
//
// Two-tier surface:
//   - Legacy per-category fields (Deps / EnvKeys / ConfigFiles) — kept
//     so existing renderers don't break.
//   - Unified Items field + Healthy + Summary (NEW, 2026-05-23 per
//     the command-surface decision) — the canonical surface for new
//     consumers. Items carries actionable Hint per failure.
//
// New consumers SHOULD read Items + Healthy + Summary. The per-category
// fields remain as a redundant projection for ergonomic per-category
// rendering; they will be removed in a follow-up cleanup once all
// renderers migrate.
type ConfigCheckReport struct {
	// === Unified surface (canonical — read this) ===
	Healthy bool        `json:"healthy"`
	Summary string      `json:"summary"`
	Items   []CheckItem `json:"items"`

	// === Per-category projection (legacy, populated for ergonomics) ===
	Deps        []ToolDescriptor       `json:"deps"`
	EnvKeys     []APIKeyDescriptor     `json:"env_keys"`
	ConfigFiles []ConfigFileDescriptor `json:"config_files"`
	Auth        []AuthDescriptor       `json:"auth,omitempty"`
}

// CheckItem is one entry in the unified ConfigCheckReport.Items list.
// Categories: "config" (config file), "env" (environment variable),
// "program" (external program), "auth" (login flow), "schema" (config
// schema validation), "permissions" (directory/file access).
//
// `Hint` is the actionable next-step string operators run when status
// != "ok" (e.g. "Run `sc config setup`", "winget install pandoc",
// "Authenticate: `ktw-example auth login`").
type CheckItem struct {
	Category string `json:"category"`           // "config" | "env" | "program" | "auth" | "schema" | "permissions"
	Name     string `json:"name"`               // file/env/program name
	Status   string `json:"status"`             // "ok" | "missing" | "outdated" | "invalid" | "warning"
	Detail   string `json:"detail,omitempty"`   // what we found
	Hint     string `json:"hint,omitempty"`     // actionable next step
	Severity string `json:"severity,omitempty"` // "info" | "warn" | "error"
}

// ConfigSetupInput specifies options for interactive setup.
type ConfigSetupInput struct {
	Reset bool `json:"reset,omitempty" cli:"reset" description:"Delete and recreate config files"`
}

// ConfigSetupOutput is the result of interactive setup.
type ConfigSetupOutput struct {
	HomeDir string           `json:"home_dir"`
	Dirs    []DirEntry       `json:"dirs,omitempty"`
	Files   []FileEntry      `json:"files,omitempty"`
	Tools   []ToolDescriptor `json:"tools,omitempty"`
	APIKeys []APIKeyEntry    `json:"api_keys,omitempty"`
}

// APIKeyEntry describes an API key configured during setup.
type APIKeyEntry struct {
	EnvName string `json:"env_name"`
	Label   string `json:"label"`
	Status  string `json:"status"` // "set", "configured", "skipped"
}
