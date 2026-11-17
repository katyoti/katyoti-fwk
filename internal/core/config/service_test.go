// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

func TestIdFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"settings.yaml", "settings"},
		{"LLM.yaml", "LLM"},
		{"LLM.yml", "LLM"},
		{"LLM.json", "LLM"},
		{"LLM/openai.yaml", "LLM(openai)"},
		{"LLM/openai.yml", "LLM(openai)"},
	}
	for _, tt := range tests {
		got := IdFromPath(tt.path)
		if got != tt.want {
			t.Errorf("IdFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestDotGet(t *testing.T) {
	m := map[string]any{
		"llm": map[string]any{
			"provider": map[string]any{
				"model": "gpt-4",
				"temp":  0.7,
			},
		},
		"simple": "value",
	}

	tests := []struct {
		key  string
		want any
		ok   bool
	}{
		{"simple", "value", true},
		{"llm.provider.model", "gpt-4", true},
		{"llm.provider.temp", 0.7, true},
		{"llm.provider", map[string]any{"model": "gpt-4", "temp": 0.7}, true},
		{"missing", nil, false},
		{"llm.missing", nil, false},
		{"llm.provider.missing", nil, false},
	}
	for _, tt := range tests {
		got, ok := dotGet(m, tt.key)
		if ok != tt.ok {
			t.Errorf("dotGet(%q): ok = %v, want %v", tt.key, ok, tt.ok)
			continue
		}
		if !tt.ok {
			continue
		}
		switch w := tt.want.(type) {
		case string:
			if got != w {
				t.Errorf("dotGet(%q) = %v, want %v", tt.key, got, w)
			}
		case float64:
			if got != w {
				t.Errorf("dotGet(%q) = %v, want %v", tt.key, got, w)
			}
		case map[string]any:
			gm, ok := got.(map[string]any)
			if !ok {
				t.Errorf("dotGet(%q) = %T, want map[string]any", tt.key, got)
			} else if len(gm) != len(w) {
				t.Errorf("dotGet(%q) map len = %d, want %d", tt.key, len(gm), len(w))
			}
		}
	}
}

func TestDeepMerge(t *testing.T) {
	dst := map[string]any{
		"a": "original",
		"nested": map[string]any{
			"x": 1,
			"y": 2,
		},
	}
	src := map[string]any{
		"b": "new",
		"nested": map[string]any{
			"y": 99,
			"z": 3,
		},
	}
	deepMerge(dst, src)

	if dst["a"] != "original" {
		t.Errorf("a = %v, want original", dst["a"])
	}
	if dst["b"] != "new" {
		t.Errorf("b = %v, want new", dst["b"])
	}
	nested := dst["nested"].(map[string]any)
	if nested["x"] != 1 {
		t.Errorf("nested.x = %v, want 1", nested["x"])
	}
	if nested["y"] != 99 {
		t.Errorf("nested.y = %v, want 99 (src wins)", nested["y"])
	}
	if nested["z"] != 3 {
		t.Errorf("nested.z = %v, want 3", nested["z"])
	}
}

func TestResolveServiceConfig_4Layers(t *testing.T) {
	s := &ConfigService{
		configTree: map[string]any{
			"LLM": map[string]any{
				"provider": "openai",
				"model":    "gpt-3.5",
				"temp":     0.5,
			},
			"LLM(custom)": map[string]any{
				"model": "gpt-4",
				"extra": "instance-only",
			},
		},
	}

	result, err := s.ResolveServiceConfig("LLM", "custom",
		WithDefaults(map[string]any{
			"timeout": 30,
			"temp":    0.0,
		}),
		WithOverrides(map[string]any{
			"temp": 1.0,
		}),
	)
	if err != nil {
		t.Fatalf("ResolveServiceConfig: %v", err)
	}

	if result["timeout"] != 30 {
		t.Errorf("timeout = %v, want 30 (from defaults)", result["timeout"])
	}
	if result["provider"] != "openai" {
		t.Errorf("provider = %v, want openai (from class)", result["provider"])
	}
	if result["model"] != "gpt-4" {
		t.Errorf("model = %v, want gpt-4 (from instance)", result["model"])
	}
	if result["extra"] != "instance-only" {
		t.Errorf("extra = %v, want instance-only", result["extra"])
	}
	if result["temp"] != 1.0 {
		t.Errorf("temp = %v, want 1.0 (from override)", result["temp"])
	}
}

func TestConfigService_DotNotation(t *testing.T) {
	s := &ConfigService{
		configTree: map[string]any{
			"llm": map[string]any{
				"provider": "anthropic",
				"model":    "claude-3",
			},
			"debug": true,
			"count": 42,
		},
	}

	if s.GetString("llm.provider", "") != "anthropic" {
		t.Errorf("GetString llm.provider = %q, want anthropic", s.GetString("llm.provider", ""))
	}
	if s.GetBool("debug", false) != true {
		t.Error("GetBool debug = false, want true")
	}
	if s.GetInt("count", 0) != 42 {
		t.Errorf("GetInt count = %d, want 42", s.GetInt("count", 0))
	}
	if s.GetString("missing", "fallback") != "fallback" {
		t.Error("GetString missing should return fallback")
	}
}

func TestConfigService_Sub(t *testing.T) {
	parent := &ConfigService{
		configLoader: loader.NewMapLoader(nil),
		configTree: map[string]any{
			"parent_key": "parent_val",
		},
	}

	childLoader := loader.NewMapLoader(nil)
	child := parent.Sub(childLoader)

	if child.Get("parent_key", nil) != "parent_val" {
		t.Errorf("child.Get(parent_key) = %v, want parent_val", child.Get("parent_key", nil))
	}
}

// GetMapLayered returns parent + child layers in priority order (parent
// first, child last) — used for APPEND semantics (Plugins.yaml).
func TestConfigService_GetMapLayered_BothLayers(t *testing.T) {
	parent := &ConfigService{
		configLoader: loader.NewMapLoader(nil),
		configTree: map[string]any{
			"Plugins": map[string]any{"defaults": []any{"a", "b"}},
		},
	}
	child := parent.Sub(loader.NewMapLoader(nil))
	child.configTree = map[string]any{
		"Plugins": map[string]any{"defaults": []any{"c"}},
	}

	layers := child.GetMapLayered("Plugins")
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}
	// Parent first.
	if d, _ := layers[0]["defaults"].([]any); len(d) != 2 {
		t.Errorf("layers[0] (parent) defaults = %v, want 2 entries", d)
	}
	if d, _ := layers[1]["defaults"].([]any); len(d) != 1 {
		t.Errorf("layers[1] (child) defaults = %v, want 1 entry", d)
	}
}

// GetMapLayered skips empty parents (no entry for the named key).
func TestConfigService_GetMapLayered_SkipsEmpty(t *testing.T) {
	parent := &ConfigService{
		configLoader: loader.NewMapLoader(nil),
		configTree:   map[string]any{}, // no Plugins key
	}
	child := parent.Sub(loader.NewMapLoader(nil))
	child.configTree = map[string]any{
		"Plugins": map[string]any{"defaults": []any{"x"}},
	}
	layers := child.GetMapLayered("Plugins")
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer (only child), got %d", len(layers))
	}
}

// GetMapLayered returns empty slice when no layer has the key.
func TestConfigService_GetMapLayered_AllEmpty(t *testing.T) {
	parent := &ConfigService{configLoader: loader.NewMapLoader(nil), configTree: map[string]any{}}
	child := parent.Sub(loader.NewMapLoader(nil))
	if got := child.GetMapLayered("Plugins"); len(got) != 0 {
		t.Errorf("expected empty layers, got %d", len(got))
	}
}

// Standalone (no parent) ConfigService still returns its own layer when present.
func TestConfigService_GetMapLayered_NoParent(t *testing.T) {
	s := &ConfigService{
		configLoader: loader.NewMapLoader(nil),
		configTree: map[string]any{
			"Plugins": map[string]any{"defaults": []any{"only"}},
		},
	}
	layers := s.GetMapLayered("Plugins")
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(layers))
	}
}
