// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scripts/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scripts/internal/engines"
	"gopkg.in/yaml.v3"
)

// ScriptManagerConfig is the config for the script service.
//
// Empty, and that is the statement: scripts come from the loaders a binary
// contributes, not from a file. It carried an `extraDirs` field that
// NewScriptManager ignored — a knob nothing honoured, which would have been
// advertised to operators as if it did something.
type ScriptManagerConfig struct{}

// discoveredScript holds a loaded script's metadata + data.
type discoveredScript struct {
	info           api.ScriptInfo
	data           []byte
	companionFiles map[string][]byte
	installDeps    bool
}

// scriptManager implements api.ScriptService.
type scriptManager struct {
	scripts map[string]*discoveredScript // provider:name → script
	engines map[string]api.ScriptEngine  // extension → engine
}

// NewScriptManager is the service factory.
func NewScriptManager(ctx context.Context, _ *ScriptManagerConfig) (api.ScriptService, error) {
	loaders := application.UseExtensions[api.ScriptLoader](ctx)
	customEngines := application.UseExtensions[api.ScriptEngine](ctx)

	// Build engine registry: extension → engine
	engineMap := make(map[string]api.ScriptEngine)
	for _, eng := range engines.BuiltinEngines() {
		for _, ext := range eng.Extensions() {
			engineMap[ext] = eng
		}
	}
	// Custom engines override built-ins
	for _, eng := range customEngines {
		for _, ext := range eng.Extensions() {
			engineMap[ext] = eng
		}
	}

	scripts := discoverScripts(loaders)

	return &scriptManager{scripts: scripts, engines: engineMap}, nil
}

func discoverScripts(loaders []api.ScriptLoader) map[string]*discoveredScript {
	scripts := make(map[string]*discoveredScript)

	for _, sl := range loaders {
		// Try loading manifest first
		manifest := loadManifest(sl)

		// Register manifest scripts
		for _, entry := range manifest {
			data, err := sl.Loader.ReadFile(path.Join(sl.Root, entry.Path))
			if err != nil || data == nil {
				continue
			}

			ext := filepath.Ext(entry.Path)
			name := entry.Name
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(entry.Path), ext)
			}

			// Build companion files as []byte
			companions := make(map[string][]byte, len(entry.CompanionFiles))
			for k, v := range entry.CompanionFiles {
				companions[k] = []byte(v)
			}

			key := sl.Provider + ":" + name
			scripts[key] = &discoveredScript{
				info: api.ScriptInfo{
					Name:        name,
					Provider:    sl.Provider,
					Description: entry.Description,
					Extension:   ext,
					TimeoutS:    int(api.TimeoutOf(entry.TimeoutS) / time.Second),
				},
				data:           data,
				companionFiles: companions,
				installDeps:    entry.InstallDeps,
			}
		}

		// Auto-discover script files not in manifest
		for _, pattern := range []string{
			path.Join(sl.Root, "*.ts"),
			path.Join(sl.Root, "*.js"),
			path.Join(sl.Root, "*.py"),
			path.Join(sl.Root, "*.sh"),
			path.Join(sl.Root, "*.go"),
		} {
			for _, entry := range sl.Loader.List(pattern) {
				ext := filepath.Ext(entry)
				name := strings.TrimSuffix(filepath.Base(entry), ext)
				key := sl.Provider + ":" + name

				// Skip if already in manifest
				if _, exists := scripts[key]; exists {
					continue
				}

				data, err := sl.Loader.ReadFile(entry)
				if err != nil || data == nil {
					continue
				}

				scripts[key] = &discoveredScript{
					info: api.ScriptInfo{
						Name:      name,
						Provider:  sl.Provider,
						Extension: ext,
						TimeoutS:  int(api.DefaultTimeout / time.Second),
					},
					data: data,
				}
			}
		}
	}

	return scripts
}

func loadManifest(sl api.ScriptLoader) []api.ScriptManifestEntry {
	data, err := sl.Loader.ReadFile(path.Join(sl.Root, "scripts.yaml"))
	if err != nil || data == nil {
		return nil
	}
	var manifest api.ScriptManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil
	}
	return manifest.Scripts
}

func (m *scriptManager) List(_ context.Context) (api.ScriptListOutput, error) {
	var infos []api.ScriptInfo
	for _, s := range m.scripts {
		infos = append(infos, s.info)
	}
	return api.ScriptListOutput{Scripts: infos}, nil
}

func (m *scriptManager) Run(ctx context.Context, in api.ScriptRunInput) (api.ScriptRunOutput, error) {
	// Look up script by provider:name
	script, ok := m.scripts[in.Name]
	if !ok {
		// Fallback: try name-only
		for _, s := range m.scripts {
			if s.info.Name == in.Name {
				script = s
				break
			}
		}
		if script == nil {
			return api.ScriptRunOutput{}, fmt.Errorf("script %q not found", in.Name)
		}
	}

	// Find engine for this extension
	engine, ok := m.engines[script.info.Extension]
	if !ok {
		return api.ScriptRunOutput{}, fmt.Errorf("no engine for extension %q (script %q)", script.info.Extension, in.Name)
	}

	// Build env vars from params
	var env []string
	for k, v := range in.Params {
		env = append(env, fmt.Sprintf("SCRIPT_PARAM_%s=%s", strings.ToUpper(k), v))
	}

	return engine.Run(ctx, api.ScriptExecInput{
		Name:           script.info.URI(),
		Data:           script.data,
		Extension:      script.info.Extension,
		CompanionFiles: script.companionFiles,
		InstallDeps:    script.installDeps,
		Stdin:          in.Stdin,
		Env:            env,
		Args:           in.Args,
		Timeout:        api.TimeoutOf(script.info.TimeoutS),
	})
}
