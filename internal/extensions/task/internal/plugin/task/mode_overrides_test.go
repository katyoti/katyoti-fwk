// SPDX-License-Identifier: Apache-2.0

package task

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// These tests were migrated from calling the in-package
// configOverrideFromModeWithParent helper to calling the new unified
// presets.BuildChildOverride entry point as part of the OD-001 spawn-policy
// refactor (tier-1-spawn-policies-refactor 2026-05-07). Same assertions,
// same UCs covered (UC-053), new entry point.

func TestConfigOverrideFromMode_WorkerExplicitAllowAll(t *testing.T) {
	// Worker preset has Tools=["*"] (explicit allow-all). It MUST serialize to
	// spawnPermissions.allowedTools=["*"] — otherwise the child arrives with
	// zero SpawnPermissions and the SpawnPermissionHook fail-closes on dangerous
	// tools (file-write/bash), silently denying a delegated worker its tools.
	// The wildcard is allow-all, not a restriction. HistoryMode is still injected.
	override := presets.BuildChildOverride(presets.ModeWorker, "")
	if override == nil {
		t.Fatal("worker should produce override with historyMode")
	}
	sp, ok := override["spawnPermissions"].(map[string]any)
	if !ok {
		t.Fatalf("worker must inject explicit allow-all spawnPermissions, got %v", override["spawnPermissions"])
	}
	allowed, _ := sp["allowedTools"].([]any)
	if len(allowed) != 1 || allowed[0] != "*" {
		t.Errorf("worker allowedTools must be wildcard [*], got %v", allowed)
	}
	if override["historyMode"] != "none" {
		t.Errorf("worker historyMode = %v, want none", override["historyMode"])
	}
}

// A fork child's override carries a background child's permissions — an
// explicit allow-all, no permission mode the agent factory refuses.
func TestConfigOverrideFromMode_ForkAllowsLikeBackground(t *testing.T) {
	for _, mode := range []presets.Mode{presets.ModeFork, presets.ModeForkIsolated} {
		override := presets.BuildChildOverride(mode, "")
		sp, ok := override["spawnPermissions"].(map[string]any)
		if !ok {
			t.Fatalf("%s: spawnPermissions not a map: %T", mode, override["spawnPermissions"])
		}
		if pm, set := sp["permissionMode"]; set {
			t.Errorf("%s: permissionMode = %v, want none (the factory refuses bubble)", mode, pm)
		}
		allowed, _ := sp["allowedTools"].([]any)
		if len(allowed) != 1 || allowed[0] != "*" {
			t.Errorf("%s: allowedTools = %v, want [*]", mode, allowed)
		}
	}
}

func TestConfigOverrideFromMode_CoordinatorSpawningBackground(t *testing.T) {
	// Coordinator parent spawning a background child → child gets safe subset
	// via the CoordinatorRestrictsChildrenPolicy on presetCoordinator.
	// (UC-053 — was the original entry point for the coord-spawn assertion.)
	override := presets.BuildChildOverride(presets.ModeBackground, presets.ModeCoordinator)
	if override == nil {
		t.Fatal("coordinator parent should produce override")
	}
	sp := override["spawnPermissions"].(map[string]any)
	if sp["permissionMode"] != "restricted" {
		t.Errorf("permissionMode = %v, want restricted", sp["permissionMode"])
	}
	allowed, _ := sp["allowedTools"].([]any)
	if len(allowed) != len(presets.SafeToolSubset) {
		t.Errorf("allowedTools size = %d, want %d", len(allowed), len(presets.SafeToolSubset))
	}
}

func TestConfigOverrideFromMode_UnknownReturnsNil(t *testing.T) {
	override := presets.BuildChildOverride("no-such-mode", "")
	if override != nil {
		t.Errorf("unknown mode should return nil, got %v", override)
	}
}
