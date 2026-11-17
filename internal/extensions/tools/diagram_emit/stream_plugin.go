// SPDX-License-Identifier: Apache-2.0

package diagram_emit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	drawiorender "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/render"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// streamPlugin draws the diagram while the model is still writing it. It
// watches the diagram-emit tool call's input as it streams, and each time
// a node, edge or zone is complete it draws what there is so far and
// sends it to the document tab (a DocumentDeltaEvent carrying the whole
// drawing). The first drawing is laid out; the next ones are drawn over
// the previous one (render.Refine) so nothing already shown moves. When
// the call is complete it draws the final diagram with a full layout,
// finalises the tab, and leaves the result for the tool to return — the
// tab and the tool's answer are the same drawing.
type streamPlugin struct {
	run    run
	ctx    context.Context
	mu     sync.Mutex
	states map[string]*drawState // by tool id
}

func newStreamPlugin() *streamPlugin { return &streamPlugin{states: map[string]*drawState{}} }

func (p *streamPlugin) Name() string                 { return "diagram_emit-stream" }
func (p *streamPlugin) SystemMessage() string        { return "" }
func (p *streamPlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (p *streamPlugin) Stable() bool                 { return true }

func (p *streamPlugin) HandleToolCall(context.Context, string, json.RawMessage) (*agentapi.ToolCallResult, error) {
	return nil, nil
}

func (p *streamPlugin) Hooks() agentapi.PluginHooks {
	return agentapi.PluginHooks{OnModelStream: p.onStream}
}

// checkpointEvery is the shortest time between two drawings of one
// streaming diagram.
const checkpointEvery = 400 * time.Millisecond

type drawState struct {
	id       string
	raw      string
	elements int // complete nodes + edges + zones drawn last time
	lastAt   time.Time
	lastXML  string
	title    string
	opened   bool
}

func (p *streamPlugin) onStream(loop agentapi.LoopContext, chunk agentapi.StreamChunk) error {
	if loop != nil {
		p.run.set(loop.RunID())
	}
	switch chunk.Type {
	case "tool_use_start":
		if chunk.ToolName == "diagram-emit" {
			p.mu.Lock()
			p.states[chunk.ToolID] = &drawState{id: chunk.ToolID}
			p.mu.Unlock()
		}
	case "tool_use_delta":
		p.mu.Lock()
		st, ok := p.states[chunk.ToolID]
		p.mu.Unlock()
		if ok {
			st.raw += chunk.ToolInput
			p.checkpoint(st)
		}
	case "tool_use_done":
		p.mu.Lock()
		st, ok := p.states[chunk.ToolID]
		delete(p.states, chunk.ToolID)
		p.mu.Unlock()
		if ok {
			p.finish(st)
		}
	}
	return nil
}

// checkpoint draws the diagram so far when a new element is complete.
func (p *streamPlugin) checkpoint(st *drawState) {
	if time.Since(st.lastAt) < checkpointEvery {
		return
	}
	snapshot, ok := completeJSON(st.raw)
	if !ok {
		return
	}
	var in Input
	if json.Unmarshal([]byte(snapshot), &in) != nil || in.Diagram == nil || in.Diagram.Viewpoint == "" || len(in.Diagram.Nodes) == 0 {
		return
	}
	n := len(in.Diagram.Nodes) + len(in.Diagram.Edges) + len(in.Diagram.Zones)
	if n <= st.elements {
		return
	}
	var (
		res drawiorender.EmitResult
		err error
	)
	switch {
	case st.lastXML != "":
		res, err = drawiorender.Refine(p.context(), in.Diagram, []byte(st.lastXML))
	case in.Previous != "":
		res, err = drawiorender.Refine(p.context(), in.Diagram, []byte(in.Previous))
	default:
		res, err = drawiorender.Emit(p.context(), in.Diagram, in.NoLayout)
	}
	if err != nil || res.XML == "" {
		return
	}
	st.elements, st.lastAt, st.lastXML = n, time.Now(), res.XML
	if st.title == "" {
		st.title = documentTitle(in.Diagram.Title)
	}
	evt := agentapi.DocumentDeltaEvent{ID: st.id, Content: res.XML}
	if !st.opened {
		evt.Title, evt.MediaType, st.opened = st.title, render.MediaTypeDrawio, true
	}
	p.publish(agentapi.EventDocumentDelta, &evt)
}

// finish draws the complete diagram, finalises the tab and keeps the
// result for the tool.
func (p *streamPlugin) finish(st *drawState) {
	var in Input
	if json.Unmarshal([]byte(st.raw), &in) != nil || in.Diagram == nil {
		return
	}
	res, err := draw(p.context(), in)
	if err != nil {
		return
	}
	remember(in, res)
	if res.XML == "" {
		return
	}
	title := st.title
	if title == "" {
		title = documentTitle(in.Diagram.Title)
	}
	evt := agentapi.DocumentEvent{ID: st.id, Title: title, MediaType: render.MediaTypeDrawio, Content: res.XML}
	p.publish(agentapi.EventDocument, &evt)
}

func (p *streamPlugin) context() context.Context {
	if p.ctx != nil {
		return p.ctx
	}
	return context.Background()
}

type stampable interface {
	StampEvent(string, string)
}

func (p *streamPlugin) publish(topic string, evt stampable) {
	bus, err := application.UseService[engine.EventBus](p.context())
	if err != nil || bus == nil {
		return
	}
	evt.StampEvent(topic, p.run.get())
	switch e := evt.(type) {
	case *agentapi.DocumentDeltaEvent:
		bus.Publish(topic, *e)
	case *agentapi.DocumentEvent:
		bus.Publish(topic, *e)
	}
}

func documentTitle(title string) string {
	if title == "" {
		title = "diagram"
	}
	return title + ".drawio"
}

// ─── the result the stream drew, for the tool ─────────────────────────

var (
	drawnMu sync.Mutex
	drawn   = map[string]drawiorender.EmitResult{}
)

func inputKey(in Input) string {
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func remember(in Input, res drawiorender.EmitResult) {
	drawnMu.Lock()
	defer drawnMu.Unlock()
	if len(drawn) > 32 {
		drawn = map[string]drawiorender.EmitResult{}
	}
	drawn[inputKey(in)] = res
}

// recall returns (and forgets) what the stream drew for this input.
func recall(in Input) (drawiorender.EmitResult, bool) {
	drawnMu.Lock()
	defer drawnMu.Unlock()
	k := inputKey(in)
	res, ok := drawn[k]
	delete(drawn, k)
	return res, ok
}

// draw is the one way a diagram-emit input becomes a drawing: over the
// previous drawing when there is one, else laid out afresh.
func draw(ctx context.Context, in Input) (drawiorender.EmitResult, error) {
	if in.Previous != "" && in.Diagram != nil {
		return drawiorender.Refine(ctx, in.Diagram, []byte(in.Previous))
	}
	return drawiorender.Emit(ctx, in.Diagram, in.NoLayout)
}

// ─── partial JSON ──────────────────────────────────────────────────────

// completeJSON turns the streamed prefix of a diagram-emit input into
// valid JSON: it cuts after the last complete element of one of the
// diagram's arrays (nodes, edges, zones) and closes what is open there.
// It reports false while no element is complete.
func completeJSON(s string) (string, bool) {
	var stack []byte
	inString, escaped := false, false
	cut, closers := -1, ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			stack = append(stack, c)
		case '}', ']':
			if len(stack) == 0 {
				return "", false
			}
			stack = stack[:len(stack)-1]
			// input { diagram { nodes [ element } — an element of one of
			// the diagram's arrays just closed.
			if c == '}' && len(stack) == 3 && stack[0] == '{' && stack[1] == '{' && stack[2] == '[' {
				cut = i + 1
				closers = ""
				for j := len(stack) - 1; j >= 0; j-- {
					if stack[j] == '[' {
						closers += "]"
					} else {
						closers += "}"
					}
				}
			}
		}
	}
	if cut < 0 {
		return "", false
	}
	return s[:cut] + closers, true
}

// run records the id of the agent run whose model stream this plugin
// watches, so the events it publishes carry it as their emitter: a chat's
// translator shows only its own runs' events, and an event with no emitter
// reaches EVERY open chat — which is how one chat's documents appeared in
// another.
type run struct {
	mu sync.Mutex
	id string
}

func (r *run) set(id string) {
	if id == "" {
		return
	}
	r.mu.Lock()
	r.id = id
	r.mu.Unlock()
}

func (r *run) get() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.id
}
