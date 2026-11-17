// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"io"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// loopContextImpl is the thread-safe implementation of api.LoopContext.
// It holds a pointer to agentState — the single source of truth for messages, turn, usage.
// API call overrides are stored here and consumed (cleared) by the machine before each LLM call.
type loopContextImpl struct {
	mu    sync.RWMutex
	state *agentState    // direct pointer to the machine's mutable state
	meta  map[string]any // ephemeral per-run metadata (not checkpointed)
	abort chan struct{}
	once  sync.Once

	// Per-call overrides — set by plugins, consumed and cleared by the machine.
	modelOverride  string
	maxTokens      int
	thinkingBudget int

	// Tool defs snapshot — set by the machine at the start of each turn.
	toolDefs []api.ToolDef

	// Trace writer — when set, the debug plugin writes NLJSON entries here.
	TraceWriter io.Writer

	// mode is the run's resolved delegation mode (set once by the
	// factory before plugins are created; read-only afterwards).
	mode string

	// permissionProfile is the run's active kernel permission profile name
	// (set once by the factory from ctx, before plugins are created;
	// read-only afterwards). The delegate plugin reads it to propagate the
	// parent's profile onto the children it spawns.
	permissionProfile string
}

// NewLoopContext creates a LoopContext. Call Bind() to connect to agentState.
func NewLoopContext() *loopContextImpl {
	return &loopContextImpl{
		meta:  make(map[string]any),
		abort: make(chan struct{}),
	}
}

// Bind connects the LoopContext to an agentState.
func (lc *loopContextImpl) Bind(state *agentState) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.state = state
}

// SetToolDefs updates the tool definitions snapshot. Called by the machine per-turn.
func (lc *loopContextImpl) SetToolDefs(defs []api.ToolDef) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.toolDefs = defs
}

// ConsumeOverrides returns and clears the per-call overrides.
// Called by the machine before each API call.
func (lc *loopContextImpl) ConsumeOverrides() (modelOverride string, maxTokens int, thinkingBudget int) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	modelOverride = lc.modelOverride
	maxTokens = lc.maxTokens
	thinkingBudget = lc.thinkingBudget
	lc.modelOverride = ""
	lc.maxTokens = 0
	lc.thinkingBudget = 0
	return
}

// --- LoopContext interface implementation ---

func (lc *loopContextImpl) GetMeta(key string) any {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.meta[key]
}

func (lc *loopContextImpl) SetMeta(key string, value any) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.meta[key] = value
}

func (lc *loopContextImpl) Messages() []api.Message {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	if lc.state == nil {
		return nil
	}
	cp := make([]api.Message, len(lc.state.Messages))
	copy(cp, lc.state.Messages)
	return cp
}

func (lc *loopContextImpl) PushMessage(msg api.Message) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.state == nil {
		return
	}
	lc.state.Messages = append(lc.state.Messages, msg)
}

func (lc *loopContextImpl) ReplaceMessages(msgs []api.Message) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.state == nil {
		return
	}
	lc.state.Messages = msgs
}

func (lc *loopContextImpl) AbortSignal() <-chan struct{} {
	return lc.abort
}

func (lc *loopContextImpl) SetModelOverride(modelID string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.modelOverride = modelID
}

func (lc *loopContextImpl) SetMaxTokens(n int) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.maxTokens = n
}

func (lc *loopContextImpl) SetThinkingBudget(tokens int) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.thinkingBudget = tokens
}

func (lc *loopContextImpl) ToolDefs() []api.ToolDef {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	cp := make([]api.ToolDef, len(lc.toolDefs))
	copy(cp, lc.toolDefs)
	return cp
}

func (lc *loopContextImpl) Turn() int {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	if lc.state == nil {
		return 0
	}
	return lc.state.Turn
}

func (lc *loopContextImpl) TotalUsage() api.Usage {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	if lc.state == nil {
		return api.Usage{}
	}
	return lc.state.TotalUsage
}

// SetRunMode records the run's resolved delegation mode. Called once
// by the factory before plugin creation.
func (lc *loopContextImpl) SetRunMode(mode string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.mode = mode
}

func (lc *loopContextImpl) Mode() string {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.mode
}

func (lc *loopContextImpl) IsChat() bool {
	return lc.Mode() == "chat"
}

// SetPermissionProfile records the run's active permission profile name.
// Called once by the factory (from ctx) before plugin creation.
func (lc *loopContextImpl) SetPermissionProfile(name string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.permissionProfile = name
}

func (lc *loopContextImpl) PermissionProfile() string {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.permissionProfile
}

// RunID returns this run's ID, stamped into loop meta by the machine at
// each Step. The delegate plugin reads it as the parent_run_id for
// spawned children. Empty until stamped (root-task pre-step).
func (lc *loopContextImpl) RunID() string {
	if v, ok := lc.GetMeta("_runId").(string); ok {
		return v
	}
	return ""
}

// InheritedPlugins returns the parent's plugin refs to seed a prompt/fork
// child with, from loop meta. Currently always nil (no setter wires
// `_plugins` yet); the accessor owns the meta-key string so task-side
// plugins don't reach it raw.
func (lc *loopContextImpl) InheritedPlugins() []any {
	if v, ok := lc.GetMeta("_plugins").([]any); ok {
		return v
	}
	return nil
}

// CloseAbort closes the abort channel, signaling all listeners.
func (lc *loopContextImpl) CloseAbort() {
	lc.once.Do(func() {
		close(lc.abort)
	})
}

var _ api.LoopContext = (*loopContextImpl)(nil)
