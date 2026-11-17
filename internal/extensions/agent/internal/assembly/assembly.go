// SPDX-License-Identifier: Apache-2.0

// Package assembly is the single place that maps an agent run's
// requirements to its complete plugin list. The factory resolves the
// request, the profile, and the registry-declared plugins, then calls
// Build — every conditional behavior (chat semantics, user-comms
// adapter, loop safety, spawn guards, recovery, trace, model fallback)
// is decided here and nowhere else. The loop receives a finished
// PluginSet and instantiates nothing.
package assembly

import (
	"io"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/plugin/compaction"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/plugin/loopsafety"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/plugin/user_comms_adapter"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/runtime"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/local/keepalive"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// Inputs carries everything the assembly step needs to decide the
// plugin list. Resolved is the registry-driven resolver output
// (profile + TASK.md refs + defaults, groups expanded, requirements
// collected); the assembly composes around it.
type Inputs struct {
	Request *api.AgentRunRequest

	// Resolved is the plugin list from PluginManager.CreatePlugins.
	Resolved []api.AgentPlugin

	// Model is the run's model, reused by the compaction plugin for its
	// summarization call. Nil disables compaction.
	Model llm.Model

	// Compaction is the resolved auto-compaction policy (ratios of the model's
	// context window), translated by the factory from the profile's
	// `compaction:` block. CompactionEnabled gates whether the plugin is added.
	Compaction        compaction.Config
	CompactionEnabled bool

	// TraceWriter enables the debug plugin (--trace).
	TraceWriter io.Writer

	// FallbackModel enables the model-fallback plugin (profile's
	// fallback_model).
	FallbackModel string
}

// Build returns the finished PluginSet for the run.
//
// Composition order (pinned by TestBuild_*):
//
//  1. the resolved plugins — wrapped by the chat `complete`-filter in
//     chat mode (the wrapper is invisible except for the tool catalog)
//  2. keepalive (chat mode only — the OnDecision "end every turn with
//     ask-user" nudge)
//  3. user-comms-adapter (when the run has a resolved UserComms channel)
//  4. output-recovery + abort-cleanup (always)
//  5. debug (when tracing)
//  6. model-fallback (when the profile declares a fallback model)
//  7. loop-safety (always; reads Polling/Idle tool attributes from the
//     live set; idle-exempt in chat mode)
//  8. spawn-permission + workspace-path guards (when the request
//     carries the corresponding restrictions)
//
// The PluginSet's stable-first sort happens in the machine constructor,
// not here — assembly order is the registration order within each
// stability class.
func Build(in Inputs) *runtime.PluginSet {
	req := in.Request
	// The loop terminator is selected by Finish, NOT by the mode name. A
	// `driven` run (chat, or any run launched finish:driven) hides `complete`
	// and runs on keepalive until an external stop; a `self` run exposes
	// `complete` and ends itself. This is the single point that picks between
	// the two — the loop core stays terminator-agnostic.
	driven := req.Finish == string(presets.FinishDriven)

	plugins := in.Resolved
	if driven {
		// Driven semantics: the `complete` tool is filtered from every resolved
		// plugin's catalog (the LLM never sees it) and the keepalive nudge keeps
		// the loop alive on text-only turns until an external driver stops it.
		// The conditional plugins added below have no `complete` tool, so only
		// the resolved list needs wrapping.
		wrapped := make([]api.AgentPlugin, len(plugins))
		for i, p := range plugins {
			wrapped[i] = newChatFilterWrapper(p)
		}
		plugins = append(wrapped, keepalive.NewPlugin())
	}

	// Channel-specific system-prompt block so the LLM frames its asks
	// for the resolved UserComms channel. NewPlugin returns nil for
	// empty / unrecognized values.
	if uca := user_comms_adapter.NewPlugin(req.UserCommsInput); uca != nil {
		plugins = append(plugins, uca)
	}

	// Built-ins (previously Go-wired inside NewAgentLoopMachine).
	plugins = append(plugins, runtime.NewOutputRecoveryPlugin(), runtime.NewAbortCleanupPlugin())
	if in.TraceWriter != nil {
		plugins = append(plugins, runtime.NewDebugPlugin(in.TraceWriter))
	}
	if mf := runtime.NewModelFallbackPlugin(in.FallbackModel); mf != nil {
		plugins = append(plugins, mf)
	}

	// Auto-compaction: once the context crosses the trigger (a fraction of the
	// model's real context window), summarize older turns into one message and
	// keep a verbatim recent tail (Claude Code's compact strategy). Policy comes
	// from the profile's `compaction:` block (LLMProfiles.yaml). Short runs
	// never cross the threshold, so it is a no-op for them.
	if in.CompactionEnabled {
		if cp := compaction.NewPlugin(in.Model, in.Compaction); cp != nil {
			plugins = append(plugins, cp)
		}
	}

	ps := runtime.NewPluginSet()
	for _, p := range plugins {
		ps.Add(p)
	}

	// Loop-safety guard: denies a tool call repeated with identical
	// arguments past its threshold. Attribute-driven — the closure
	// reads Polling / Idle from the live ToolDefs of the assembled set,
	// so the plugin never knows tool names. A driven (keepalive) run exempts
	// idle tools (ask-user's "wait for the user" repeats by design).
	ps.Add(loopsafety.NewPlugin(driven, func(toolName string) (bool, bool, bool) {
		for _, p := range ps.Plugins() {
			for _, def := range p.ToolDefs() {
				if def.Name == toolName {
					return def.Polling, def.Idle, def.Blocking
				}
			}
		}
		return false, false, false
	}))

	// Per-spawn guards (parent-controlled; run in OnPreToolExec before
	// the user-curated permission gate).
	runtime.AttachSpawnPermissions(ps, req.SpawnPermissions)
	runtime.AttachWorkspacePaths(ps, req.WorkspacePaths)

	return ps
}
