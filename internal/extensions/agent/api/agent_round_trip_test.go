// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"testing"
)

// AgentRunRequest is the wire-format request envelope for the agent
// strategy. The fields that carry per-spawn dimensions (HistoryMode,
// WorkspacePaths, MaxBudgetUSD) must round-trip through JSON without
// loss — every cross-process spawn relies on this.
//
// These tests close the L1 layer for UCs whose semantics are
// "the dimension is preserved across the spawn boundary" — the
// behavioral closures live in agent runtime tests or L2 scenarios.

func TestAgentRunRequest_HistoryModeNone_RoundTrip(t *testing.T) {
	// UC-070 — HistoryMode "none" survives serialize+deserialize.
	req := AgentRunRequest{HistoryMode: "none"}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got AgentRunRequest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.HistoryMode != "none" {
		t.Errorf("HistoryMode round-trip = %q, want none", got.HistoryMode)
	}
	if got.InitialMessages != nil {
		t.Errorf("InitialMessages should be empty for HistoryMode none, got %v", got.InitialMessages)
	}
}

func TestAgentRunRequest_HistoryModePromptOnly_RoundTrip(t *testing.T) {
	// UC-071 — HistoryMode "prompt_only" survives serialize+deserialize.
	req := AgentRunRequest{HistoryMode: "prompt_only"}
	raw, _ := json.Marshal(req)
	var got AgentRunRequest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.HistoryMode != "prompt_only" {
		t.Errorf("HistoryMode round-trip = %q, want prompt_only", got.HistoryMode)
	}
}

func TestAgentRunRequest_HistoryModeFullFork_RoundTrip(t *testing.T) {
	// UC-072 — HistoryMode "full_fork" + InitialMessages round-trip preserves
	// the pre-populated conversation. This is the data half; the validation
	// half ("full_fork requires InitialMessages") is in
	// factory_validation_test.go.
	req := AgentRunRequest{
		HistoryMode: "full_fork",
		InitialMessages: []Message{
			{Role: "user"},
			{Role: "assistant"},
		},
	}
	raw, _ := json.Marshal(req)
	var got AgentRunRequest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.HistoryMode != "full_fork" {
		t.Errorf("HistoryMode round-trip = %q, want full_fork", got.HistoryMode)
	}
	if len(got.InitialMessages) != 2 {
		t.Errorf("InitialMessages len = %d, want 2", len(got.InitialMessages))
	}
}

func TestAgentRunRequest_WorkspacePathsReadPaths_RoundTrip(t *testing.T) {
	// UC-074 — WorkspacePaths.ReadPaths preserves across the spawn boundary.
	// The hook-enforcement half is verified by agent runtime tests; this
	// closes the wire-format guarantee.
	req := AgentRunRequest{
		WorkspacePaths: &WorkspacePaths{
			ReadPaths:  []string{"src/", "docs/"},
			WritePaths: []string{"src/generated/"},
		},
	}
	raw, _ := json.Marshal(req)
	var got AgentRunRequest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.WorkspacePaths == nil {
		t.Fatal("WorkspacePaths nil after round-trip")
	}
	if len(got.WorkspacePaths.ReadPaths) != 2 ||
		got.WorkspacePaths.ReadPaths[0] != "src/" ||
		got.WorkspacePaths.ReadPaths[1] != "docs/" {
		t.Errorf("ReadPaths = %v, want [src/, docs/]", got.WorkspacePaths.ReadPaths)
	}
	if len(got.WorkspacePaths.WritePaths) != 1 || got.WorkspacePaths.WritePaths[0] != "src/generated/" {
		t.Errorf("WritePaths = %v, want [src/generated/]", got.WorkspacePaths.WritePaths)
	}
}

func TestAgentRunRequest_MaxBudgetUSD_RoundTrip(t *testing.T) {
	// UC-043 — MaxBudgetUSD field preserves across the spawn boundary.
	// The actual abort-on-overspend behavior requires a real LLM (L3).
	req := AgentRunRequest{MaxBudgetUSD: 12.50}
	raw, _ := json.Marshal(req)
	var got AgentRunRequest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.MaxBudgetUSD != 12.50 {
		t.Errorf("MaxBudgetUSD round-trip = %v, want 12.50", got.MaxBudgetUSD)
	}
}
