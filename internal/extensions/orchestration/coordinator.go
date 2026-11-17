// SPDX-License-Identifier: Apache-2.0

package orchestration

import "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"

// CoordinatorManager provides mode-specific behavioral prompts for agents.
// Each delegation mode (worker, coordinator, team-leader, etc.) gets a
// distinct prompt that guides the agent's behavior.
type CoordinatorManager struct{}

// NewCoordinatorManager creates a coordinator manager.
func NewCoordinatorManager() *CoordinatorManager {
	return &CoordinatorManager{}
}

// GetModePrompt returns the behavioral prompt for the given delegation mode.
// If mode is empty, returns "" (no extra prompt — task body is sufficient).
//
// Resolution order:
//  1. Local hardcoded prompts (worker/background/coordinator/team-leader/teammate)
//     — these are the orchestration-flavored versions with team/messaging guidance
//  2. presets.PromptTemplate(mode) — fork/isolated/fork-isolated/remote
//     — the canonical mode prompts from the presets package
//
// This unifies both prompt sources so every mode in the spec gets behavioral
// guidance injected into the system prompt.
func (cm *CoordinatorManager) GetModePrompt(mode string) string {
	switch mode {
	case "coordinator":
		return coordinatorPrompt
	case "team-leader":
		return teamLeaderPrompt
	case "worker":
		return workerPrompt
	case "background":
		return backgroundPrompt
	case "teammate":
		return teammatePrompt
	}
	// Fall back to presets — covers fork, isolated, fork-isolated, remote.
	return presets.PromptTemplate(presets.Mode(mode))
}

// GetWorkerToolSet returns the predefined safe tool subset for workers
// spawned by a coordinator.
func (cm *CoordinatorManager) GetWorkerToolSet() []string {
	return []string{
		"file-read",
		"file-write",
		"file-edit",
		"glob",
		"complete",
	}
}

const workerPrompt = "## Worker Mode\n\n" +
	"You are a specialist worker. Focus on your specific task. If input was provided, use it directly — do not ask the user to repeat it.\n\n" +
	"**Termination**: When the task is done, you MUST use the **complete** tool to return your result. " +
	"The `summary` field is the ONLY output the parent agent receives — include ALL relevant data, not just a description of what you did. " +
	"The parent cannot see your conversation, only your summary.\n\n"

const backgroundPrompt = "## Background Worker Mode\n\n" +
	"You are a background worker. You are running in the background — the parent is not waiting for you. " +
	"Focus on your specific task. Do not ask questions.\n\n" +
	"**Termination**: When the task is done, you MUST use the **complete** tool to record your result.\n\n"

const coordinatorPrompt = "## Coordinator Mode\n\n" +
	"You are a coordinator agent. Orchestrate specialists to produce the deliverables.\n\n" +
	"### Rules\n" +
	"- When delegating, pass ALL required data via input or prompt — workers cannot see your conversation\n" +
	"- When a worker completes, its `complete` summary is the ONLY output you receive — synthesize it before delegating the next step\n" +
	"- Use `mode: \"worker\"` (synchronous) when you need the result before proceeding\n" +
	"- Use `mode: \"background\"` when tasks can run in parallel\n\n" +
	"**Termination**: When all deliverables are produced and every required specialist has been consulted, you MUST use the **complete** tool to return the final summary to the user. Do not end the conversation with plain text — the `complete` tool's `summary` is the only output the user sees.\n\n"

const teamLeaderPrompt = "## Team Leader Mode\n\n" +
	"You are the leader of a team. Create your team, spawn teammates, and coordinate their work.\n\n" +
	"### Protocol\n" +
	"1. Create the team with `create-team`\n" +
	"2. Spawn teammates with clear, specific prompts via `spawn-teammate`\n" +
	"3. Monitor progress via `task-status` and `check-messages`\n" +
	"4. Synthesize results when all teammates complete\n" +
	"5. You MUST use the **complete** tool to return the final synthesized result\n\n" +
	"### Communication\n" +
	"- Use `send-message` for direct messages or broadcast with '*'\n" +
	"- Teammates can message you and each other\n" +
	"- Teammates cannot see your conversation — pass ALL required information in their spawn prompt\n\n"

const teammatePrompt = "## Teammate Mode\n\n" +
	"You are a teammate. Focus on your assigned task.\n\n" +
	"### Communication\n" +
	"- Use `check-messages` to check for messages from your leader or other teammates\n" +
	"- Use `send-message` to communicate with your leader or other teammates\n" +
	"- If you need help or are blocked, send a message to your leader\n\n" +
	"**Termination**: When your task is done, you MUST use the **complete** tool to report your result back to the leader.\n\n"
