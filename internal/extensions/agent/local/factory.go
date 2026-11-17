// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/assembly"
	iplugin "github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/plugin"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/plugin/compaction"
	iprofile "github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/profile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/runtime"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	ktspengine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// loadPluginsConfigLayered loads PluginsConfig with APPEND semantics across
// the global + per-universe ConfigService chain.
//
// Per the universes-and-config-scoping ADR's resolution rules table,
// Plugins.yaml uses APPEND (not REPLACE): the universe's plugin list extends
// the global baseline rather than replacing it. The Defaults + Plugins
// slices are concatenated layer-by-layer (parent first, child last) so a
// universe author who wants to add ONE plugin doesn't have to copy the
// entire global list into their universe's Plugins.yaml.
//
// Returns the merged PluginsConfig. If no Application in context or no
// Plugins config in any layer, returns the zero value (no error — a no-op).
func loadPluginsConfigLayered(ctx context.Context) (api.PluginsConfig, error) {
	var merged api.PluginsConfig
	cfgSvc, err := application.UseService[*config.ConfigService](ctx)
	if err != nil || cfgSvc == nil {
		// Fallback to single-layer UseConfig — preserves behavior in test
		// harnesses that don't wire a full ConfigService chain.
		return application.UseConfig[api.PluginsConfig](ctx)
	}
	layers := cfgSvc.GetMapLayered(config.ConfigName[api.PluginsConfig]())
	for _, layer := range layers {
		var local api.PluginsConfig
		if err := config.DecodeMap(layer, &local); err != nil {
			return merged, fmt.Errorf("decode Plugins layer: %w", err)
		}
		merged.Defaults = append(merged.Defaults, local.Defaults...)
		merged.Plugins = append(merged.Plugins, local.Plugins...)
	}
	return merged, nil
}

// AgentMachineFactory creates AgentLoopMachine instances from AgentRunRequest.
// Resolves all runtime deps (profile, model, plugins) from context via UseService[T].
// Same code for fresh start and restart from checkpoint.
type AgentMachineFactory struct {
	profileMgr iprofile.LLMProfiles
	pluginMgr  iplugin.PluginManager
}

// NewAgentMachineFactory creates a factory with injected managers.
func NewAgentMachineFactory(profileMgr iprofile.LLMProfiles, pluginMgr iplugin.PluginManager) *AgentMachineFactory {
	return &AgentMachineFactory{profileMgr: profileMgr, pluginMgr: pluginMgr}
}

func (f *AgentMachineFactory) Name() string { return "agent-loop" }

// CreateMachine creates an AgentLoopMachine from a serializable request.
// ctx carries Application (via WithApplication) and all DI services (via UseService).
func (f *AgentMachineFactory) CreateMachine(ctx context.Context, request any) (ktspengine.Machine, error) {
	req, err := f.resolveRequest(request)
	if err != nil {
		return nil, fmt.Errorf("agent factory: %w", err)
	}

	// Fold the `skills:` / `tools:` sugar lists into Plugins as skill:/tool:
	// refs before plugin resolution reads req.Plugins below.
	req.DesugarPluginLists()

	// Validate HistoryMode dimension. summary mode requires a summarizer
	// agent that doesn't exist yet — fail fast rather than silently ignore.
	if req.HistoryMode == "summary" {
		return nil, fmt.Errorf("agent factory: history mode 'summary' is not yet implemented")
	}
	// Isolation outside this process (a terminal pane, a remote host) is not
	// built: refused, never run in-process as if it were isolated.
	if req.IsolationMode == "pane" || req.IsolationMode == "remote" {
		return nil, fmt.Errorf("agent factory: isolation mode %q is not built — the child would run in this process", req.IsolationMode)
	}
	// Consistency check: full_fork should have InitialMessages.
	if req.HistoryMode == "full_fork" && len(req.InitialMessages) == 0 {
		return nil, fmt.Errorf("agent factory: history mode 'full_fork' requires InitialMessages to be set")
	}
	// Validate PermissionMode. "bubble" mode is documented as a deny-route-
	// to-parent flow but is currently a no-op in the hook (R4 step 5 falls
	// through to allow). Until the ProtocolPermission round-trip lands as
	// a tier-2 follow-up, reject the mode at attach time so a parent that
	// thinks it has set bubble mode fails LOUD instead of silently granting
	// the child full access (R9 — security medium-finding-2).
	if req.SpawnPermissions != nil && req.SpawnPermissions.PermissionMode == "bubble" {
		return nil, fmt.Errorf("agent factory: PermissionMode 'bubble' is not yet implemented (tier-2); use 'restricted' for safe-subset or set AllowedTools + DisallowedTools explicitly")
	}

	// Resolve profile
	profile, err := f.profileMgr.ResolveProfile(req.ProfileName)
	if err != nil {
		return nil, fmt.Errorf("resolve profile: %w", err)
	}
	if req.MaxTurns > 0 {
		profile.MaxTurns = api.IntPtr(req.MaxTurns)
	}

	// Create model
	model, err := f.profileMgr.CreateModelFromProfile(profile, req.Overrides)
	if err != nil {
		// A model that cannot be created - its key not set - says why in
		// the person's language, as a failed call does.
		return nil, fmt.Errorf("create model: %w", runtime.ModelCallError(err, profile.Model.Name))
	}

	// Plugin refs from request; defaults from PluginsConfig.
	//
	// PluginsConfig uses APPEND semantics across the global + per-universe
	// config layers per the universes-and-config-scoping ADR — global plugin
	// list is the baseline, the universe extends it. Order: global first,
	// universe second (later entries can override earlier per existing
	// plugin precedence semantics).
	pluginRefs := withObjectContext(req.Plugins, req.Context, f.pluginMgr.Has)
	var defaults []string
	if pluginsCfg, err := loadPluginsConfigLayered(ctx); err == nil {
		defaults = pluginsCfg.Defaults
	}

	// Build session context (values in context.Context). Enrich the
	// plugin-context envelope with the delegation identity so the "agent as
	// human" ask-user channel (bubble_to_parent / via_agent) can route a
	// child's question UP to its parent run.
	pctx := req.Context
	if req.ParentRunID != "" {
		pctx = map[string]any{}
		for k, v := range req.Context {
			pctx[k] = v
		}
		pctx["_parent_run_id"] = req.ParentRunID
		// Store the enriched envelope back on the request itself: the machine's
		// per-turn withSessionValues re-injects req.Context onto the tool ctx,
		// and the bubble_to_parent ask-user channel reads _parent_run_id at
		// tool-execution time (not plugin-creation time). Without this, the
		// child's question can't find its parent run.
		req.Context = pctx
	}
	sessionCtx := api.WithPluginContext(ctx, pctx)
	sessionCtx = api.WithVariables(sessionCtx, req.Variables)
	if req.Mode != "" {
		sessionCtx = api.WithAgentMode(sessionCtx, req.Mode)
	}
	// Publish the resolved UserComms.Input into ctx so tools
	// (ask-user / notify-user) can pick the right channel
	// implementation at execute time. The resolver in
	// resolveFromExecRequest already finalized this value via
	// EffectiveUserCommsInput; here we just propagate it.
	sessionCtx = api.WithUserCommsInput(sessionCtx, req.UserCommsInput)

	// Build LoopContext — shared state for plugins. Messages are synced from
	// agentState at step boundaries (checkpoint-safe).
	loop := runtime.NewLoopContext()
	loop.TraceWriter = api.TraceWriterFrom(ctx)
	loop.SetRunMode(req.Mode)
	// Record the active permission profile (injected onto ctx by TaskService.Run
	// from the task-declared / propagated profile) so the delegate plugin can
	// pass it down to the workers this run spawns — children run on the bg app
	// context and cannot inherit the profile via ctx.
	loop.SetPermissionProfile(runtime.PermissionProfileFrom(ctx))
	// Abort-signal wiring depends on isolation mode.
	// - Default / in_process: child's abort closes when parent context cancels (cascade)
	// - context_isolated: child's abort is independent — parent cancel does NOT close it.
	//   The child is still a goroutine in the same process, but its abort signal has
	//   its own lifecycle (closed only when the child terminates or is explicitly killed).
	if req.IsolationMode != "context_isolated" {
		go func() {
			<-ctx.Done()
			loop.CloseAbort()
		}()
	}

	// Instantiate plugins
	plugins, err := f.pluginMgr.CreatePlugins(profile.Plugins, pluginRefs, defaults, sessionCtx, loop)
	if err != nil {
		return nil, fmt.Errorf("create plugins: %w", err)
	}

	// Assembly: the single place that maps the run's requirements to
	// the complete plugin list — chat semantics, user-comms adapter,
	// built-ins (recovery / abort-cleanup / debug / model-fallback),
	// loop safety, and the per-spawn guards. See agent/internal/assembly.
	compactionCfg, compactionOn := resolveCompaction(profile.Compaction)
	ps := assembly.Build(assembly.Inputs{
		Request:           req,
		Resolved:          plugins,
		Model:             model,
		Compaction:        compactionCfg,
		CompactionEnabled: compactionOn,
		TraceWriter:       loop.TraceWriter,
		FallbackModel:     profile.FallbackModel,
	})

	// I3: wire the bus-backed audit emitter once per process. Hooks publish
	// permission denials on engine.ChildPermissionDeniedTopic via the
	// runtime.AuditEmitter indirection; the adapter here bridges the
	// engine-neutral runtime event to the engine topic. Idempotent.
	wireAuditEmitter(ctx)

	// Connect plugin lifecycle
	if err := ps.ConnectAll(ctx); err != nil {
		return nil, fmt.Errorf("connect plugins: %w", err)
	}

	m := runtime.NewAgentLoopMachine(model, req.Prompt, profile.SystemPromptPrefix, ps, loop)
	m.SetRunRequest(req)
	// Capture the shared engine Channel from the creation ctx so each per-turn
	// tool-dispatch ctx carries it — the comms service emits the child.<id>
	// wake events on it (a parent's await-child reacts instantly, not on timeout).
	m.SetChannel(ktspengine.ChannelFrom(ctx))

	// Per-session model-call observer (llm-model-calls.jsonl):
	// structured call_start / sse_event / call_end events for every
	// model call, enabled by the profile's log_model_calls (or the
	// LLMProfiles.yaml global default). The loop passes the observer to
	// the model on each call and closes it at the terminal chokepoint.
	if req.SessionID != "" && f.profileMgr.ModelCallLogging(profile) {
		logPath := filepath.Join(
			application.Env().SessionsDir(), req.SessionID, "llm-model-calls.jsonl")
		m.SetModelCallObserver(runtime.NewModelCallLogObserver(logPath))
	}
	// A tool result over its MaxResultSize is kept whole in the session.
	if req.SessionID != "" {
		m.SetResultDir(filepath.Join(application.Env().SessionsDir(), req.SessionID, "tool-results"))
	}

	return m, nil
}

// resolveCompaction translates a profile's `compaction:` block into the
// plugin's Config plus whether it is enabled. A nil block (profile omits it)
// means enabled with the plugin's built-in fallback ratios; a block with
// `enabled: false` turns it off. The ratios are passed straight through —
// zero-valued fields fall back inside compaction.Config.withDefaults, so the
// authoritative values live in LLMProfiles.yaml, not here.
func resolveCompaction(cc *iprofile.CompactionConfig) (compaction.Config, bool) {
	if cc == nil {
		return compaction.Config{}, true
	}
	if cc.Enabled != nil && !*cc.Enabled {
		return compaction.Config{}, false
	}
	return compaction.Config{
		TriggerRatio:     cc.TriggerRatio,
		KeepRecentRatio:  cc.KeepRecentRatio,
		SummaryMaxTokens: cc.SummaryMaxTokens,
	}, true
}

// resolveRequest converts the incoming request to *AgentRunRequest.
// Accepts *TaskExecRequest (from TaskService) or *AgentRunRequest (direct API).
func (f *AgentMachineFactory) resolveRequest(request any) (*api.AgentRunRequest, error) {
	switch r := request.(type) {
	case *taskapi.TaskExecRequest:
		return f.resolveFromExecRequest(r)
	case *api.AgentRunRequest:
		return r, nil
	default:
		return nil, fmt.Errorf("expected *TaskExecRequest or *AgentRunRequest, got %T", request)
	}
}

// isAttended reports whether the run was launched interactively (the user is
// sitting in the GUI run/chat surface), signaled by the reserved `_attended`
// plugin-context key the GUI sets on every user-initiated launch. Attended
// runs route ask-user to the inline `direct` channel instead of the Inbox.
func isAttended(pluginContext map[string]any) bool {
	if pluginContext == nil {
		return false
	}
	v, ok := pluginContext["_attended"]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// resolveFromExecRequest builds AgentRunRequest from the separated request.
// Config is decoded into AgentRunRequest fields. Input is propagated as context.
// Body is already injected into config as "prompt" by the framework.
func (f *AgentMachineFactory) resolveFromExecRequest(er *taskapi.TaskExecRequest) (*api.AgentRunRequest, error) {
	configMap, _ := er.Config.(map[string]any)
	if configMap == nil {
		configMap = make(map[string]any)
	}

	data, err := json.Marshal(configMap)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	var req api.AgentRunRequest
	if err := decodeAgentConfig(data, &req); err != nil {
		return nil, err
	}

	// Prompt comes from config (injected by framework via BodyFieldName -> "prompt").
	if req.Prompt == "" {
		if p, ok := configMap[presets.ConfigKeyPrompt].(string); ok {
			req.Prompt = p
		}
	}

	// Profile comes from TASK.md config as `capability:` — a yaml-only
	// key the JSON round-trip above cannot map onto ProfileName
	// (json tag is "profileName"). Read it explicitly, like prompt.
	if req.ProfileName == "" {
		if c, ok := configMap[presets.ConfigKeyCapability].(string); ok {
			req.ProfileName = c
		}
	}

	// Runtime context.
	req.WorkspaceRoot = er.Runtime.WorkspaceRoot
	req.ParentRunID = er.Runtime.ParentRunID
	req.SessionID = er.Runtime.SessionID

	// Forward the task's output contract to the plugin context so the
	// `complete` tool advertises the REAL output schema to the LLM
	// (reserved `_`-prefixed substrate key, like `_target_*`).
	if len(er.Runtime.OutputSchema) > 0 {
		if req.Context == nil {
			req.Context = make(map[string]any)
		}
		req.Context["_output_schema"] = string(er.Runtime.OutputSchema)
	}

	// Top-level run policy: when there is no parent AND the caller did not
	// declare any SpawnPermissions, treat this as an interactive user run and
	// apply AllowAll. The user-curated permission profile is the next layer
	// of confirmation. Spawn paths (delegate/workflow children) always carry
	// a ParentRunID and inherit perms from their mode preset via
	// configOverrideFromModeWithParent, so they are unaffected. Programmatic
	// consumers wanting strict fail-closed behavior must construct an explicit
	// SpawnPermissions{} (zero-value) in their request.
	if req.SpawnPermissions == nil && req.ParentRunID == "" {
		req.SpawnPermissions = &api.SpawnPermissions{AllowAll: true}
	}

	// Propagate input as plugin context.
	if len(er.Input) > 0 {
		if req.Context == nil {
			req.Context = make(map[string]any)
		}
		if userInput, ok := er.Input["user_input"].(string); ok && userInput != "" {
			req.Context["user_input"] = userInput
		} else {
			inputData, err := json.Marshal(er.Input)
			if err == nil && len(er.Input) > 0 {
				req.Context["user_input"] = string(inputData)
			}
		}
	}

	// Finalize UserCommsInput via the auto-detection rule.
	// After unmarshal, req.UserCommsInput
	// carries either (a) the TASK.md frontmatter value, (b) the preset
	// default emitted by BuildChildOverride for spawn paths, or
	// (c) empty for top-level task runs whose frontmatter didn't set
	// it. Empty → derive from the mode's preset default (chat → direct;
	// system-spawned → inbox; teammate → via_leader; remote → none).
	// Non-empty → trust the upstream value (explicit operator/preset
	// signal wins).
	if req.UserCommsInput == "" {
		presetDefault := presets.InputInbox
		if req.Mode != "" {
			if cfg, err := presets.ResolvePreset(presets.Mode(req.Mode), nil); err == nil && cfg.UserComms.Input != "" {
				presetDefault = cfg.UserComms.Input
			}
		}
		// Attended launch (the user is sitting in the GUI run/chat surface,
		// flagged via the `_attended` context key): asks should resolve to
		// the inline `direct` channel rather than the system-spawned Inbox.
		// Only when the mode preset hasn't already chosen a specific channel
		// (so mode:chat stays direct, a teammate stays via_leader, etc.); a
		// task's explicit `user_comms_input:` already short-circuited above.
		if presetDefault == presets.InputInbox && isAttended(req.Context) {
			presetDefault = presets.InputDirect
		}
		req.UserCommsInput = string(presets.EffectiveUserCommsInput(presetDefault, ""))
	}

	// Resolve the loop-termination mode the same way: an explicit launch value
	// (delegate `finish:` / TASK.md) wins; otherwise derive from the mode's
	// preset (chat → driven, everything else → self). The assembly reads this to
	// pick the loop terminator (keepalive + complete-hide vs self-complete).
	if req.Finish == "" {
		var presetFinish presets.FinishMode
		if req.Mode != "" {
			if cfg, err := presets.ResolvePreset(presets.Mode(req.Mode), nil); err == nil {
				presetFinish = cfg.Finish
			}
		}
		req.Finish = string(presets.EffectiveFinish(presetFinish, ""))
	}

	// Diagnostic: the resolved ask-user channel + the mode it derived from.
	// A chat-launched run must show mode=chat / userCommsInput=direct here;
	// an inbox value on a chat launch pinpoints where mode:chat was lost.
	log.Printf("[usercomms] resolved mode=%q userCommsInput=%q parent=%q session=%q",
		req.Mode, req.UserCommsInput, req.ParentRunID, req.SessionID)

	return &req, nil
}

// ── SchemaProvider ─────────────────────────────────────────────────────

func (f *AgentMachineFactory) DefaultInputSchema() json.RawMessage {
	return json.RawMessage(`{"name":"agent-input","root":"AgentInput","types":{"AgentInput":{"type":"object","fields":{"user_input":{"type":"string","description":"Initial message or instruction for the agent","optional":true}}}}}`)
}

func (f *AgentMachineFactory) DefaultOutputSchema() json.RawMessage { return nil }

func (f *AgentMachineFactory) BodyFieldName() string { return "prompt" }

// Ensure AgentMachineFactory implements MachineFactory and SchemaProvider.
var _ ktspengine.MachineFactory = (*AgentMachineFactory)(nil)
var _ ktspengine.SchemaProvider = (*AgentMachineFactory)(nil)

// decodeAgentConfig decodes a TASK.md `config:` block onto an AgentRunRequest
// and REFUSES a key the request does not have. Until 2026-09-22 the decode
// was lenient: `max_turns: 3` (the json tag is `maxTurns`), `permission_mode`
// (no such field) and any misspelling were dropped without a word, so a task
// ran with defaults its author had overridden on paper. Keys starting with
// `_` are substrate-reserved (`_target_*`, `_mode`) and pass through; the
// yaml-only keys are mapped by hand after this.
func decodeAgentConfig(data []byte, req *api.AgentRunRequest) error {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("unmarshal to AgentRunRequest: %w", err)
	}
	filtered := make(map[string]json.RawMessage, len(probe))
	for k, v := range probe {
		if api.IsPassThroughConfigKey(k) {
			continue
		}
		filtered[k] = v
	}
	// Strict at the TOP level only: nested override objects (workspacePaths,
	// spawnPermissions) carry spawn-path keys the request's structs do not
	// model, and encoding/json's DisallowUnknownFields would refuse them.
	keys := make([]string, 0, len(filtered))
	for k := range filtered {
		keys = append(keys, k)
	}
	unknown := api.UnknownConfigKeys(keys)
	if len(unknown) > 0 {
		return fmt.Errorf("task config: unknown key(s) %q — not keys of an agent task's config (the request's json tags are the contract: maxTurns, mode, plugins, skills, tools, …)", unknown)
	}
	clean, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := json.Unmarshal(clean, req); err != nil {
		return fmt.Errorf("unmarshal to AgentRunRequest: %w", err)
	}
	return nil
}

// withObjectContext attaches the object-context plugin to a run on an object:
// a run with a target receives that object's context whatever plugins its task
// lists, when this binary registers the plugin. The refs are never changed in
// place.
func withObjectContext(refs []api.PluginRef, pluginContext map[string]any, has func(string) bool) []api.PluginRef {
	target, _ := pluginContext[api.PluginContextTarget].(string)
	if target == "" || !has(api.ObjectContextPlugin) {
		return refs
	}
	for _, r := range refs {
		if r.Type == api.ObjectContextPlugin {
			return refs
		}
	}
	return append(append([]api.PluginRef(nil), refs...), api.PluginRef{Type: api.ObjectContextPlugin})
}
