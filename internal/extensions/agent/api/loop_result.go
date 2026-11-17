// SPDX-License-Identifier: Apache-2.0

package api

// LoopResult is the structured return value when the agent loop exits.
// Richer than RunResult — includes full message history, cost, and plugin metadata.
type LoopResult struct {
	// Messages is the full conversation history (user + assistant + system).
	Messages []Message

	// FinalText is the last assistant text block (convenience extraction).
	FinalText string

	// StopReason is the model's stop_reason on the final assistant turn
	// ("end_turn", "max_tokens"), or a loop-level reason
	// ("max_turns", "max_budget", "aborted", "completed").
	StopReason string

	// TurnCount is the number of model calls (iterations) executed.
	TurnCount int

	// Usage is the cumulative token usage across all model calls.
	Usage Usage

	// Cost is the cumulative USD cost (input + output tokens, cache, etc.).
	Cost float64

	// Errors contains any non-fatal errors encountered during execution.
	// Terminal errors cause the loop to return an error rather than a result.
	Errors []error

	// Meta is an extensible metadata bag populated by plugins via OnResult.
	Meta map[string]any
}

// ToRunResult converts a LoopResult to the legacy RunResult format.
func (r *LoopResult) ToRunResult() RunResult {
	status := RunCompleted
	switch r.StopReason {
	case "aborted":
		status = RunAborted
	case "max_turns":
		status = RunMaxTurns
	}
	if len(r.Errors) > 0 {
		status = RunError
	}

	var summary string
	if s, ok := r.Meta["summary"].(string); ok {
		summary = s
	} else {
		summary = r.FinalText
	}

	var files []string
	if f, ok := r.Meta["files"].([]string); ok {
		files = f
	}

	return RunResult{
		Status:  status,
		Summary: summary,
		Files:   files,
		Turns:   r.TurnCount,
		Usage:   r.Usage,
	}
}
