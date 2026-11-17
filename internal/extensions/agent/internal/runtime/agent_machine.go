// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// AgentLoopMachine implements machine.Machine for the LLM agent loop.
// States: init → thinking → tool_dispatch → (loop) → complete/failed
type AgentLoopMachine struct {
	// warn warns on the run (engine.RunWarnerFrom of its step's context):
	// what goes wrong around a step while the run goes on.
	warn            func(reason string)
	model           llm.Model
	plugins         *PluginSet
	hooks           *HookChain
	loop            *loopContextImpl // concrete type for sync methods
	systemPromptPfx string           // static prefix from profile
	taskBody        string
	runRequest      *api.AgentRunRequest // resolved request from factory (may be nil)

	// channel is the shared engine Channel, captured at machine-creation time
	// (where the ctx carries it) and re-injected onto every per-turn
	// tool-dispatch ctx by withSessionValues. Without this, a tool's
	// ChannelFrom(ctx) is nil and the comms service can't emit the wake events
	// (child.<id>.suspended) that let a parent's await-child react instantly.
	channel *machine.Channel

	// modelCallObserver, when non-nil, receives structured events for
	// every model call (the per-session llm-model-calls.jsonl sink).
	// The loop passes it to the model via llm.WithCallObserver and
	// closes it at the terminal chokepoint.
	modelCallObserver llm.ModelCallObserver

	// resultDir, when set, is where a tool result over its MaxResultSize is
	// kept whole (the run's session folder's tool-results/); the model gets
	// its preview (capToolResult).
	resultDir string

	// Cache: last system message and tool list hashes for LLM cache optimization
	lastSysMsgHash [32]byte
	lastToolsHash  [32]byte
	cacheHits      int
}

// NewAgentLoopMachine creates a machine from resolved config, plugin set, and loop context.
// Sorts plugins stable-first for LLM cache optimization (stable prefix, dynamic suffix).
//
// The loop is pure mechanism: it instantiates ZERO plugins. The
// complete set — including built-ins like output-recovery,
// abort-cleanup, debug-on-trace, and model-fallback — arrives finished
// from the assembly step (agent/internal/assembly).
func NewAgentLoopMachine(model llm.Model, taskBody, systemPromptPfx string, plugins *PluginSet, loop *loopContextImpl) *AgentLoopMachine {
	plugins.SortStableFirst()
	hooks := plugins.BuildHooks()

	return &AgentLoopMachine{
		model:           model,
		plugins:         plugins,
		hooks:           hooks,
		loop:            loop,
		systemPromptPfx: systemPromptPfx,
		taskBody:        taskBody,
	}
}

func (m *AgentLoopMachine) Name() string { return "agent-loop" }

// StateLayout names the layout of the agentState it saves: bump it when that
// shape changes, so a run saved by an older build is reported interrupted
// rather than read half (engine.StateLayouter).
func (m *AgentLoopMachine) StateLayout() string { return "agent-loop/1" }

// RestartsAfterCrash: an agent working when its process died starts again
// from its last saved step — a model call is made again; tool calls in flight
// are answered "interrupted by a restart", never run again blindly
// (engine.Restartable).
func (m *AgentLoopMachine) RestartsAfterCrash() bool { return true }

// SetRunRequest stores the resolved AgentRunRequest so InitialState can use it
// for context (user_input, initial messages, max turns) instead of relying on
// the event payload type assertion.
func (m *AgentLoopMachine) SetRunRequest(req *api.AgentRunRequest) {
	m.runRequest = req
}

// SetChannel captures the shared engine Channel (resolved from the
// machine-creation ctx) so withSessionValues can re-inject it onto each
// per-turn tool-dispatch ctx. Safe to pass nil (root runs without an engine
// channel — the comms wake is simply skipped).
func (m *AgentLoopMachine) SetChannel(ch *machine.Channel) {
	m.channel = ch
}

// SetModelCallObserver attaches the per-session model-call observer.
// Set by the factory when the resolved profile enables log_model_calls.
func (m *AgentLoopMachine) SetModelCallObserver(o llm.ModelCallObserver) {
	m.modelCallObserver = o
}

// SetResultDir sets the folder a tool result over its limit is kept in.
func (m *AgentLoopMachine) SetResultDir(dir string) {
	m.resultDir = dir
}

// withSessionValues re-applies the session-scoped context values from
// the run request onto the given (executor-driven) context. The agent
// factory sets these on the plugin-creation context, but tools execute
// with the executor's runtime context — without this re-injection they
// can't read the resolved UserComms channel, the plugin-context
// envelope, the resolved variables, or the delegation mode. Returns the
// input ctx unchanged when no run request is set.
func (m *AgentLoopMachine) withSessionValues(ctx context.Context) context.Context {
	if m.runRequest == nil {
		return ctx
	}
	ctx = api.WithUserCommsInput(ctx, m.runRequest.UserCommsInput)
	// Stamp the run id so tool-emitted events (ask-user prompt_shown /
	// user_input) carry the emitter the per-session translator filters on —
	// and the run as the CAUSE of every write its tools make (the object
	// events and journal records say which run changed what), at the depth
	// the run was started at (a run a reaction started is one level deeper).
	if rid := m.loop.RunID(); rid != "" {
		ctx = api.WithRunID(ctx, rid)
		if current, ok := cause.CauseFrom(ctx); !ok || current.Kind != cause.KindReaction {
			ctx = cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: rid, Depth: cause.DepthFrom(ctx)})
		}
	}
	if m.runRequest.Context != nil {
		ctx = api.WithPluginContext(ctx, m.runRequest.Context)
	}
	if m.runRequest.Variables != nil {
		ctx = api.WithVariables(ctx, m.runRequest.Variables)
	}
	if m.runRequest.Mode != "" {
		ctx = api.WithAgentMode(ctx, m.runRequest.Mode)
	}
	// Re-inject the shared engine Channel so a tool's ChannelFrom(ctx) resolves
	// it — the comms service needs it to emit the wake events that let a parent's
	// await-child react instantly instead of polling to a timeout.
	if m.channel != nil {
		ctx = machine.WithChannel(ctx, m.channel)
	}
	return ctx
}

// agentState is the mutable conversation state held in State.Data.
type agentState struct {
	TaskName   string        `json:"taskName"`
	Messages   []api.Message `json:"messages"`
	Turn       int           `json:"turn"`
	MaxTurns   int           `json:"maxTurns"`
	TotalUsage api.Usage     `json:"totalUsage"`
	// MaxBudgetUSD is the run's cost ceiling (0 = none): no model call is made
	// once its cost has reached it.
	MaxBudgetUSD float64   `json:"maxBudgetUSD,omitempty"`
	StartedAt    time.Time `json:"startedAt"`

	// Last LLM response (set by thinking, consumed by tool_dispatch)
	LastResponse *llm.ContentResponse `json:"lastResponse,omitempty"`
	// Last LLM call timing
	LastLLMDurationMs int64 `json:"lastLlmDurationMs,omitempty"`
	LastTTFTMs        int64 `json:"lastTtftMs,omitempty"`

	// StructuredOutput is the task's declared output envelope (the complete
	// tool's full input for an output-schema task). When set, finish makes it the
	// machine StepResult.Output so a programmatic consumer reads task.output as
	// the structured map instead of the run-summary struct.
	StructuredOutput map[string]any `json:"structuredOutput,omitempty"`

	// Batch is a tool batch stopped on a call that awaits (a tool returned
	// api.ErrAwait): the results of the calls before it, which call it is, and
	// what the tool needs to check again. Set in the `awaiting` state, saved
	// with the run; nil otherwise.
	Batch *toolBatch `json:"batch,omitempty"`
}

// toolBatch is a tool batch in progress: LastResponse holds its calls.
type toolBatch struct {
	Results    []llm.ToolCallResponse `json:"results,omitempty"`
	APIResults []api.ToolResult       `json:"apiResults,omitempty"`
	Complete   *api.ErrRunComplete    `json:"complete,omitempty"`
	// Next is the index of the call that awaits; the calls after it have not
	// run yet.
	Next int `json:"next"`
	// Input is that call's input as the permission gate let it through — its
	// JSON text, kept as a string so it reads back exactly after a restart (a
	// raw JSON value would be re-encoded from a decoded map: numbers past
	// 2^53 changed, keys reordered).
	Input string `json:"input,omitempty"`
	// Resume is what the tool returned to check again (api.ErrAwait.Resume).
	Resume map[string]any `json:"resume,omitempty"`
}

func (s *agentState) accumulateUsage(u api.Usage) {
	s.TotalUsage.InputTokens += u.InputTokens
	s.TotalUsage.OutputTokens += u.OutputTokens
	s.TotalUsage.CacheReadInputTokens += u.CacheReadInputTokens
	s.TotalUsage.CacheCreationInputTokens += u.CacheCreationInputTokens
	// All turns of a run share one model — carry its identity + accumulate the
	// per-call cost into the run total so session_end reports real spend.
	if u.Model != "" {
		s.TotalUsage.Model = u.Model
		s.TotalUsage.Provider = u.Provider
	}
	if u.Cost != nil {
		if s.TotalUsage.Cost == nil {
			s.TotalUsage.Cost = &llm.Cost{}
		}
		s.TotalUsage.Cost.InputUSD += u.Cost.InputUSD
		s.TotalUsage.Cost.OutputUSD += u.Cost.OutputUSD
		s.TotalUsage.Cost.CacheReadUSD += u.Cost.CacheReadUSD
		s.TotalUsage.Cost.CacheWriteUSD += u.Cost.CacheWriteUSD
		s.TotalUsage.Cost.TotalUSD += u.Cost.TotalUSD
	}
}

// spentUSD is what the run's model calls have cost so far.
func (s *agentState) spentUSD() float64 {
	if s.TotalUsage.Cost == nil {
		return 0
	}
	return s.TotalUsage.Cost.TotalUSD
}

func (s *agentState) makeResult(status api.RunStatus, summary string, files []string, err error) *api.RunResult {
	return &api.RunResult{
		Status:   status,
		Summary:  summary,
		Files:    files,
		Turns:    s.Turn,
		Usage:    s.TotalUsage,
		Duration: time.Since(s.StartedAt),
		Error:    err,
	}
}

func (m *AgentLoopMachine) InitialState(event machine.Event) machine.State {
	// Use the resolved request from the factory if available.
	// Fall back to event payload for backward compatibility.
	req := m.runRequest
	if req == nil {
		req, _ = event.Payload.(*api.AgentRunRequest)
	}
	taskName := ""
	maxTurns := 50
	budget := 0.0
	if req != nil {
		taskName = req.Name
		if req.MaxTurns > 0 {
			maxTurns = req.MaxTurns
		}
		budget = req.MaxBudgetUSD
	}

	// Build initial messages:
	// 1. Fork mode: use pre-populated conversation history (InitialMessages)
	// 2. Normal: single user message with user_input or default
	var messages []api.Message
	if req != nil && len(req.InitialMessages) > 0 {
		messages = req.InitialMessages
	} else {
		initialMsg := "Please start the task."
		if req != nil {
			if userInput, ok := req.Context["user_input"].(string); ok && userInput != "" {
				initialMsg = userInput
			}
		}
		messages = []api.Message{api.NewTextMessage(api.RoleUser, initialMsg)}
	}

	as := &agentState{
		TaskName:     taskName,
		Messages:     messages,
		Turn:         0,
		MaxTurns:     maxTurns,
		MaxBudgetUSD: budget,
		StartedAt:    time.Now(),
	}

	return machine.State{Name: "init", Data: as}
}

// Step dispatches based on state name.
// Syncs messages between agentState (checkpoint) and LoopContext (plugin view).
func (m *AgentLoopMachine) Step(ctx context.Context, state *machine.State, event machine.Event) (machine.ActionFunc, error) {
	as, err := m.ensureTyped(state)
	if err != nil {
		return nil, err
	}
	m.warn = machine.RunWarnerFrom(ctx)

	// Bind loop context to agentState so plugins see live messages.
	// Both the machine and plugins read/write as.Messages through the same pointer.
	m.loop.Bind(as)

	// Expose this run's ID to plugins via loop meta. The delegate plugin uses
	// this as the parent_run_id when spawning child tasks, so reports can
	// reconstruct the delegation tree.
	if state.RunID != "" {
		m.loop.SetMeta("_runId", state.RunID)
	}

	// Expose this run's delegation mode so delegate/messaging plugins can
	// adapt spawning behavior (e.g., coordinator restricts worker tools).
	if m.runRequest != nil && m.runRequest.Mode != "" {
		m.loop.SetMeta("_mode", m.runRequest.Mode)
	}

	// Expose teammate identity so the orchestration plugin can auto-subscribe
	// to the agent's own direct and team broadcast topics.
	if m.runRequest != nil && m.runRequest.TeammateName != "" {
		m.loop.SetMeta("_teammate_name", m.runRequest.TeammateName)
	}
	if m.runRequest != nil && m.runRequest.TeamName != "" {
		m.loop.SetMeta("_team_name", m.runRequest.TeamName)
	}

	switch state.Name {
	case "init":
		return m.stepInit(ctx, as)
	case "thinking":
		return m.stepThinking(ctx, as)
	case "tool_dispatch", "awaiting":
		if event.Type == machine.RestartedEvent && state.Name == "tool_dispatch" {
			return m.stepInterruptedBatch(as)
		}
		// awaiting: the batch stopped on a call that awaits; its event came,
		// or the run was restored after a restart — the call checks again.
		// Replayed after a crash, the calls after it may have run: they are
		// answered interrupted, never run again.
		return m.stepToolDispatch(ctx, as, event.Replayed)
	default:
		return nil, fmt.Errorf("agent-loop: unknown state %q", state.Name)
	}
}

// stepInit: emit session_start, transition to thinking
func (m *AgentLoopMachine) stepInit(_ context.Context, as *agentState) (machine.ActionFunc, error) {
	return func(ctx context.Context) (*machine.StepResult, error) {
		info := m.model.Info()
		evt := api.SessionStartEvent{
			Name:          as.TaskName,
			Model:         info.ModelID,
			Provider:      info.Provider,
			ContextWindow: info.ContextWindow,
		}
		if m.runRequest != nil {
			evt.ParentRunID = m.runRequest.ParentRunID
			evt.Mode = m.runRequest.Mode
		}
		return &machine.StepResult{
			NextState: "thinking",
			Data:      as,
			Emit: []machine.Event{{
				Type:    api.EventSessionStart,
				Payload: evt,
			}},
		}, nil
	}, nil
}

// buildSystemMessage assembles the system prompt from all plugins (per-turn, dynamic).
// Tracks hash for LLM cache optimization — unchanged messages maximize cache hits.
func (m *AgentLoopMachine) buildSystemMessage() string {
	pluginMsg := m.plugins.BuildSystemMessage()
	sysMsg := pluginMsg + "\n# Task\n\n" + m.taskBody
	if m.systemPromptPfx != "" {
		sysMsg = m.systemPromptPfx + "\n\n" + sysMsg
	}
	m.lastSysMsgHash = sha256.Sum256([]byte(sysMsg))
	return sysMsg
}

// buildToolDefs collects tool definitions from all plugins (per-turn, dynamic).
// Tracks hash for LLM cache optimization.
func (m *AgentLoopMachine) buildToolDefs() []api.ToolDef {
	var defs []api.ToolDef
	seen := make(map[string]bool)
	var hashBuf strings.Builder
	for _, p := range m.plugins.Plugins() {
		for _, def := range p.ToolDefs() {
			if !seen[def.Name] {
				defs = append(defs, def)
				seen[def.Name] = true
				hashBuf.WriteString(def.Name)
				hashBuf.WriteByte(0)
			}
		}
	}
	m.lastToolsHash = sha256.Sum256([]byte(hashBuf.String()))
	return defs
}

// buildDispatcher creates a fresh dispatcher from current plugin tools.
func (m *AgentLoopMachine) buildDispatcher() *Dispatcher {
	return m.plugins.BuildDispatcher()
}

// checkAbort checks if the context is cancelled and fires H10: OnAbort hooks.
// Returns a StepResult for abort, or nil to continue.
func (m *AgentLoopMachine) checkAbort(ctx context.Context, as *agentState) *machine.StepResult {
	select {
	case <-ctx.Done():
	default:
		return nil
	}
	// Context cancelled — fire OnAbort hooks
	for _, hook := range m.hooks.OnAbort {
		_ = hook(m.loop)
	}
	result := as.makeResult(api.RunAborted, "Aborted by context cancellation", nil, ctx.Err())
	return m.finish("complete", as, result, nil, nil)
}

// streamHooks fires the OnModelStream hooks on a chunk that does not fail the
// turn (thinking, tool use): an error is a warning on the run, once per turn.
func (m *AgentLoopMachine) streamHooks(chunk api.StreamChunk, warned *bool) {
	for _, hook := range m.hooks.OnModelStream {
		if err := hook(m.loop, chunk); err != nil && !*warned {
			*warned = true
			m.warnf("a stream hook failed on %s: %v", chunk.Type, err)
		}
	}
}

// warnf warns on the run (a no-op outside a task service).
func (m *AgentLoopMachine) warnf(format string, args ...any) {
	if m.warn != nil {
		m.warn(fmt.Sprintf(format, args...))
	}
}

// fireOnError fires H9 hooks and returns whether the error was recovered.
func (m *AgentLoopMachine) fireOnError(loopErr *api.LoopError) bool {
	for _, hook := range m.hooks.OnError {
		if err := hook(m.loop, loopErr); err != nil {
			m.warnf("an error hook failed while handling %q: %v", loopErr.Phase, err)
		}
		if loopErr.Recovered {
			return true
		}
	}
	return false
}

// stepThinking: H1 → build sys msg + tools → H2 → LLM call (H3 per chunk) → H4 → route
func (m *AgentLoopMachine) stepThinking(_ context.Context, as *agentState) (machine.ActionFunc, error) {
	return func(ctx context.Context) (*machine.StepResult, error) {
		// Check abort before starting turn
		if abortResult := m.checkAbort(ctx, as); abortResult != nil {
			return abortResult, nil
		}

		as.Turn++
		var emits []machine.Event

		// Max turns check
		if as.Turn > as.MaxTurns {
			result := as.makeResult(api.RunMaxTurns, fmt.Sprintf("Aborted: exceeded maximum turns (%d)", as.MaxTurns), nil, nil)
			return m.finish("complete", as, result, nil, emits), nil
		}
		// Budget check: no model call once the run's cost has reached it.
		if spent := as.spentUSD(); as.MaxBudgetUSD > 0 && spent >= as.MaxBudgetUSD {
			result := as.makeResult(api.RunBudgetExceeded, fmt.Sprintf("Aborted: its cost $%.4f reached its budget $%.4f", spent, as.MaxBudgetUSD), nil, nil)
			return m.finish("complete", as, result, nil, emits), nil
		}

		// H1: OnTurnStart
		for _, hook := range m.hooks.OnTurnStart {
			if err := hook(m.loop, as.Turn); err != nil {
				result := as.makeResult(api.RunError, "", nil, fmt.Errorf("OnTurnStart: %w", err))
				return m.finish("failed", as, result, nil, emits), nil
			}
		}

		// Emit turn_start event
		emits = append(emits, machine.Event{
			Type:    api.EventTurnStart,
			Payload: api.TurnStartEvent{Turn: as.Turn, MaxTurns: as.MaxTurns},
		})

		// Build system message and tools per-turn (dynamic)
		sysMsg := m.buildSystemMessage()
		tools := m.buildToolDefs()

		// H2: OnPreAPICall
		for _, hook := range m.hooks.OnPreAPICall {
			if err := hook(m.loop, as.Messages); err != nil {
				result := as.makeResult(api.RunError, "", nil, fmt.Errorf("OnPreAPICall: %w", err))
				return m.finish("failed", as, result, nil, emits), nil
			}
		}

		// Update LoopContext tool snapshot (plugins can read via loop.ToolDefs())
		m.loop.SetToolDefs(tools)

		// Consume per-call overrides set by plugins (e.g., ModelFallback, OutputRecovery)
		modelOverride, maxTokensOverride, thinkingOverride := m.loop.ConsumeOverrides()

		// Build langchaingo call options
		lcTools := api.ToolDefsToTools(tools)
		opts := []llm.CallOption{
			llm.WithTools(lcTools),
		}
		if m.modelCallObserver != nil {
			opts = append(opts, llm.WithCallObserver(m.modelCallObserver))
		}
		if maxTokensOverride > 0 {
			opts = append(opts, llm.WithMaxTokens(maxTokensOverride))
		}
		if modelOverride != "" {
			opts = append(opts, llm.WithModel(modelOverride))
		}
		if thinkingOverride > 0 {
			opts = append(opts, llm.WithInterleaveThinking(true))
		}
		if thinkingOverride > 0 {
			opts = append(opts, llm.WithThinkingBudget(thinkingOverride))
		}

		// ── LLM call (streaming) ──
		// Emit llm_request
		llmReqEvent := api.LLMRequestEvent{
			Turn:         as.Turn,
			MessageCount: len(as.Messages),
			ToolCount:    len(tools),
			Messages:     as.Messages,
		}
		if as.Turn == 1 {
			llmReqEvent.SystemPrompt = sysMsg
			names := make([]string, len(tools))
			for i, t := range tools {
				names[i] = t.Name
			}
			llmReqEvent.ToolNames = names
		}
		emits = append(emits, machine.Event{Type: api.EventLLMRequest, Payload: llmReqEvent})

		// Build messages with system prompt prepended
		llmMessages := as.Messages
		if sysMsg != "" {
			llmMessages = append([]api.Message{api.NewTextMessage(api.RoleSystem, sysMsg)}, llmMessages...)
		}

		// Start streaming LLM call
		llmStart := time.Now()
		var firstToken bool
		var ttft time.Duration

		sm, _ := m.model.(llm.StreamingModel)
		stream, err := sm.GenerateContentStream(ctx, llmMessages, opts...)
		if err != nil {
			loopErr := &api.LoopError{Phase: "api_call", Err: err}
			if m.fireOnError(loopErr) {
				as.Turn--
				return &machine.StepResult{NextState: "thinking", Data: as, Emit: emits}, nil
			}
			return m.modelCallFailed(as, err, emits), nil
		}
		defer stream.Close()

		// Consume stream — emit events + call hooks in real time
		var textContent string
		var allToolCalls []llm.ToolCall
		var stopReason string
		var usage api.Usage
		var streamWarned bool // a stream hook's error is warned once per turn

		for stream.Next() {
			ev := stream.Event()

			if !firstToken && (ev.Type == llm.StreamEventTextDelta || ev.Type == llm.StreamEventThinkingDelta) {
				ttft = time.Since(llmStart)
				firstToken = true
			}

			switch ev.Type {
			case llm.StreamEventTextDelta:
				textContent += ev.Delta
				emits = append(emits, machine.Event{
					Type:    api.EventTextDelta,
					Payload: api.TextDeltaEvent{Text: ev.Delta, Turn: as.Turn},
				})
				streamChunk := api.StreamChunk{Type: "text_delta", Delta: ev.Delta}
				for _, hook := range m.hooks.OnModelStream {
					if hookErr := hook(m.loop, streamChunk); hookErr != nil {
						stream.Close()
						result := as.makeResult(api.RunError, "", nil, fmt.Errorf("OnModelStream: %w", hookErr))
						return m.finish("failed", as, result, nil, emits), nil
					}
				}

			case llm.StreamEventThinkingDelta:
				streamChunk := api.StreamChunk{Type: "thinking_delta", Delta: ev.Delta}
				m.streamHooks(streamChunk, &streamWarned)

			case llm.StreamEventToolUseStart:
				streamChunk := api.StreamChunk{Type: "tool_use_start", ToolID: ev.ToolID, ToolName: ev.ToolName}
				m.streamHooks(streamChunk, &streamWarned)

			case llm.StreamEventToolUseDelta:
				streamChunk := api.StreamChunk{Type: "tool_use_delta", ToolID: ev.ToolID, ToolInput: ev.ToolInput}
				m.streamHooks(streamChunk, &streamWarned)

			case llm.StreamEventToolUseDone:
				allToolCalls = append(allToolCalls, llm.ToolCall{
					ID: ev.ToolID,
					FunctionCall: &llm.FunctionCall{
						Name:      ev.ToolName,
						Arguments: ev.ToolInput,
					},
				})
				// Notify plugins that the tool call is complete (full input
				// JSON now available). Lets streaming-aware plugins finalize
				// state — e.g. create-document emits the final DocumentEvent.
				streamChunk := api.StreamChunk{
					Type:      "tool_use_done",
					ToolID:    ev.ToolID,
					ToolName:  ev.ToolName,
					ToolInput: ev.ToolInput,
				}
				m.streamHooks(streamChunk, &streamWarned)

			case llm.StreamEventUsage:
				usage = api.Usage{
					InputTokens:              ev.InputTokens,
					OutputTokens:             ev.OutputTokens,
					CacheReadInputTokens:     ev.CacheReadInputTokens,
					CacheCreationInputTokens: ev.CacheCreationInputTokens,
				}
				// Cost AT SOURCE: the model knows its own pricing, so we price
				// the call here and ride the breakdown on the Usage. Consumers
				// (journal cost tab, status line) just sum — no provider-formula
				// duplication, and multi-model sessions price each run correctly.
				mi := m.model.Info()
				usage.Model = mi.ModelID
				usage.Provider = mi.Provider
				c := mi.ComputeCost(ev.InputTokens, ev.OutputTokens, ev.CacheReadInputTokens, ev.CacheCreationInputTokens)
				usage.Cost = &c

			case llm.StreamEventDone:
				stopReason = ev.StopReason
			}
		}

		llmDuration := time.Since(llmStart)
		as.LastTTFTMs = ttft.Milliseconds()

		if err := stream.Err(); err != nil {
			loopErr := &api.LoopError{Phase: "api_call", Err: err}
			if m.fireOnError(loopErr) {
				as.Turn--
				return &machine.StepResult{NextState: "thinking", Data: as, Emit: emits}, nil
			}
			return m.modelCallFailed(as, err, emits), nil
		}

		resp := stream.Response()
		if resp == nil || len(resp.Choices) == 0 {
			result := as.makeResult(api.RunError, "", nil, fmt.Errorf("LLM returned empty response on turn %d", as.Turn))
			return m.finish("failed", as, result, nil, emits), nil
		}

		// Build assistant message from stream results
		var assistantParts []llm.ContentPart
		if textContent != "" {
			assistantParts = append(assistantParts, llm.TextPart(textContent))
		}
		for _, tc := range allToolCalls {
			assistantParts = append(assistantParts, tc)
		}
		assistantMsg := api.Message{Role: api.RoleAssistant, Parts: assistantParts}

		// H4: OnPostAPICall
		for _, hook := range m.hooks.OnPostAPICall {
			if err := hook(m.loop, assistantMsg); err != nil {
				result := as.makeResult(api.RunError, "", nil, fmt.Errorf("OnPostAPICall: %w", err))
				return m.finish("failed", as, result, nil, emits), nil
			}
		}

		as.accumulateUsage(usage)
		as.Messages = append(as.Messages, assistantMsg)
		as.LastResponse = resp
		as.LastLLMDurationMs = llmDuration.Milliseconds()

		// Publish this call's full prompt size (uncached input + cache read +
		// cache write) so the compaction plugin can decide on the next
		// OnTurnStart whether the context has grown past its trigger.
		m.loop.SetMeta("_last_prompt_tokens",
			usage.InputTokens+usage.CacheReadInputTokens+usage.CacheCreationInputTokens)

		// Emit llm_response
		emits = append(emits, machine.Event{
			Type: api.EventLLMResponse,
			Payload: api.LLMResponseEvent{
				Turn:       as.Turn,
				StopReason: stopReason,
				Blocks:     len(assistantParts),
				Usage:      usage,
				Content:    resp.Choices,
				DurationMs: llmDuration.Milliseconds(),
				TTFTMs:     as.LastTTFTMs,
			},
		})

		// Emit message_shown for complete text
		if textContent != "" {
			emits = append(emits, machine.Event{
				Type:    api.EventMessageShown,
				Payload: api.MessageShownEvent{Text: textContent, Turn: as.Turn},
			})
		}

		// Store this turn's stop reason for plugins (every turn: output
		// recovery counts the consecutive cut ones).
		m.loop.SetMeta("_last_stop_reason", stopReason)

		// Check if there are tool calls
		hasTools := len(allToolCalls) > 0

		if hasTools {
			as.LastResponse = resp
			return &machine.StepResult{NextState: "tool_dispatch", Data: as, Emit: emits}, nil
		}

		// No tools — fire H7: OnDecision
		decision := &api.Decision{}
		for _, hook := range m.hooks.OnDecision {
			if err := hook(m.loop, decision); err != nil {
				result := as.makeResult(api.RunError, "", nil, fmt.Errorf("OnDecision: %w", err))
				return m.finish("failed", as, result, nil, emits), nil
			}
		}

		if decision.Continue {
			// Plugin requested continuation — inject messages and loop
			as.Messages = append(as.Messages, decision.Messages...)
			as.LastResponse = nil

			// H8: OnTurnEnd
			for _, hook := range m.hooks.OnTurnEnd {
				if err := hook(m.loop, as.Turn); err != nil {
					result := as.makeResult(api.RunError, "", nil, fmt.Errorf("OnTurnEnd: %w", err))
					return m.finish("failed", as, result, nil, emits), nil
				}
			}
			return &machine.StepResult{NextState: "thinking", Data: as, Emit: emits}, nil
		}

		// H8: OnTurnEnd
		for _, hook := range m.hooks.OnTurnEnd {
			if err := hook(m.loop, as.Turn); err != nil {
				result := as.makeResult(api.RunError, "", nil, fmt.Errorf("OnTurnEnd: %w", err))
				return m.finish("failed", as, result, nil, emits), nil
			}
		}

		next, result := textTurnEnd(as, stopReason, textContent)
		if next == "failed" {
			return m.finish("failed", as, result, nil, emits), nil
		}
		return m.finish("complete", as, result, &api.LoopResult{
			Messages:   as.Messages,
			FinalText:  textContent,
			StopReason: stopReason,
			TurnCount:  as.Turn,
			Usage:      as.TotalUsage,
			Meta:       make(map[string]any),
		}, emits), nil
	}, nil
}

// stepToolDispatch: H5 per tool → dispatch → H6 after all → H7 decision → H8 turn end
// interruptedByRestart answers a tool call that was in flight when the process
// died: it may or may not have taken effect.
const interruptedByRestart = "interrupted by a restart: the process running this task stopped while this call was in progress — it may or may not have taken effect; check before doing it again"

// stepInterruptedBatch ends a tool batch that was in flight when the process
// died (the run restarted after a crash): the calls done before keep their
// results; every call not done yet is answered interruptedByRestart; the run
// goes on to its next turn, where the model decides what to redo.
func (m *AgentLoopMachine) stepInterruptedBatch(as *agentState) (machine.ActionFunc, error) {
	return func(context.Context) (*machine.StepResult, error) {
		batch := as.Batch
		if batch == nil {
			batch = &toolBatch{}
		}
		results := batch.Results
		var emits []machine.Event
		var calls []llm.ToolCall
		if as.LastResponse != nil {
			for _, c := range as.LastResponse.Choices {
				calls = append(calls, c.ToolCalls...)
			}
		}
		for i, tc := range calls {
			if i < batch.Next {
				continue
			}
			name := ""
			if tc.FunctionCall != nil {
				name = tc.FunctionCall.Name
			}
			results = append(results, llm.ToolCallResponse{ToolCallID: tc.ID, Name: name, Content: interruptedByRestart})
			emits = append(emits, machine.Event{
				Type:    api.EventToolError,
				Payload: api.ToolErrorEvent{Turn: as.Turn, Tool: name, ID: tc.ID, Error: interruptedByRestart},
			})
		}
		if len(results) > 0 {
			as.Messages = append(as.Messages, api.NewToolResultMessages(results...)...)
		}
		as.LastResponse = nil
		as.Batch = nil
		return &machine.StepResult{NextState: "thinking", Data: as, Emit: emits}, nil
	}, nil
}

// awaitStep is the step of a batch stopped on an awaiting call (its batch
// saved): the machine waits on the await's event — another process's event
// (EventURI, bridged onto the bus) or an in-process topic (a child run's end)
// — and checks again on resume and at the await's recheck period.
func awaitStep(as *agentState, emits []machine.Event, toolName string, await *api.ErrAwait) *machine.StepResult {
	cond := machine.WaitCondition{Topic: await.Topic, Label: toolName}
	if await.EventURI != "" {
		cond = machine.WaitCondition{Topic: machine.ExternalEventTopic(await.EventURI), EventURI: await.EventURI, Label: toolName}
	}
	return &machine.StepResult{
		NextState: "awaiting",
		Data:      as,
		Emit:      emits,
		Wait: &machine.WaitSpec{
			Conditions:      []machine.WaitCondition{cond},
			Mode:            machine.WaitAny,
			RecheckOnResume: true,
			// The tool asked to check again at this period, event or not.
			Timeout:      await.RecheckEvery,
			TimeoutLabel: "recheck",
		},
	}
}

func (m *AgentLoopMachine) stepToolDispatch(_ context.Context, as *agentState, replayed bool) (machine.ActionFunc, error) {
	return func(ctx context.Context) (*machine.StepResult, error) {
		resp := as.LastResponse
		if resp == nil {
			result := as.makeResult(api.RunError, "", nil, fmt.Errorf("no LLM response to dispatch"))
			return &machine.StepResult{NextState: "failed", Data: as, Output: result}, nil
		}

		dispatcher := m.buildDispatcher()

		// Re-inject session-scoped context values into the dispatch ctx
		// so tools can read them at execute time. The factory set these
		// on the plugin-creation context (sessionCtx), but the executor
		// drives the loop with its own ctx, which carries the
		// Application (for UseService) but not the session values. Tools
		// like ask-user read these from their runtime ctx — without the
		// re-inject they'd see empty values and fall back to defaults
		// (e.g. the interactive HumanGate instead of the resolved
		// UserComms channel).
		ctx = m.withSessionValues(ctx)

		// Expose the current turn so tools that emit their own
		// conversation events (prompt_shown / user_input) can tag them.
		ctx = api.WithCurrentTurn(ctx, as.Turn)

		// Tools may await instead of blocking (api.ErrAwait): this loop waits
		// as a machine — no slot held, its state saved.
		ctx = api.WithMachineWaits(ctx)

		// Build attribute lookups for tool-call event rendering. The loop
		// keys behavior off ToolDef attributes, never off tool names.
		toolDisplays := make(map[string]*api.ToolDisplay)
		interactiveTools := make(map[string]bool)
		for _, p := range m.plugins.Plugins() {
			for _, def := range p.ToolDefs() {
				if def.Display != nil {
					toolDisplays[def.Name] = def.Display
				}
				if def.Interactive {
					interactiveTools[def.Name] = true
				}
			}
		}

		// A batch stopped on an awaiting call goes on from that call, with the
		// results of the calls before it.
		batch := as.Batch
		if batch == nil {
			batch = &toolBatch{}
		}
		toolResults := batch.Results
		apiToolResults := batch.APIResults
		runCompleteResult := batch.Complete
		var emits []machine.Event

		// Collect tool calls from ALL choices (langchaingo returns one Choice per tool call)
		var dispatchToolCalls []llm.ToolCall
		for _, c := range resp.Choices {
			dispatchToolCalls = append(dispatchToolCalls, c.ToolCalls...)
		}

		// awaitCall stops the batch on call i: the results so far and the
		// call's input saved, the machine waiting on the await's event.
		awaitCall := func(i int, toolName string, input json.RawMessage, await *api.ErrAwait) *machine.StepResult {
			as.Batch = &toolBatch{
				Results: toolResults, APIResults: apiToolResults, Complete: runCompleteResult,
				Next: i, Input: string(input), Resume: await.Resume,
			}
			return awaitStep(as, emits, toolName, await)
		}
		// emitToolCall publishes a call's tool_call immediately (not deferred
		// via emits) so the terminal protocol can display the tool BEFORE
		// Dispatch blocks (e.g. ask-user blocks on HumanGate waiting for user
		// answer). Not added to emits to avoid double-publish. The emitter
		// (run id) is stamped: publishImmediate bypasses the executor's
		// EventStamper that the deferred `emits` path uses, so without it the
		// tool_call lands with an empty emitter and the journal can't
		// attribute it to its run.
		emitToolCall := func(toolName, toolID string, input json.RawMessage) {
			toolCallEvt := api.ToolCallEvent{
				Turn:    as.Turn,
				Tool:    toolName,
				ID:      toolID,
				Input:   input,
				Display: toolDisplays[toolName],
			}
			toolCallEvt.StampEvent(api.EventToolCall, m.loop.RunID())
			publishImmediate(ctx, api.EventToolCall, toolCallEvt)
		}

		for i, tc := range dispatchToolCalls {
			if i < batch.Next {
				continue // ran before the await
			}
			toolName := tc.FunctionCall.Name
			toolID := tc.ID
			toolInput := json.RawMessage(tc.FunctionCall.Arguments)

			// The call that awaited is called again to check: it passed the
			// hooks and the gate, and its tool_call was shown, the first time.
			callCtx := ctx
			var effectiveInput json.RawMessage
			// It is called again even when it gave no Resume: an empty one
			// tells it it checks an answer and does not ask anew.
			resuming := as.Batch != nil && i == batch.Next
			if replayed && as.Batch != nil && i > batch.Next {
				// The woken step was cut by a crash: this call may or may not
				// have run — answered, never run again (OD-009).
				toolResults = append(toolResults, llm.ToolCallResponse{ToolCallID: toolID, Name: toolName, Content: interruptedByRestart})
				apiToolResults = append(apiToolResults, api.ToolResult{ToolUseID: toolID, ToolName: toolName, Content: interruptedByRestart, IsError: true})
				emits = append(emits, machine.Event{
					Type:    api.EventToolError,
					Payload: api.ToolErrorEvent{Turn: as.Turn, Tool: toolName, ID: toolID, Error: interruptedByRestart},
				})
				continue
			}
			permTask, _ := batch.Resume[permissionTaskKey].(string)
			if permID, _ := batch.Resume[permissionRequestKey].(string); resuming && (permID != "" || permTask != "") {
				// The call waited on the person's permission (an inbox request, or its human task):
				// read the answer. Allowed, the tool runs with its saved input
				// and a plain context — the permission is not its own await.
				effectiveInput = json.RawMessage(batch.Input)
				var answer permissionAnswer
				var msg string
				var again *api.ErrAwait
				if permTask != "" {
					// Asked as a permission human task.
					rules := rulesOf(batch.Resume)
					answer, msg = resumePermissionTask(ctx, toolName, permTask, rules)
					again = permissionTaskAwait(permTask, rules)
				} else {
					answer, msg = resumePermission(ctx, toolName, permID)
					again = permissionAwait(permID)
				}
				switch answer {
				case permissionPending:
					return awaitCall(i, toolName, effectiveInput, again), nil
				case permissionDenied:
					toolResults = append(toolResults, llm.ToolCallResponse{ToolCallID: toolID, Name: toolName, Content: msg})
					apiToolResults = append(apiToolResults, api.ToolResult{ToolUseID: toolID, ToolName: toolName, Content: msg, IsError: true})
					continue
				}
				emitToolCall(toolName, toolID, effectiveInput)
			} else if resuming {
				resume := batch.Resume
				if resume == nil {
					resume = map[string]any{}
				}
				callCtx = api.WithAwaitResume(ctx, resume)
				effectiveInput = json.RawMessage(batch.Input)
			} else {

				// H5: OnPreToolExec — includes the loop-safety repetition
				// guard (local/loopsafety plugin), which denies a tool call
				// repeated with identical arguments past its threshold.
				var denied bool
				var denyReason string
				inputMap := make(map[string]any)
				if len(bytes.TrimSpace(toolInput)) > 0 && string(bytes.TrimSpace(toolInput)) != "null" {
					if err := json.Unmarshal(toolInput, &inputMap); err != nil {
						m.warnf("the input of tool %s is not a JSON object — its permission hooks saw none: %v", toolName, err)
					}
				}
				for _, hook := range m.hooks.OnPreToolExec {
					td, err := hook(m.loop, toolName, inputMap)
					if err != nil {
						denied = true
						denyReason = err.Error()
						break
					}
					if td != nil && td.Decision == api.Deny {
						denied = true
						denyReason = td.Reason
						break
					}
				}

				if denied {
					toolResults = append(toolResults, llm.ToolCallResponse{ToolCallID: toolID, Name: toolName, Content: fmt.Sprintf("Denied: %s", denyReason)})
					apiToolResults = append(apiToolResults, api.ToolResult{ToolUseID: toolID, ToolName: toolName, Content: denyReason, IsError: true})
					continue
				}

				// Per-tool permission gate
				gateOutcome := applyPermissionGate(ctx, dispatcher, toolName, toolInput)
				if gateOutcome.awaitRequest != "" {
					// Asked through the inbox: wait for the person's answer.
					return awaitCall(i, toolName, gateOutcome.effectiveInput, permissionAwait(gateOutcome.awaitRequest)), nil
				}
				if gateOutcome.awaitTask != "" {
					// Asked as a permission human task: wait
					// for its end.
					return awaitCall(i, toolName, gateOutcome.effectiveInput, permissionTaskAwait(gateOutcome.awaitTask, gateOutcome.rules)), nil
				}
				if !gateOutcome.allow {
					toolResults = append(toolResults, llm.ToolCallResponse{ToolCallID: toolID, Name: toolName, Content: gateOutcome.errMsg})
					apiToolResults = append(apiToolResults, api.ToolResult{ToolUseID: toolID, ToolName: toolName, Content: gateOutcome.errMsg, IsError: true})
					continue
				}
				effectiveInput = gateOutcome.effectiveInput
				emitToolCall(toolName, toolID, effectiveInput)
			}

			// Dispatch tool. Interactive tools (ask-user) emit their own
			// prompt_shown / user_input events from inside the dispatch —
			// the loop knows nothing about specific tools.
			toolStart := time.Now()
			toolCallResult, err := dispatcher.Dispatch(callCtx, toolName, effectiveInput)
			toolDuration := time.Since(toolStart)

			// The tool's answer comes later: wait for its event as a machine,
			// then call it again (state `awaiting`).
			var await *api.ErrAwait
			if errors.As(err, &await) {
				return awaitCall(i, toolName, effectiveInput, await), nil
			}

			var tcErr *api.ErrRunComplete
			if errors.As(err, &tcErr) {
				runCompleteResult = tcErr
				toolResults = append(toolResults, llm.ToolCallResponse{ToolCallID: toolID, Name: toolName, Content: fmt.Sprintf("Task complete: %s", tcErr.Summary)})
				apiToolResults = append(apiToolResults, api.ToolResult{ToolUseID: toolID, ToolName: toolName, Content: tcErr.Summary, IsError: false})
				emits = append(emits, machine.Event{
					Type:    api.EventToolResult,
					Payload: api.ToolResultEvent{Turn: as.Turn, Tool: toolName, ID: toolID, Result: "complete", DurationMs: toolDuration.Milliseconds()},
				})
			} else if err != nil {
				loopErr := &api.LoopError{Phase: "tool_exec", Err: err}
				if m.fireOnError(loopErr) {
					continue
				}
				toolResults = append(toolResults, llm.ToolCallResponse{ToolCallID: toolID, Name: toolName, Content: err.Error()})
				apiToolResults = append(apiToolResults, api.ToolResult{ToolUseID: toolID, ToolName: toolName, Content: err.Error(), IsError: true})
				emits = append(emits, machine.Event{
					Type:    api.EventToolError,
					Payload: api.ToolErrorEvent{Turn: as.Turn, Tool: toolName, ID: toolID, Error: err.Error(), DurationMs: toolDuration.Milliseconds()},
				})
			} else {
				resultStr := capToolResult(dispatcher.LookupTool(toolName), toolID, toolCallResult.ResultString(), m.resultDir)
				toolResults = append(toolResults, llm.ToolCallResponse{ToolCallID: toolID, Name: toolName, Content: resultStr})
				apiToolResults = append(apiToolResults, api.ToolResult{ToolUseID: toolID, ToolName: toolName, Content: resultStr, IsError: false})
				emits = append(emits, machine.Event{
					Type:    api.EventToolResult,
					Payload: api.ToolResultEvent{Turn: as.Turn, Tool: toolName, ID: toolID, Result: resultStr, DurationMs: toolDuration.Milliseconds()},
				})

				// Emit tool_feedback — except for interactive tools, whose
				// result is the user's answer (already surfaced by the tool
				// itself as a user_input event).
				if !interactiveTools[toolName] {
					feedbackSummary := resultStr
					if len(feedbackSummary) > 200 {
						feedbackSummary = feedbackSummary[:200] + "..."
					}
					emits = append(emits, machine.Event{
						Type:    api.EventToolFeedback,
						Payload: api.ToolFeedbackEvent{Tool: toolName, Summary: feedbackSummary, DurationMs: toolDuration.Milliseconds()},
					})
				}
			}
		}

		// H6: OnPostToolExec
		for _, hook := range m.hooks.OnPostToolExec {
			if err := hook(m.loop, apiToolResults); err != nil {
				result := as.makeResult(api.RunError, "", nil, fmt.Errorf("OnPostToolExec: %w", err))
				return m.finish("failed", as, result, nil, emits), nil
			}
		}

		// Append tool results to messages
		if len(toolResults) > 0 {
			as.Messages = append(as.Messages, api.NewToolResultMessages(toolResults...)...)
		}
		as.LastResponse = nil
		as.Batch = nil

		// Run complete?
		if runCompleteResult != nil {
			// An output-schema task carries its declared envelope here — surface
			// it as the machine output so task.output is the structured map.
			as.StructuredOutput = runCompleteResult.Output
			result := as.makeResult(api.RunCompleted, runCompleteResult.Summary, runCompleteResult.Files, nil)

			// H8: OnTurnEnd — still fires before the terminal chokepoint
			// (which fires H11 OnResult and emits complete/session_end).
			for _, hook := range m.hooks.OnTurnEnd {
				if err := hook(m.loop, as.Turn); err != nil {
					result := as.makeResult(api.RunError, "", nil, fmt.Errorf("OnTurnEnd: %w", err))
					return m.finish("failed", as, result, nil, emits), nil
				}
			}

			return m.finish("complete", as, result, &api.LoopResult{
				Messages:   as.Messages,
				FinalText:  runCompleteResult.Summary,
				StopReason: "completed",
				TurnCount:  as.Turn,
				Usage:      as.TotalUsage,
				Meta:       map[string]any{"summary": runCompleteResult.Summary, "files": runCompleteResult.Files},
			}, emits), nil
		}

		// H7: OnDecision
		decision := &api.Decision{}
		for _, hook := range m.hooks.OnDecision {
			if err := hook(m.loop, decision); err != nil {
				result := as.makeResult(api.RunError, "", nil, fmt.Errorf("OnDecision: %w", err))
				return m.finish("failed", as, result, nil, emits), nil
			}
		}

		if decision.Continue {
			as.Messages = append(as.Messages, decision.Messages...)
		}

		// H8: OnTurnEnd
		for _, hook := range m.hooks.OnTurnEnd {
			if err := hook(m.loop, as.Turn); err != nil {
				result := as.makeResult(api.RunError, "", nil, fmt.Errorf("OnTurnEnd: %w", err))
				return m.finish("failed", as, result, nil, emits), nil
			}
		}

		// Continue to next turn
		return &machine.StepResult{NextState: "thinking", Data: as, Emit: emits}, nil
	}, nil
}

// textTurnEnd is how a run ends on a turn with no tool call that no plugin
// continued: completed with the text as its summary (the model ended without
// the complete tool) - unless the answer was cut at the output limit, which is
// no end of the work: then it fails, saying so.
func textTurnEnd(as *agentState, stopReason, text string) (string, *api.RunResult) {
	if stopReason == "max_tokens" {
		return "failed", as.makeResult(api.RunError, text, nil,
			fmt.Errorf("the model's answer was cut at its output limit (max_tokens) on turn %d and could not be continued", as.Turn))
	}
	summary := text
	if summary == "" {
		summary = "LLM ended conversation without using the complete tool"
	}
	return "complete", as.makeResult(api.RunCompleted, summary, nil, nil)
}

// --- Terminal chokepoint ---

// finish is the SINGLE terminal chokepoint: every path that ends the
// run — natural completion, the complete tool, errors, aborts, max
// turns — flows through it. It fires the H11 OnResult hooks and then
// appends the complete + session_end events, so a new terminal path
// cannot forget either. lr may carry a site-specific LoopResult
// (final text, stop reason, meta); nil builds a generic one from the
// run result.
func (m *AgentLoopMachine) finish(next string, as *agentState, result *api.RunResult, lr *api.LoopResult, emits []machine.Event) *machine.StepResult {
	if lr == nil {
		lr = &api.LoopResult{
			Messages:   as.Messages,
			FinalText:  result.Summary,
			StopReason: result.Status.String(),
			TurnCount:  as.Turn,
			Usage:      as.TotalUsage,
			Meta:       make(map[string]any),
		}
	}
	for _, hook := range m.hooks.OnResult {
		if err := hook(m.loop, lr); err != nil {
			lr.Errors = append(lr.Errors, err)
		}
	}
	if c, ok := m.modelCallObserver.(io.Closer); ok {
		_ = c.Close()
	}
	emits = append(emits, m.completeEvent(result), m.sessionEndEvent(as))
	// An output-schema task makes its structured envelope the machine output, so
	// task.output (and RunResponse.Output) carry the declared map a consumer
	// reads with zero NLP. Schema-less runs keep the RunResult struct.
	out := any(result)
	if as.StructuredOutput != nil {
		out = as.StructuredOutput
	}
	step := &machine.StepResult{NextState: next, Data: as, Output: out, Emit: emits}
	// A failed end says why: the run is recorded failed with its reason
	// , never completed with its error lost.
	if next == "failed" {
		step.Failure = &machine.Failure{Reason: failureReason(result)}
	}
	return step
}

// failureReason is why a run failed, as text: its error, else its summary,
// else its status.
func failureReason(r *api.RunResult) string {
	switch {
	case r.Error != nil:
		return r.Error.Error()
	case r.Summary != "":
		return r.Summary
	}
	return r.Status.String()
}

// --- Event helpers ---

func (m *AgentLoopMachine) completeEvent(r *api.RunResult) machine.Event {
	ev := api.CompleteEvent{
		Status:   r.Status.String(),
		Summary:  r.Summary,
		Turns:    r.Turns,
		Usage:    r.Usage,
		Duration: r.Duration,
	}
	if r.Error != nil {
		ev.Error = r.Error.Error()
		if msg, ok := usermsg.Of(r.Error); ok {
			ev.Msg = &msg
		}
	}
	return machine.Event{Type: api.EventComplete, Payload: ev}
}

// modelCallFailed ends a run whose model call failed: its error event and its
// result carry the error's text and the message key of its cause (no key, a
// refused key, an unreachable endpoint, an unknown model), so its chat, its
// result and its transcript say why and what to do.
func (m *AgentLoopMachine) modelCallFailed(as *agentState, err error, emits []machine.Event) *machine.StepResult {
	model := ""
	if m.model != nil {
		model = m.model.Info().ModelID
	}
	keyed := ModelCallError(err, model)
	ev := api.ErrorEvent{Turn: as.Turn, Error: err.Error()}
	if msg, ok := usermsg.Of(keyed); ok {
		ev.Msg = &msg
	}
	emits = append(emits, machine.Event{Type: api.EventError, Payload: ev})
	result := as.makeResult(api.RunError, "", nil, fmt.Errorf("LLM error on turn %d: %w", as.Turn, keyed))
	return m.finish("failed", as, result, nil, emits)
}

func (m *AgentLoopMachine) sessionEndEvent(as *agentState) machine.Event {
	return machine.Event{
		Type: api.EventSessionEnd,
		Payload: api.SessionEndEvent{
			Turns:      as.Turn,
			Usage:      as.TotalUsage,
			DurationMs: time.Since(as.StartedAt).Milliseconds(),
		},
	}
}

// Streaming is handled via GenerateContentStream → Stream iterator.
// The agent loop consumes StreamEvents in stepThinking.

// ensureTyped handles State.Data type conversion after checkpoint restore. A
// state that does not read back whole is an error: a run never goes on with a
// half-read conversation.
func (m *AgentLoopMachine) ensureTyped(state *machine.State) (*agentState, error) {
	if as, ok := state.Data.(*agentState); ok {
		return as, nil
	}
	as := &agentState{}
	if state.Data != nil {
		b, err := json.Marshal(state.Data)
		if err != nil {
			return nil, fmt.Errorf("agent-loop: saved state: %w", err)
		}
		if err := json.Unmarshal(b, as); err != nil {
			return nil, fmt.Errorf("agent-loop: saved state cannot be read back: %w", err)
		}
	}
	state.Data = as
	return as, nil
}

// publishImmediate sends an event directly to the EventBus without waiting
// for the StepResult.Emit batch. Used for events that must reach subscribers
// BEFORE a blocking tool call (e.g. ask-user blocks on HumanGate — the prompt
// event must reach the TUI so the user can answer and unblock it).
func publishImmediate(ctx context.Context, topic string, payload any) {
	bus, err := application.UseService[machine.EventBus](ctx)
	if err != nil || bus == nil {
		return
	}
	_ = bus.Publish(topic, payload)
}
