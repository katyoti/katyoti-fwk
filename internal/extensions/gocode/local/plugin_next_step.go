// SPDX-License-Identifier: Apache-2.0

// Package local adds a gocode-specific agent plugin that injects a
// "## Suggested next step" section into the system prompt whenever
// a persona is spawned on a `module` node. The suggestion is keyed
// off the live state observable via `gocode/ModuleStatus` —
// blocking gates, module health, tier state.
//
// Per modules/code-gocode/concepts.md §8 + the next-step-provider task (Option A —
// merge into CWM plugin block). Lives in gocode/ (not cwm/) so the
// CWM core stays free of method-pack knowledge.
//
// Composition: the existing `cwm` plugin already injects
// `<cwm_object_context>` (node identity + effective constraint rules
// + item counts). This plugin contributes a SECOND system-message
// block, prepended at every turn:
//
//	## Suggested next step
//	- module health is broken — re-run `gocode module-verify` to
//	  see which UC failed
//	- 3 UC failing — spawn the `coder` persona to address them
//	- charter version < 0.1 — publish via `gocode charter-publish`
//
// Falls through to a no-op when the spawn isn't bound to a module
// (or when ModuleStatus returns an error).
package local

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// nextStepPlugin contributes a static system-message block built at
// plugin-creation time. Suggestions reflect the module's state at
// spawn-time; the agent loop's tool-calls can refresh state but the
// initial briefing is what guides the persona's opening action.
type nextStepPlugin struct {
	block string // pre-built "## Suggested next step" block, or empty
}

func (p *nextStepPlugin) Name() string                 { return "gocode-next-step" }
func (p *nextStepPlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (p *nextStepPlugin) Hooks() agentapi.PluginHooks  { return agentapi.PluginHooks{} }
func (p *nextStepPlugin) Stable() bool                 { return true }
func (p *nextStepPlugin) HandleToolCall(_ context.Context, name string, _ json.RawMessage) (*agentapi.ToolCallResult, error) {
	return nil, fmt.Errorf("gocode-next-step plugin has no tools (got %q)", name)
}
func (p *nextStepPlugin) SystemMessage() string { return p.block }

// buildNextStepBlock returns the "## Suggested next step" block
// built from a ModuleStatus snapshot. Empty when the module is
// healthy + at-target + all gates clear (nothing to suggest).
func buildNextStepBlock(status api.ModuleStatusOutput) string {
	var lines []string

	// Health-driven suggestions first — they're the most actionable.
	switch status.Module.Health {
	case api.ModuleHealthBroken:
		lines = append(lines, fmt.Sprintf(
			"- module health is BROKEN (%d UC failing) — re-run `gocode module-verify --id %s` to confirm the failures, then spawn the `coder` persona to address them",
			status.UCTotals.Fail, status.Module.ID))
	case api.ModuleHealthDegraded:
		lines = append(lines, fmt.Sprintf(
			"- module health is DEGRADED — fix the open lint errors / no-test UCs / fail-deferred OD chain before promote (run `gocode module-lint --id %s` for the punch-list)",
			status.Module.ID))
	}

	// UC-specific suggestions (additive to health).
	if status.UCTotals.NoTest > 0 && status.Module.Health != api.ModuleHealthBroken {
		lines = append(lines, fmt.Sprintf(
			"- %d UC with no test — spawn the `tester` persona to author L1 tests + populate test-case items (TCID convention: `L<n>-uc<NNN>-<slug>`)",
			status.UCTotals.NoTest))
	}

	// Audit-finding suggestions.
	if status.AuditCritOpen > 0 || status.AuditHighOpen > 0 {
		lines = append(lines, fmt.Sprintf(
			"- %d critical / %d high audit findings open — spawn the `reviewer` persona to triage them; close via UC implementations or defer via OD",
			status.AuditCritOpen, status.AuditHighOpen))
	}

	// Tier-state suggestions (only when nothing higher-priority).
	if len(lines) == 0 {
		switch status.Module.TierState {
		case api.TierStateBootstrapping:
			lines = append(lines, fmt.Sprintf(
				"- module is bootstrapping (no charter yet) — spawn the `specifier` persona to author UCs, OR run `gocode module-discover --path <code-path> --target-module-id %s` to seed from existing code",
				status.Module.ID))
		case api.TierStateSpecifying:
			lines = append(lines,
				"- module has charter but no UCs yet — `specifier` persona authors the use-case items + DoD bodies")
		case api.TierStatePlanning:
			lines = append(lines,
				"- module is planning — `architect` persona finalizes module decomposition, then handoff to `coder`")
		case api.TierStateGated:
			lines = append(lines, fmt.Sprintf(
				"- module is at the promote gate but blocked — review `blocking_gates` field (currently %d unmet), fix each before retrying `gocode module-promote --id %s`",
				len(status.BlockingGates), status.Module.ID))
		case api.TierStateExecuting:
			lines = append(lines,
				"- module is executing — coder + tester work the UC queue; track progress via `gocode module-verify`")
		}
	}

	// Promote-gate suggestions when all UCs pass + audit clean — the
	// only thing left before promote is the gate items.
	if len(lines) == 0 && len(status.BlockingGates) > 0 {
		lines = append(lines, "- module is technically clean — only the promote gate items remain:")
		for _, g := range status.BlockingGates {
			lines = append(lines, "  - "+g)
		}
		lines = append(lines, fmt.Sprintf(
			"  run `gocode module-promote --id %s` once cleared (or `--force` with audit trail)",
			status.Module.ID))
	}

	if len(lines) == 0 {
		// Nothing actionable. Don't emit a block at all.
		return ""
	}

	var b strings.Builder
	b.WriteString("## Suggested next step\n\n")
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}

// newNextStepPlugin builds the plugin, snapshotting the suggestion
// at creation time. Resolution failures (no _target, no
// GocodeService, NodeGet error) degrade to an empty block — the
// plugin is then harmless.
func newNextStepPlugin(ctx context.Context) agentapi.AgentPlugin {
	pc := agentapi.PluginContextFrom(ctx)
	if pc == nil {
		return &nextStepPlugin{}
	}
	nodeID, _ := pc[agentapi.PluginContextTarget].(string)
	if nodeID == "" {
		return &nextStepPlugin{}
	}
	svc, err := application.UseService[api.GocodeService](ctx)
	if err != nil || svc == nil {
		return &nextStepPlugin{}
	}
	status, err := svc.ModuleStatus(ctx, api.ModuleStatusInput{ID: nodeID})
	if err != nil || status.Module.ID == "" {
		return &nextStepPlugin{}
	}
	return &nextStepPlugin{block: buildNextStepBlock(status)}
}

// nextStepPluginStrategy registers a PluginFactory under the
// strategy name `gocode-next-step`. Bind via `plugins: [gocode-next-step]`
// in any persona's TASK.md. The gocode baseline personas don't yet
// reference this — `plugins: [augmented, cwm]` is the default —
// but adding it is a one-line frontmatter change. v1 wires it from
// the gocodelocal AgentPluginExtension so any persona can opt in.
var nextStepPluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
	"gocode-next-step", "gocode 'suggested next step' system-prompt block",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "gocode-next-step",
			CreateFunc: func(ctx context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{newNextStepPlugin(ctx)}, nil
			},
		}, nil
	},
)

// AgentPluginExtension contributes the `gocode-next-step` plugin
// strategy. Only binaries that include the agent loop (today:
// ktw-agents) should register it. The standalone ktw-gocode sidecar
// has no agent runtime + omits this contribution silently (mirrors
// cwmlocal.AgentPluginExtension's host-tolerance).
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("gocode", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, nextStepPluginStrategy)
		})
	}
}

// Compile-time assertion.
var _ agentapi.AgentPlugin = (*nextStepPlugin)(nil)
