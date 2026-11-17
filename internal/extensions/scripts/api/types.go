// SPDX-License-Identifier: Apache-2.0

// Package api defines the public types for the scripts extension.
package api

import (
	"context"
	"io"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// ScriptService is the public interface for discovering and running scripts.
// Scripts are addressed by provider:name URIs.
type ScriptService interface {
	List(ctx context.Context) (ScriptListOutput, error)
	Run(ctx context.Context, in ScriptRunInput) (ScriptRunOutput, error)
}

// ScriptServiceDef describes the ScriptService interface for auto-generation.
var ScriptServiceDef = meta.ServiceDef{
	Name:    "scripts",
	Short:   "Script discovery and execution service",
	Long:    "Discovers and executes scripts from sidecar embed/scripts/ directories. Scripts are addressed by provider:name URIs. Supports TypeScript (bun), Python, Bash, and Go runtimes.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"List": {Short: "List available scripts", RenderTemplate: listScriptsTmpl},
		"Run":  {Short: "Execute a script by provider:name"},
	},
}

// --- Input/Output types ---

type ScriptRunInput struct {
	Name   string            `json:"name"   cli:"arg" desc:"Script URI (provider:name)" required:"true"`
	Params map[string]string `json:"params,omitempty" desc:"Named parameters (passed as SCRIPT_PARAM_* env vars)"`
	Args   []string          `json:"args,omitempty"   desc:"Positional arguments"`
	Stdin  io.Reader         `json:"-"` // not serializable — for programmatic use only
}

type ScriptRunOutput struct {
	Stdout   string `json:"stdout"           desc:"Execution stdout"`
	Stderr   string `json:"stderr,omitempty" desc:"Execution stderr"`
	ExitCode int    `json:"exitCode"         desc:"Exit code (0 = success)"`
}

type ScriptListOutput struct {
	Scripts []ScriptInfo `json:"scripts" desc:"Available scripts" ui-cli:"mode=table,columns=name;provider;extension;description,primary=name,sort=provider;name"`
}

type ScriptInfo struct {
	Name        string `json:"name"        desc:"Script name"`
	Provider    string `json:"provider"    desc:"Provider (sidecar name)"`
	Description string `json:"description" desc:"What this script does"`
	Extension   string `json:"extension"   desc:"File extension (.ts, .py, .sh, .go)"`
	// TimeoutS is how long a run of the script may last, in seconds: the
	// bound its scripts.yaml entry declares (timeout_s), else DefaultTimeout.
	// The engine kills the run at it, and the script's slot method advertises
	// it, so a caller waits exactly that.
	TimeoutS int `json:"timeoutS,omitempty" desc:"How long a run may last, in seconds"`
}

// DefaultTimeout bounds a run of a script that declares no timeout_s.
const DefaultTimeout = 5 * time.Minute

// TimeoutOf is the bound a run of a script declaring timeoutS gets: the
// declared seconds, else DefaultTimeout.
func TimeoutOf(timeoutS int) time.Duration {
	if timeoutS > 0 {
		return time.Duration(timeoutS) * time.Second
	}
	return DefaultTimeout
}

// URI returns the provider:name identifier.
func (s ScriptInfo) URI() string {
	if s.Provider == "" {
		return s.Name
	}
	return s.Provider + ":" + s.Name
}

// --- ScriptEngine ---

// ScriptEngine is the interface for a pluggable script runtime.
// Each engine handles a set of file extensions.
type ScriptEngine interface {
	// Name returns the engine identifier (e.g., "bun", "python", "bash").
	Name() string

	// Extensions returns the file extensions this engine handles (e.g., [".ts", ".js"]).
	Extensions() []string

	// Run executes a script with the given input.
	Run(ctx context.Context, in ScriptExecInput) (ScriptRunOutput, error)
}

// ScriptExecInput is the low-level execution input for a ScriptEngine.
type ScriptExecInput struct {
	Name           string            // human-readable identifier
	Data           []byte            // script content
	Extension      string            // file extension (.ts, .py, .sh)
	CompanionFiles map[string][]byte // additional files (e.g., package.json)
	InstallDeps    bool              // run dependency install before execution
	Stdin          io.Reader
	Env            []string
	Args           []string
	Timeout        time.Duration
}

// --- ScriptLoader ---

// ScriptLoader is a typed wrapper for the script files extension point.
type ScriptLoader struct {
	Loader   loader.Loader
	Root     string // root path (e.g., "embed/scripts")
	Provider string // provider name (e.g., "ktw-docs")
}

// --- Script manifest ---

// ScriptManifest is the parsed scripts.yaml file.
type ScriptManifest struct {
	Scripts []ScriptManifestEntry `yaml:"scripts"`
}

// ScriptManifestEntry is one script in the manifest.
type ScriptManifestEntry struct {
	Name           string            `yaml:"name"`
	Path           string            `yaml:"path"`
	Description    string            `yaml:"description"`
	CompanionFiles map[string]string `yaml:"companion_files,omitempty"` // filename → content
	InstallDeps    bool              `yaml:"install_deps,omitempty"`
	// TimeoutS declares how long a run of the script may last, in seconds
	// (0 = DefaultTimeout).
	TimeoutS int `yaml:"timeout_s,omitempty"`
}
