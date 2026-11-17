// SPDX-License-Identifier: Apache-2.0

// Package api defines the public types for the LLM profiles service.
//
// Note: package + Go types still named "runner" for source-tree
// continuity (the folder rename is a separate, larger refactor). The
// user-facing CLI/KTSP slot is `llm-profiles list` (renamed from
// `profiles list` 2026-05-23 per the command-surface decision —
// ktw-agents-only, lives under `sc llm-profiles`).
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// RunnerService lists configured LLM profiles. The task-listing variant
// was removed — that data lives in `tasks list` (TaskService).
type RunnerService interface {
	List(ctx context.Context, in ListProfilesInput) (ListProfilesOutput, error)
}

// RunnerServiceDef describes the RunnerService for auto-generation.
// Service name `llm-profiles` produces KTSP slot
// `<provider>:llm-profiles/list` and (manually wired) CLI verb
// `sc llm-profiles list`. AutoKTSPContainer's top-level CLI is
// SUPPRESSED via WithKTSPSlotTags({"cli": "false"}) — see services.go.
var RunnerServiceDef = meta.ServiceDef{
	Name:    "llm-profiles",
	Short:   "LLM profile management",
	Long:    `Lists configured LLM profiles. Profiles are loaded from LLMProfiles.yaml. ktw-agents only — lives under 'sc llm-profiles'.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"List": {
			Short:          "List configured LLM profiles",
			RenderTemplate: listProfilesTmpl,
		},
	},
}

// ── List ─────────────────────────────────────────────────────────────────

type ListProfilesInput struct{}

type ListProfilesOutput struct {
	Default  string        `json:"default"  desc:"Default profile name"`
	Profiles []ProfileInfo `json:"profiles"`
}

type ProfileInfo struct {
	Name     string `json:"name"     desc:"Profile name"`
	Model    string `json:"model"    desc:"Model type name"`
	MaxTurns string `json:"maxTurns" desc:"Maximum agent turns"`
}

// ── Render templates ─────────────────────────────────────────────────────

const listProfilesTmpl = `{{ header "Profiles" -}}
{{ if .profiles -}}
{{ table .profiles "name" "model" "Max Turns:maxTurns" }}
{{ dim (printf "%d profile(s)" (count .profiles)) }}
{{- if .default }}{{ dim (printf "Default: %s" .default) }}{{ end }}
{{- else -}}
{{ dim "No profiles configured." }}
{{- end }}`
