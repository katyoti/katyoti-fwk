// SPDX-License-Identifier: Apache-2.0

package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// AggregateMetrics holds tree-wide totals across a delegation tree.
// Built by AggregateTree() — sums per-session summary data.
type AggregateMetrics struct {
	// RootSessionID is the session id at the top of the tree.
	RootSessionID string `json:"rootSessionId"`

	// AgentCount is the total number of agent runs in the tree (including the root).
	AgentCount int `json:"agentCount"`

	// TotalTokens is the sum of all input + output tokens across the tree.
	TotalTokens int `json:"totalTokens"`

	// InputTokens / OutputTokens are the per-direction sums.
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`

	// TotalDurationMs is the sum of all session durations (wall-clock per session).
	// Note: this is NOT the wall-clock duration of the root run, which would be less
	// if children ran in parallel. It's a total compute time number.
	TotalDurationMs int `json:"totalDurationMs"`

	// TotalTurns is the sum of all turns across all agents.
	TotalTurns int `json:"totalTurns"`

	// TotalErrors is the sum of error_count across all sessions.
	TotalErrors int `json:"totalErrors"`

	// ToolsByName aggregates tool call counts across the tree.
	// Example: {"file-write": 5, "ask-user": 8, "delegate": 3}
	ToolsByName map[string]int `json:"toolsByName"`

	// SkillsByAgent records which skills each agent accessed.
	// Inferred from get_skill_instructions / get_skill_reference tool calls.
	// Example: {"goal-interview": ["interview-technique"], "plan-builder": ["planning-methodology"]}
	SkillsByAgent map[string][]string `json:"skillsByAgent"`

	// PerAgent gives a flat per-agent breakdown for quick lookups.
	PerAgent []AgentMetrics `json:"perAgent"`
}

// AgentMetrics is the per-session view used inside AggregateMetrics.
type AgentMetrics struct {
	RunID        string         `json:"runId"`
	SessionID    string         `json:"sessionId"`
	Task         string         `json:"task"`
	Mode         string         `json:"mode,omitempty"`
	Status       string         `json:"status"`
	Turns        int            `json:"turns"`
	InputTokens  int            `json:"inputTokens"`
	OutputTokens int            `json:"outputTokens"`
	TotalTokens  int            `json:"totalTokens"`
	DurationMs   int            `json:"durationMs"`
	ErrorCount   int            `json:"errorCount"`
	ToolsByName  map[string]int `json:"toolsByName,omitempty"`
	SkillsLoaded []string       `json:"skillsLoaded,omitempty"`
}

// AggregateTree walks the delegation tree and sums per-session metrics
// from each node's summary.json file. Sessions without a summary.json
// (in-progress or interrupted) contribute zero metrics but still appear
// in PerAgent with whatever metadata is available.
func AggregateTree(tree *DelegationNode) (*AggregateMetrics, error) {
	if tree == nil {
		return nil, fmt.Errorf("nil tree")
	}

	agg := &AggregateMetrics{
		RootSessionID: tree.SessionID,
		ToolsByName:   make(map[string]int),
		SkillsByAgent: make(map[string][]string),
	}

	walkAggregate(tree, agg)
	return agg, nil
}

func walkAggregate(node *DelegationNode, agg *AggregateMetrics) {
	agg.AgentCount++

	metrics := AgentMetrics{
		RunID:     node.RunID,
		SessionID: node.SessionID,
		Task:      node.Task,
		Mode:      node.Mode,
	}

	// Try summary.json first; fall back to computing from journals.
	loaded := false
	sumPath := filepath.Join(node.Dir, "summary.json")
	if data, err := os.ReadFile(sumPath); err == nil {
		var sum Summary
		if json.Unmarshal(data, &sum) == nil {
			metrics.Status = sum.Status
			metrics.Turns = sum.Turns
			metrics.InputTokens = sum.InputTok
			metrics.OutputTokens = sum.OutputTok
			metrics.TotalTokens = sum.TotalTok
			metrics.DurationMs = sum.DurationMs
			metrics.ErrorCount = sum.ErrorCount
			metrics.ToolsByName = sum.ToolCalls
			loaded = true
		}
	}
	if !loaded {
		// Compute from journals on-the-fly (summary.json is not always written)
		llmPath := filepath.Join(node.Dir, "llm.jsonl")
		userPath := filepath.Join(node.Dir, "user.jsonl")
		if s, err := machine.ComputeSummary(llmPath, userPath); err == nil {
			metrics.Status = "complete"
			if s.Status != "" {
				metrics.Status = s.Status
			}
			metrics.Turns = s.Turns
			metrics.InputTokens = s.InputTokens
			metrics.OutputTokens = s.OutputTokens
			metrics.TotalTokens = s.TotalTokens
			metrics.DurationMs = int(s.DurationMs)
			metrics.ErrorCount = s.ErrorCount
			metrics.ToolsByName = s.ToolCalls
			loaded = true
		}
	}

	if loaded {
		agg.TotalTokens += metrics.TotalTokens
		agg.InputTokens += metrics.InputTokens
		agg.OutputTokens += metrics.OutputTokens
		agg.TotalDurationMs += metrics.DurationMs
		agg.TotalTurns += metrics.Turns
		agg.TotalErrors += metrics.ErrorCount
		for tool, count := range metrics.ToolsByName {
			agg.ToolsByName[tool] += count
		}
	}

	// Extract skills loaded by this agent from llm.jsonl tool_calls
	skills := extractSkillsLoaded(node.Dir)
	if len(skills) > 0 {
		metrics.SkillsLoaded = skills
		agg.SkillsByAgent[node.Task] = skills
	}

	agg.PerAgent = append(agg.PerAgent, metrics)

	for _, child := range node.Children {
		walkAggregate(child, agg)
	}
}

// extractSkillsLoaded scans the parsed conversation for get_skill_instructions
// and get_skill_reference tool calls, returning the unique skill names loaded
// by this agent.
//
// Relies on summarizeInput() in parse.go knowing how to extract the bare skill
// name from these tools' input — it returns "skill_name" for get_skill_instructions
// and "skill_name:reference_path" for get_skill_reference.
func extractSkillsLoaded(sessionDir string) []string {
	convo, err := ParseConversation(sessionDir)
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var skills []string
	for _, turn := range convo.Turns {
		for _, tc := range turn.ToolCalls {
			// Post-2026-05-24: `skills-get-reference` was dropped (references
			// resolve via the generic `contributions-get` tool now). Only
			// `skills-get-instruction` remains as a skill-specific tool name.
			if tc.Name != "skills-get-instruction" {
				continue
			}
			// tc.Input is the skill URI. Strip the reference_path suffix to
			// dedup by skill name.
			name := tc.Input
			if idx := strings.Index(name, ":"); idx > 0 {
				name = name[:idx]
			}
			if name != "" && !seen[name] {
				seen[name] = true
				skills = append(skills, name)
			}
		}
	}
	return skills
}
