// SPDX-License-Identifier: Apache-2.0

package render

import (
	"reflect"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/archdiagram"
)

// fakePresetFinder returns a static preset for known names, nil otherwise.
// Lets the projectLayoutHintsWith tests run without seeding the
// archdiagram package's global preset registry.
func fakePresetFinder(named map[string]*archdiagram.LayoutPreset) presetFinder {
	return func(name string) *archdiagram.LayoutPreset {
		return named[name]
	}
}

func archiLayeredPreset() *archdiagram.LayoutPreset {
	return &archdiagram.LayoutPreset{
		Name:        "archi-layered",
		Description: "Hierarchical left-to-right with orthogonal edges",
		LayoutOptions: map[string]string{
			"elk.algorithm":        "layered",
			"elk.direction":        "RIGHT",
			"elk.spacing.nodeNode": "80",
			"elk.layered.spacing.nodeNodeBetweenLayers": "120",
			"org.eclipse.elk.partitioning.activate":     "true",
			"elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
			"elk.edgeRouting":                           "ORTHOGONAL",
			"elk.layered.nodePlacement.strategy":        "NETWORK_SIMPLEX",
		},
	}
}

// TestProjectLayoutHints_NilInput
func TestProjectLayoutHints_NilInput(t *testing.T) {
	if got := projectLayoutHintsWith(nil, nil); got != nil {
		t.Errorf("nil hints: want nil, got %+v", got)
	}
}

// TestProjectLayoutHints_ExplicitFieldsOnly — no preset; explicit
// Algorithm/Direction/ElkOptions go through unchanged.
func TestProjectLayoutHints_ExplicitFieldsOnly(t *testing.T) {
	h := &drawioapi.LayoutHints{
		Algorithm:  drawioapi.LayoutLayered,
		Direction:  "DOWN",
		ElkOptions: map[string]string{"elk.spacing.nodeNode": "60"},
	}
	got := projectLayoutHintsWith(h, nil)
	if got.Algorithm != "layered" {
		t.Errorf("Algorithm: got %q", got.Algorithm)
	}
	if got.Direction != "DOWN" {
		t.Errorf("Direction: got %q", got.Direction)
	}
	if got.ExtraOptions["elk.spacing.nodeNode"] != "60" {
		t.Errorf("ExtraOptions: got %v", got.ExtraOptions)
	}
}

// TestProjectLayoutHints_PresetExpands — preset wins over explicit
// Algorithm/Direction; ExtraOptions populated from preset.
func TestProjectLayoutHints_PresetExpands(t *testing.T) {
	h := &drawioapi.LayoutHints{Preset: "archi-layered"}
	find := fakePresetFinder(map[string]*archdiagram.LayoutPreset{
		"archi-layered": archiLayeredPreset(),
	})
	got := projectLayoutHintsWith(h, find)
	if got.Algorithm != "layered" {
		t.Errorf("Algorithm: got %q, want layered (from preset)", got.Algorithm)
	}
	if got.Direction != "RIGHT" {
		t.Errorf("Direction: got %q, want RIGHT (from preset)", got.Direction)
	}
	if got.ExtraOptions["org.eclipse.elk.partitioning.activate"] != "true" {
		t.Errorf("partitioning.activate: got %v", got.ExtraOptions)
	}
	if got.ExtraOptions["elk.layered.nodePlacement.strategy"] != "NETWORK_SIMPLEX" {
		t.Errorf("nodePlacement: got %v", got.ExtraOptions)
	}
}

// TestProjectLayoutHints_PresetWinsOverExplicitAlgorithm — the analysis
// promised: "preset wins over Algorithm + Direction when both set".
// Verify the contract.
func TestProjectLayoutHints_PresetWinsOverExplicitAlgorithm(t *testing.T) {
	h := &drawioapi.LayoutHints{
		Preset:    "archi-layered",
		Algorithm: drawioapi.LayoutForce, // explicit override that should LOSE to preset
		Direction: "DOWN",                // ditto
	}
	find := fakePresetFinder(map[string]*archdiagram.LayoutPreset{
		"archi-layered": archiLayeredPreset(),
	})
	got := projectLayoutHintsWith(h, find)
	if got.Algorithm != "layered" {
		t.Errorf("Algorithm: got %q, want layered (preset wins)", got.Algorithm)
	}
	if got.Direction != "RIGHT" {
		t.Errorf("Direction: got %q, want RIGHT (preset wins)", got.Direction)
	}
}

// TestProjectLayoutHints_ElkOptionsOverridePreset — caller's explicit
// ElkOptions DO override preset's options per-key (most fine-grained wins).
func TestProjectLayoutHints_ElkOptionsOverridePreset(t *testing.T) {
	h := &drawioapi.LayoutHints{
		Preset:     "archi-layered",
		ElkOptions: map[string]string{"elk.spacing.nodeNode": "300"}, // override
	}
	find := fakePresetFinder(map[string]*archdiagram.LayoutPreset{
		"archi-layered": archiLayeredPreset(),
	})
	got := projectLayoutHintsWith(h, find)
	if got.ExtraOptions["elk.spacing.nodeNode"] != "300" {
		t.Errorf("nodeNode: want 300 (explicit), got %v", got.ExtraOptions["elk.spacing.nodeNode"])
	}
	// Other preset options preserved.
	if got.ExtraOptions["org.eclipse.elk.partitioning.activate"] != "true" {
		t.Errorf("partitioning lost: %v", got.ExtraOptions)
	}
}

// TestProjectLayoutHints_UnknownPresetFallsThrough — preset name that
// doesn't resolve doesn't panic; explicit fields apply as if no preset.
func TestProjectLayoutHints_UnknownPresetFallsThrough(t *testing.T) {
	h := &drawioapi.LayoutHints{
		Preset:    "nonexistent",
		Algorithm: drawioapi.LayoutForce,
		Direction: "LEFT",
	}
	find := fakePresetFinder(map[string]*archdiagram.LayoutPreset{}) // empty registry
	got := projectLayoutHintsWith(h, find)
	if got.Algorithm != "force" {
		t.Errorf("Algorithm: got %q, want force (explicit, no preset)", got.Algorithm)
	}
	if got.Direction != "LEFT" {
		t.Errorf("Direction: got %q", got.Direction)
	}
}

// TestProjectLayoutHints_EmptyPresetSkipsLookup — empty Preset string
// just means "don't use a preset"; explicit fields apply.
func TestProjectLayoutHints_EmptyPresetSkipsLookup(t *testing.T) {
	h := &drawioapi.LayoutHints{
		Preset:    "",
		Algorithm: drawioapi.LayoutStress,
	}
	called := false
	find := func(name string) *archdiagram.LayoutPreset {
		called = true
		return nil
	}
	got := projectLayoutHintsWith(h, find)
	if called {
		t.Error("preset finder should not be called for empty Preset")
	}
	if got.Algorithm != "stress" {
		t.Errorf("Algorithm: got %q", got.Algorithm)
	}
}

// TestProjectLayoutHints_NilFinderSkipsPresetPath — defensive: nil finder
// shouldn't panic; preset path is silently skipped.
func TestProjectLayoutHints_NilFinderSkipsPresetPath(t *testing.T) {
	h := &drawioapi.LayoutHints{Preset: "archi-layered", Algorithm: drawioapi.LayoutForce}
	got := projectLayoutHintsWith(h, nil)
	if got.Algorithm != "force" {
		t.Errorf("Algorithm: got %q, want force (nil finder, no preset applied)", got.Algorithm)
	}
}

// TestProjectLayoutHints_DefaultExtraOptionsNotMutatedByCaller — the
// preset's ExtraOptions map should be COPIED, not aliased, so that
// caller-side ElkOptions edits don't leak back into the preset registry.
// (defensive — guards against future bugs where someone reuses a preset
// across multiple diagrams)
func TestProjectLayoutHints_DefaultExtraOptionsNotMutatedByCaller(t *testing.T) {
	preset := archiLayeredPreset()
	originalSpacing := preset.LayoutOptions["elk.spacing.nodeNode"]
	h := &drawioapi.LayoutHints{
		Preset:     "archi-layered",
		ElkOptions: map[string]string{"elk.spacing.nodeNode": "999"},
	}
	find := fakePresetFinder(map[string]*archdiagram.LayoutPreset{"archi-layered": preset})
	_ = projectLayoutHintsWith(h, find)
	if preset.LayoutOptions["elk.spacing.nodeNode"] != originalSpacing {
		t.Errorf("preset.LayoutOptions mutated! original=%q now=%q",
			originalSpacing, preset.LayoutOptions["elk.spacing.nodeNode"])
	}
}

// TestProjectLayoutHints_ExpectedShape — sanity check the full shape
// after a preset expansion + ElkOptions override looks right.
func TestProjectLayoutHints_ExpectedShape(t *testing.T) {
	h := &drawioapi.LayoutHints{
		Preset:     "archi-layered",
		ElkOptions: map[string]string{"elk.spacing.nodeNode": "300"},
	}
	find := fakePresetFinder(map[string]*archdiagram.LayoutPreset{
		"archi-layered": archiLayeredPreset(),
	})
	got := projectLayoutHintsWith(h, find)
	// ApplyToLayout (in archdiagram/layouts.go) moves elk.spacing.nodeNode
	// and elk.layered.spacing.nodeNodeBetweenLayers OUT of the raw options
	// map and INTO the dedicated NodeSpacing + LayerSpacing fields. Then
	// ExtraELKOptions() excludes those keys. So the ExtraOptions map
	// contains only the "non-standard" preset options.
	want := &archdiagram.LayoutOptions{
		Algorithm:    "layered",
		Direction:    "RIGHT",
		NodeSpacing:  80,
		LayerSpacing: 120,
		ExtraOptions: map[string]string{
			"elk.spacing.nodeNode":                      "300", // caller's override
			"org.eclipse.elk.partitioning.activate":     "true",
			"elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
			"elk.edgeRouting":                           "ORTHOGONAL",
			"elk.layered.nodePlacement.strategy":        "NETWORK_SIMPLEX",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("shape mismatch\nwant: %+v\ngot:  %+v", want, got)
	}
}
