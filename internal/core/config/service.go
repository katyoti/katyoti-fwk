// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// ResolveOption configures Resolve behavior.
type ResolveOption func(*resolveOpts)

type resolveOpts struct {
	defaults  map[string]any
	overrides map[string]any
}

// WithDefaults provides a defaults map merged as layer 1 (lowest priority).
func WithDefaults(defaults map[string]any) ResolveOption {
	return func(o *resolveOpts) { o.defaults = defaults }
}

// WithOverrides provides an overrides map merged as layer 4 (highest priority).
func WithOverrides(overrides map[string]any) ResolveOption {
	return func(o *resolveOpts) { o.overrides = overrides }
}

// ConfigService provides config value access, config resolution, and service config merging.
// Created during bootstrap from a loader rooted at the config directory.
//
// Provides:
//   - Dot-notation access for nested config values
//   - Config resolution with optional defaults/overrides
//   - 4-layer service config merge: default → class → instance → override
//   - Parent chaining via Sub()
type ConfigService struct {
	configLoader loader.Loader
	configTree   map[string]any
	parent       *ConfigService
}

// NewConfigService creates a ConfigService backed by the given loader.
// The loader should be rooted at the config directory (e.g., {homeDir}/config).
// It scans for YAML/JSON files and builds a merged config tree.
func NewConfigService(cfgLoader loader.Loader) *ConfigService {
	s := &ConfigService{
		configLoader: cfgLoader,
	}
	s.configTree = buildConfigTree(cfgLoader)
	return s
}

// ── Dot-notation access ─────────────────────────────────────────────────

// Get retrieves a value by dot-notation key from the merged config tree.
// Returns defaultVal if the key is not found.
func (s *ConfigService) Get(key string, defaultVal any) any {
	v, ok := dotGet(s.configTree, key)
	if ok {
		return v
	}
	if s.parent != nil {
		return s.parent.Get(key, defaultVal)
	}
	return defaultVal
}

// GetString returns a string value by dot-notation key.
func (s *ConfigService) GetString(key string, defaultVal string) string {
	v := s.Get(key, defaultVal)
	if str, ok := v.(string); ok {
		return str
	}
	return defaultVal
}

// GetInt returns an int value by dot-notation key.
func (s *ConfigService) GetInt(key string, defaultVal int) int {
	v := s.Get(key, defaultVal)
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return defaultVal
}

// GetBool returns a bool value by dot-notation key.
func (s *ConfigService) GetBool(key string, defaultVal bool) bool {
	v := s.Get(key, defaultVal)
	if b, ok := v.(bool); ok {
		return b
	}
	return defaultVal
}

// ── Map access ──────────────────────────────────────────────────────────

// GetMap returns the raw config tree entry for the given name as map[string]any.
// Returns (nil, false) if the name has no config tree entry.
func (s *ConfigService) GetMap(name string) (map[string]any, bool) {
	v, ok := s.configTree[name]
	if !ok {
		if s.parent != nil {
			return s.parent.GetMap(name)
		}
		return nil, false
	}
	m, ok := v.(map[string]any)
	return m, ok
}

// GetMapLayered returns the raw config tree entry for `name` from THIS
// service AND every parent in the chain. Order: lowest priority (root parent)
// first, highest priority (this) last. Empty / missing layers are omitted —
// the returned slice has only present layers.
//
// Used for APPEND semantics where consumers need each layer's value
// separately so they can concatenate slice fields. For REPLACE semantics
// (universe wins), use GetMap.
//
// See modules/config/concepts.md ("Two layers") — Plugins.yaml is the one
// APPEND case.
func (s *ConfigService) GetMapLayered(name string) []map[string]any {
	var layers []map[string]any
	if s.parent != nil {
		layers = s.parent.GetMapLayered(name)
	}
	if v, ok := s.configTree[name]; ok {
		if m, ok := v.(map[string]any); ok && len(m) > 0 {
			layers = append(layers, m)
		}
	}
	return layers
}

// ── Config resolution ───────────────────────────────────────────────────

// ResolveConfig returns the raw merged config map for a given config name.
// Applies options (defaults, overrides) if provided.
func (s *ConfigService) ResolveConfig(configName string, opts ...ResolveOption) (map[string]any, error) {
	var o resolveOpts
	for _, opt := range opts {
		opt(&o)
	}

	result := make(map[string]any)

	if o.defaults != nil {
		deepMerge(result, o.defaults)
	}

	// GetMap already handles local → parent fallback
	if data, ok := s.GetMap(configName); ok {
		deepMerge(result, data)
	}

	if o.overrides != nil {
		deepMerge(result, o.overrides)
	}

	return result, nil
}

// LoadConfig resolves a config by name and decodes it into target.
// Missing config is not an error — target keeps its defaults.
//
// ProcessConfig (envkey, default, required tags) always runs, even when
// no config file exists. This ensures env-var-backed fields are resolved
// regardless of whether a YAML file is present.
func (s *ConfigService) LoadConfig(configName string, target any, opts ...ResolveOption) error {
	merged, err := s.ResolveConfig(configName, opts...)
	if err != nil {
		return fmt.Errorf("config %q: %w", configName, err)
	}
	if len(merged) > 0 {
		// DecodeMap → DecodeBytes → ProcessConfig (envkey/default/required).
		return DecodeMap(merged, target)
	}
	// No config file found — still run ProcessConfig so envkey/default/required
	// tags fire on the zero-value struct.
	if reflect.TypeOf(target).Kind() == reflect.Ptr &&
		reflect.TypeOf(target).Elem().Kind() == reflect.Struct {
		return ProcessConfig(target)
	}
	return nil
}

// ── Service config resolution (4-layer merge) ───────────────────────────

// ResolveServiceConfig merges config for a service and returns the result:
//  1. defaults (WithDefaults option)
//  2. class-level: config keyed by className
//  3. instance-level: config keyed by className(instanceName)
//  4. overrides (WithOverrides option)
func (s *ConfigService) ResolveServiceConfig(className, instanceName string, opts ...ResolveOption) (map[string]any, error) {
	var o resolveOpts
	for _, opt := range opts {
		opt(&o)
	}

	result := make(map[string]any)

	// Layer 1: defaults
	if o.defaults != nil {
		deepMerge(result, o.defaults)
	}

	// Layer 2: class-level (e.g., "LLM") — GetMap handles local → parent fallback
	if classData, ok := s.GetMap(className); ok {
		deepMerge(result, classData)
	}

	// Layer 3: instance-level (e.g., "LLM(openai)") — GetMap handles local → parent fallback
	if instanceName != "" {
		instanceKey := ServiceName(className, instanceName)
		if instData, ok := s.GetMap(instanceKey); ok {
			deepMerge(result, instData)
		}
	}

	// Layer 4: overrides
	if o.overrides != nil {
		deepMerge(result, o.overrides)
	}

	return result, nil
}

// LoadServiceConfig resolves and decodes service config into target.
func (s *ConfigService) LoadServiceConfig(className, instanceName string, target any, opts ...ResolveOption) error {
	merged, err := s.ResolveServiceConfig(className, instanceName, opts...)
	if err != nil {
		return fmt.Errorf("service config %q(%q): %w", className, instanceName, err)
	}
	if len(merged) == 0 {
		return nil
	}
	return DecodeMap(merged, target)
}

// ── Sub ─────────────────────────────────────────────────────────────────

// Sub returns a child ConfigService with this as parent.
// The child reads from its own loader and falls back to this service for missing keys.
func (s *ConfigService) Sub(l loader.Loader) *ConfigService {
	child := &ConfigService{
		configLoader: l,
		parent:       s,
	}
	child.configTree = buildConfigTree(l)
	return child
}

// ── Config tree building ────────────────────────────────────────────────

func buildConfigTree(l loader.Loader) map[string]any {
	tree := make(map[string]any)

	files := l.List("*.yaml")
	files = append(files, l.List("*.yml")...)
	files = append(files, l.List("*.json")...)

	subDirs := l.List("*/*.yaml")
	subDirs = append(subDirs, l.List("*/*.yml")...)
	subDirs = append(subDirs, l.List("*/*.json")...)
	files = append(files, subDirs...)

	for _, f := range files {
		id := IdFromPath(f)
		var data map[string]any
		if err := loadFileInto(l, f, &data); err != nil || data == nil {
			continue
		}
		tree[id] = data
	}

	return tree
}

func loadFileInto(l loader.Loader, path string, target any) error {
	data, err := l.ReadFile(path)
	if err != nil || data == nil {
		return err
	}
	format, ok := marshal.FormatFor(path)
	if !ok {
		return fmt.Errorf("config: unknown format for %q", path)
	}
	if err := DecodeBytes(data, format, target); err != nil {
		return fmt.Errorf("config %q: %w", path, err)
	}
	return nil
}
