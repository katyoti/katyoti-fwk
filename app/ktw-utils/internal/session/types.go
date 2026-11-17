// SPDX-License-Identifier: Apache-2.0

// Package session provides deterministic analysis of ktw-agents session data.
// It reads the dual JSONL journals (llm.jsonl + user.jsonl) and summary.json
// produced by Stage 1 (session-observability) and computes evaluation reports
// answering 3 questions: outcome quality, efficiency, and internal health.
//
// This is a library — CLI commands and future LLM tool wrappers are thin callers.
package session

import "time"

// Severity levels for findings.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// FindingType categorizes what area a finding relates to.
type FindingType string

const (
	FindingEfficiency  FindingType = "efficiency"
	FindingQuality     FindingType = "quality"
	FindingHealth      FindingType = "health"
	FindingAbandonment FindingType = "abandonment"
)

// Finding is a typed diagnostic produced by analysis.
type Finding struct {
	Type      FindingType `json:"type"`
	Severity  Severity    `json:"severity"`
	Message   string      `json:"message"`
	Metric    string      `json:"metric"`
	Value     float64     `json:"value"`
	Threshold float64     `json:"threshold"`
}

// SessionReport is the full evaluation of a single session.
type SessionReport struct {
	SessionID  string            `json:"session_id"`
	Task       string            `json:"task"`
	Model      string            `json:"model"`
	DurationMs int64             `json:"duration_ms"`
	Timestamp  time.Time         `json:"timestamp"`
	Outcome    OutcomeQuality    `json:"outcome"`
	Efficiency EfficiencyMetrics `json:"efficiency"`
	Health     InternalHealth    `json:"health"`
	Findings   []Finding         `json:"findings"`
}

// OutcomeQuality answers: "Did the user get the expected result?"
type OutcomeQuality struct {
	Status      string `json:"status"`
	Turns       int    `json:"turns"`
	Corrections int    `json:"corrections"`
	Abandoned   bool   `json:"abandoned"`
	Verdict     string `json:"verdict"`
}

// EfficiencyMetrics answers: "Were LLM calls and user interactions optimal?"
type EfficiencyMetrics struct {
	TotalTokens       int                `json:"total_tokens"`
	InputTokens       int                `json:"input_tokens"`
	OutputTokens      int                `json:"output_tokens"`
	TokensPerTurn     float64            `json:"tokens_per_turn"`
	ContentOnlyTurns  int                `json:"content_only_turns"`
	ContentOnlyRatio  float64            `json:"content_only_ratio"`
	ToolsAvailable    int                `json:"tools_available"`
	ToolsUsed         int                `json:"tools_used"`
	UnusedTools       []string           `json:"unused_tools"`
	UnusedToolRatio   float64            `json:"unused_tool_ratio"`
	AskUserCount      int                `json:"ask_user_count"`
	UserResponseTimes []UserResponseTime `json:"user_response_times,omitempty"`
	AvgUserResponseMs int64              `json:"avg_user_response_ms"`
	Verdict           string             `json:"verdict"`
}

// UserResponseTime captures a single prompt→answer timing.
type UserResponseTime struct {
	Turn       int    `json:"turn"`
	ToolID     string `json:"tool_id,omitempty"`
	Question   string `json:"question,omitempty"`
	ResponseMs int64  `json:"response_ms"`
}

// InternalHealth answers: "Did the engine and tools work correctly?"
type InternalHealth struct {
	ErrorCount      int           `json:"error_count"`
	ToolCallCount   int           `json:"tool_call_count"`
	ErrorRate       float64       `json:"error_rate"`
	AvgLLMLatencyMs int64         `json:"avg_llm_latency_ms"`
	AvgTTFTMs       int64         `json:"avg_ttft_ms"`
	PerTurnLatency  []TurnLatency `json:"per_turn_latency,omitempty"`
	TTFTTrend       string        `json:"ttft_trend"`
	TimingAnomalies []string      `json:"timing_anomalies,omitempty"`
	Verdict         string        `json:"verdict"`
}

// TurnLatency captures per-turn timing data.
type TurnLatency struct {
	Turn       int   `json:"turn"`
	DurationMs int64 `json:"duration_ms"`
	TTFTMs     int64 `json:"ttft_ms"`
}

// SessionDetail is the enriched session view for the show command.
type SessionDetail struct {
	SessionID   string         `json:"session_id"`
	Task        string         `json:"task"`
	Model       string         `json:"model"`
	Provider    string         `json:"provider"`
	Status      string         `json:"status"`
	DurationMs  int64          `json:"duration_ms"`
	Turns       int            `json:"turns"`
	Tokens      int            `json:"total_tokens"`
	ToolCalls   map[string]int `json:"tool_calls"`
	TurnDetails []TurnDetail   `json:"turn_details"`
}

// TurnDetail is a per-turn view for session show.
type TurnDetail struct {
	Number     int      `json:"number"`
	UserText   string   `json:"user_text,omitempty"`
	AssistText string   `json:"assist_text,omitempty"`
	ToolNames  []string `json:"tool_names,omitempty"`
	Errors     int      `json:"errors"`
	DurationMs int      `json:"duration_ms"`
	InputTok   int      `json:"input_tokens"`
	OutputTok  int      `json:"output_tokens"`
}

// TrendFilter configures which sessions to include in trend computation.
type TrendFilter struct {
	Task  string `json:"task,omitempty"`
	Model string `json:"model,omitempty"`
	Limit int    `json:"limit,omitempty"`
}

// TrendData contains aggregated metrics across sessions.
type TrendData struct {
	SessionCount int                   `json:"session_count"`
	DateRange    [2]string             `json:"date_range"`
	ByTask       map[string]*TaskTrend `json:"by_task"`
	Regressions  []Finding             `json:"regressions"`
}

// TaskTrend holds aggregated metrics for a specific task.
type TaskTrend struct {
	Task           string   `json:"task"`
	SessionCount   int      `json:"session_count"`
	CompletionRate float64  `json:"completion_rate"`
	AvgTurns       float64  `json:"avg_turns"`
	AvgTokens      float64  `json:"avg_tokens"`
	AvgDurationMs  float64  `json:"avg_duration_ms"`
	AvgErrorRate   float64  `json:"avg_error_rate"`
	AvgTTFTMs      float64  `json:"avg_ttft_ms"`
	TokenTrend     string   `json:"token_trend"`
	ErrorTrend     string   `json:"error_trend"`
	RecentSessions []string `json:"recent_sessions"`
}

// MetricsLine is one row in the metrics.jsonl append log.
type MetricsLine struct {
	SessionID         string  `json:"session_id"`
	Task              string  `json:"task"`
	Model             string  `json:"model"`
	Timestamp         string  `json:"timestamp"`
	Status            string  `json:"status"`
	Turns             int     `json:"turns"`
	TotalTokens       int     `json:"total_tokens"`
	ContentOnlyRatio  float64 `json:"content_only_ratio"`
	UnusedToolRatio   float64 `json:"unused_tool_ratio"`
	ErrorRate         float64 `json:"error_rate"`
	AvgLLMLatencyMs   int64   `json:"avg_llm_latency_ms"`
	AvgTTFTMs         int64   `json:"avg_ttft_ms"`
	AskUserCount      int     `json:"ask_user_count"`
	AvgUserResponseMs int64   `json:"avg_user_response_ms"`
	DurationMs        int64   `json:"duration_ms"`
	FindingCount      int     `json:"finding_count"`
}
