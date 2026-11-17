// SPDX-License-Identifier: Apache-2.0

package archdiagram

import (
	"embed"
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// BuiltinPresets are the layout presets embedded in the ktw-docs binary.
// Loaded lazily from the app's embed.FS via SetEmbeddedFS.
var builtinPresets []LayoutPreset
var builtinPresetsLoaded bool
var builtinEFS *embed.FS

// SetEmbeddedFS stores the app's embed.FS for loading layout presets at runtime.
func SetEmbeddedFS(efs embed.FS) {
	builtinEFS = &efs
}

// GetBuiltinPresets returns all layout presets from the embedded FS.
func GetBuiltinPresets() []LayoutPreset {
	if builtinPresetsLoaded {
		return builtinPresets
	}
	builtinPresetsLoaded = true
	if builtinEFS == nil {
		return nil
	}
	presets, err := LoadLayoutPresets(*builtinEFS, "embed/elk-layouts")
	if err != nil {
		return nil
	}
	builtinPresets = presets
	return builtinPresets
}

// FindPreset looks up a preset by name.
// PresetNames lists the names of the host's layout presets, empty when
// the host embedded none.
func PresetNames() []string {
	var names []string
	for _, p := range GetBuiltinPresets() {
		names = append(names, p.Name)
	}
	return names
}

func FindPreset(name string) *LayoutPreset {
	for _, p := range GetBuiltinPresets() {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

// LayoutPreset is a named ELK layout configuration loaded from embed/elk-layouts/*.yaml.
type LayoutPreset struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	LayoutOptions map[string]string `yaml:"layoutOptions"`
}

// LoadLayoutPresets reads all layout preset YAML files from the embedded FS.
func LoadLayoutPresets(efs embed.FS, dir string) ([]LayoutPreset, error) {
	entries, err := efs.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read layout presets dir %q: %w", dir, err)
	}

	var presets []LayoutPreset
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := efs.ReadFile(path.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var preset LayoutPreset
		if err := yaml.Unmarshal(data, &preset); err != nil {
			continue
		}
		if preset.Name == "" {
			preset.Name = strings.TrimSuffix(entry.Name(), ".yaml")
		}
		presets = append(presets, preset)
	}
	return presets, nil
}

// ApplyPreset applies a layout preset's options to the ELK graph translation.
// The preset's layoutOptions override the default ELK options.
func (p *LayoutPreset) ApplyToLayout(layout *LayoutOptions) {
	if dir, ok := p.LayoutOptions["elk.direction"]; ok {
		layout.Direction = dir
	}
	if algo, ok := p.LayoutOptions["elk.algorithm"]; ok {
		layout.Algorithm = algo
	}
	if ns, ok := p.LayoutOptions["elk.spacing.nodeNode"]; ok {
		layout.NodeSpacing = parseFloat(ns, layout.NodeSpacing)
	}
	if ls, ok := p.LayoutOptions["elk.layered.spacing.nodeNodeBetweenLayers"]; ok {
		layout.LayerSpacing = parseFloat(ls, layout.LayerSpacing)
	}
}

// ExtraELKOptions returns the preset's layoutOptions that are NOT covered
// by the standard LayoutOptions fields — these go directly to ELK.
func (p *LayoutPreset) ExtraELKOptions() map[string]string {
	standard := map[string]bool{
		"elk.algorithm": true, "elk.direction": true,
		"elk.spacing.nodeNode":                      true,
		"elk.layered.spacing.nodeNodeBetweenLayers": true,
	}
	extras := make(map[string]string)
	for k, v := range p.LayoutOptions {
		if !standard[k] {
			extras[k] = v
		}
	}
	return extras
}

func parseFloat(s string, fallback float64) float64 {
	var v float64
	if _, err := fmt.Sscanf(s, "%f", &v); err != nil {
		return fallback
	}
	return v
}
