// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/refmatch"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
)

var strategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"skill", "Skill access tools",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "skill",
			CreateFunc:  skillFactory,
		}, nil
	},
)

// ContributeStrategy registers the skill plugin strategy.
func ContributeStrategy(app *application.Application) error {
	return agentapi.ContributePluginStrategy(app, strategyDef)
}

// skillFactory creates a single skill plugin with the filtered skill list.
//
// Filtering rules based on refs (selectors live in Props["skills"], the
// desugared task `skills:` field):
//   - no refs, OR a ref with no `skills` props → load ALL advertised skills
//   - a ref carrying `skills: [...]` → resolve each entry as a selector
//     (exact name, prefix glob, or `<provider>:/skill/<glob>`) via refmatch
//
// Examples (task config):
//   - skills: [spec-extractor]        → load only "spec-extractor"
//   - skills: ["*"]                   → all advertised skills
//   - skills: [cwm-*]                 → advertised skills named cwm-*
//   - skills: [ktw-methods:/skill/*] → advertised skills from one provider
//
// The bare `skill` member of the `augmented` group (no props) loads all
// advertised skills — the default when a task asks for the bundle.
//
// Data source: `mbapi.Catalog` published by ktw-agents's
// sidecarDiscoveryExt. Catalog entries already carry the full
// `SkillBody` (L0 + L1 in one fetch) — no per-skill RPC at agent
// setup. The `SkillManager` is still resolved (and held by the
// plugin) for runtime ops: `get-skill-reference`, `run-skill-script`
// — both touch arbitrary files that aren't shipped in the descriptor.
// Per `bus-split-aggregator-to-catalog` step 7.
func skillFactory(ctx context.Context, _ agentapi.LoopContext, refs []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		return nil, nil
	}
	// Resolve against the FULL catalog (incl. unadvertised) so an EXACT
	// `skills:` name can reach an unadvertised skill; the refmatch advertised
	// gate keeps bulk selectors (`*`, glob) to advertised-only. (Global
	// discovery — `skills list` — still uses the advertised-only view.)
	skills, err := api.SkillsFromCatalogAll(ctx, cat)
	if err != nil || len(skills) == 0 {
		return nil, nil
	}

	// SkillManager still needed for runtime tool calls (get-skill-reference,
	// run-skill-script) — references and scripts aren't in the descriptor.
	mgr, err := application.UseService[api.SkillManager](ctx)
	if err != nil {
		return nil, nil
	}

	// Resolve via the shared wildcard model (refmatch) over the FULL set.
	// Selectors come from each ref's Props["skills"] list — the desugared task
	// `skills:` field. A ref with NO `skills` props is the bare `skill` member
	// of the `augmented` group, and no refs at all, both mean "all advertised":
	// the `*` selector below, gated to advertised by refmatch. An EXACT name
	// reaches an unadvertised skill. (The legacy `skill:<name>` colon-ref form
	// is retired — use `skills: [<name>]`.)
	var sels []refmatch.Selector
	for _, r := range refs {
		entries := agentapi.StringList(r.Props["skills"])
		if len(entries) == 0 {
			sels = append(sels, refmatch.Selector{Name: "*"})
			continue
		}
		for _, e := range entries {
			sels = append(sels, api.ParseSkillSelector(e))
		}
	}
	if len(sels) == 0 {
		sels = append(sels, refmatch.Selector{Name: "*"})
	}
	matched := refmatch.Match(api.SkillItems(skills), sels)
	want := make(map[string]bool, len(matched))
	for _, m := range matched {
		want[m.Provider+"\x00"+m.Name] = true
	}
	filtered := make([]api.SkillInfo, 0, len(matched))
	for _, s := range skills {
		if want[s.Provider+"\x00"+s.Name] {
			filtered = append(filtered, s)
		}
	}
	skills = filtered
	if len(skills) == 0 {
		// Silence here is how a misspelled `skills:` entry shipped for months:
		// no plugin, no error, no log. Say which selectors matched nothing.
		names := make([]string, 0, len(sels))
		for _, s := range sels {
			names = append(names, s.Provider+":"+s.Name)
		}
		slog.Warn("skills: no skill matches the task's selectors — check the names against `sc skills list`", "selectors", names)
		return nil, nil
	}

	return []agentapi.AgentPlugin{newSkillAccessPlugin(mgr, skills)}, nil
}

type skillAccessPlugin struct {
	mgr    api.SkillManager
	skills []api.SkillInfo
}

func newSkillAccessPlugin(mgr api.SkillManager, skills []api.SkillInfo) *skillAccessPlugin {
	return &skillAccessPlugin{mgr: mgr, skills: skills}
}

func (p *skillAccessPlugin) Name() string                { return "skill" }
func (p *skillAccessPlugin) Hooks() agentapi.PluginHooks { return agentapi.PluginHooks{} }
func (p *skillAccessPlugin) Stable() bool                { return false }

func (p *skillAccessPlugin) SystemMessage() string {
	return api.BuildSystemPrompt(p.skills, api.PromptModeAgent, "")
}

func (p *skillAccessPlugin) ToolDefs() []agentapi.ToolDef {
	// Use typed tools — schemas auto-generated from Go structs
	tools := SkillTools()
	defs := make([]agentapi.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = t.ToolDef()
	}
	return defs
}

func (p *skillAccessPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	// Delegate to typed tools
	for _, t := range SkillTools() {
		if t.ToolDef().Name == name {
			return t.ToolCall(ctx, input)
		}
	}
	return nil, fmt.Errorf("unknown tool: %q", name)
}
