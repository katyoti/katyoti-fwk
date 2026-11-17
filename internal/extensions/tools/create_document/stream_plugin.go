// SPDX-License-Identifier: Apache-2.0

package create_document

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// streamPlugin is a tiny AgentPlugin that only registers OnModelStream.
// It watches for `create-document` tool-use deltas in the LLM stream,
// extracts the `content` field incrementally via a partial-JSON parser,
// and publishes DocumentDeltaEvent on the EventBus so the translator
// can produce msg.delta messages — giving the user live "building up"
// rendering as the LLM types the document.
type streamPlugin struct {
	run    run
	ctx    context.Context
	mu     sync.Mutex
	accums map[string]*docStreamState // keyed by tool_id
}

func newStreamPlugin() *streamPlugin {
	return &streamPlugin{
		accums: make(map[string]*docStreamState),
	}
}

func (p *streamPlugin) Name() string                 { return "create-document-stream" }
func (p *streamPlugin) SystemMessage() string        { return "" }
func (p *streamPlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (p *streamPlugin) Stable() bool                 { return true }

func (p *streamPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	return nil, nil // this plugin contributes no tools
}

func (p *streamPlugin) Hooks() agentapi.PluginHooks {
	return agentapi.PluginHooks{
		OnModelStream: p.onStream,
	}
}

// onStream receives every LLM stream chunk. For tool_use_delta chunks
// where the tool is create-document, parse incrementally and emit
// document deltas for new chars inside the `content` field.
func (p *streamPlugin) onStream(loop agentapi.LoopContext, chunk agentapi.StreamChunk) error {
	if loop != nil {
		p.run.set(loop.RunID())
	}
	// Filter: only care about tool input deltas for create-document
	switch chunk.Type {
	case "tool_use_start":
		if chunk.ToolName == "create-document" {
			p.mu.Lock()
			p.accums[chunk.ToolID] = newDocStreamState(chunk.ToolID)
			p.mu.Unlock()
		}
		return nil

	case "tool_use_delta":
		p.mu.Lock()
		state, ok := p.accums[chunk.ToolID]
		p.mu.Unlock()
		if !ok {
			return nil // not our tool, or we missed the start
		}
		deltas := state.feed(chunk.ToolInput)
		if len(deltas) == 0 && !state.metaEmitted {
			return nil
		}
		p.emit(state, deltas)
		return nil

	case "tool_use_done":
		p.mu.Lock()
		state, ok := p.accums[chunk.ToolID]
		if ok {
			delete(p.accums, chunk.ToolID)
		}
		p.mu.Unlock()
		if !ok {
			return nil
		}
		// Publish the final DocumentEvent so the translator can finalize
		// the message (status: streaming → done) with the full content.
		p.emitDone(state)
		return nil
	}
	return nil
}

// emitDone publishes the final DocumentEvent when the tool call is complete.
func (p *streamPlugin) emitDone(state *docStreamState) {
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	bus, err := application.UseService[engine.EventBus](ctx)
	if err != nil || bus == nil {
		return
	}

	// Parse the full accumulated JSON to extract the final content.
	var parsed struct {
		Title     string `json:"title"`
		MediaType string `json:"mediaType"`
		Content   string `json:"content"`
	}
	_ = jsonUnmarshal(state.accumulated, &parsed)

	title := parsed.Title
	if title == "" {
		title = state.title
	}
	mediaType := parsed.MediaType
	if mediaType == "" {
		mediaType = state.mediaType
	}

	evt := agentapi.DocumentEvent{
		ID:        state.id,
		Title:     title,
		MediaType: mediaType,
		Content:   parsed.Content,
	}
	evt.StampEvent(agentapi.EventDocument, p.run.get())
	bus.Publish(agentapi.EventDocument, evt)
}

// jsonUnmarshal is a package-local indirection for easier testing.
func jsonUnmarshal(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}

// emit publishes the accumulated deltas on the EventBus.
func (p *streamPlugin) emit(state *docStreamState, deltas []string) {
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	bus, err := application.UseService[engine.EventBus](ctx)
	if err != nil || bus == nil {
		return
	}

	// First emission: include title + media type so the GUI can open the tab.
	title, mediaType := "", ""
	if !state.metaEmitted && state.title != "" {
		title = state.title
		mediaType = state.mediaType
		state.metaEmitted = true
	}

	if title != "" || len(deltas) > 0 {
		joined := ""
		for _, d := range deltas {
			joined += d
		}
		evt := agentapi.DocumentDeltaEvent{
			ID:        state.id,
			Delta:     joined,
			Title:     title,
			MediaType: mediaType,
		}
		evt.StampEvent(agentapi.EventDocumentDelta, p.run.get())
		bus.Publish(agentapi.EventDocumentDelta, evt)
	}
}

// ── Partial-JSON state machine ─────────────────────────────────────────
// We can't json.Unmarshal partial JSON. Instead we scan the accumulated
// string and track:
//   - whether we've seen the "title" / "mediaType" / "content" keys
//   - the extracted values for title and mediaType (complete strings, small)
//   - the running offset inside the "content" string value so we can emit
//     only the new characters on each feed()
//
// Simplifications:
//   - We assume Claude/OpenAI emit well-formed JSON (no malformed escapes)
//   - We handle escape sequences inside strings (\n, \", \\, \t, \uXXXX)

type docStreamState struct {
	id              string
	accumulated     string // full JSON accumulated so far
	title           string
	mediaType       string
	contentStart    int // byte offset in accumulated where "content" value begins (just past opening quote)
	sawContentStart bool
	emittedDecoded  string // what we've already sent as deltas (decoded form), for diff comparison
	contentEndSeen  bool
	metaEmitted     bool
}

func newDocStreamState(id string) *docStreamState {
	return &docStreamState{id: id}
}

// feed appends new raw JSON and returns any newly-available content deltas.
// The decoded content grows monotonically across calls; we diff against what
// we've already emitted to produce the new chars.
func (s *docStreamState) feed(raw string) []string {
	s.accumulated += raw
	var deltas []string

	// 1. Extract title and mediaType (small complete strings).
	if s.title == "" {
		s.title = extractString(s.accumulated, "title")
	}
	if s.mediaType == "" {
		s.mediaType = extractString(s.accumulated, "mediaType")
	}

	// 2. Find the start of "content": " if not yet found.
	if !s.sawContentStart {
		idx := findStringValueStart(s.accumulated, "content")
		if idx < 0 {
			return deltas
		}
		s.sawContentStart = true
		s.contentStart = idx
	}

	// 3. Decode the full content span [contentStart : end] and diff against
	//    what we've already emitted. This avoids tracking parse cursors that
	//    get confused across chunk boundaries.
	end, complete := findStringValueEnd(s.accumulated, s.contentStart)
	endPos := end
	if !complete {
		endPos = len(s.accumulated)
	}

	if endPos > s.contentStart {
		decoded, _ := decodeJSONStringFragment(s.accumulated[s.contentStart:endPos])
		if len(decoded) > len(s.emittedDecoded) {
			newPart := decoded[len(s.emittedDecoded):]
			s.emittedDecoded = decoded
			deltas = append(deltas, newPart)
		}
	}

	if complete {
		s.contentEndSeen = true
	}

	return deltas
}

// extractString finds "<key>": "value" and returns the decoded value.
// Returns "" if not found or incomplete.
func extractString(s, key string) string {
	start := findStringValueStart(s, key)
	if start < 0 {
		return ""
	}
	end, complete := findStringValueEnd(s, start)
	if !complete {
		return ""
	}
	decoded, _ := decodeJSONStringFragment(s[start:end])
	return decoded
}

// findStringValueStart finds the opening quote of the value for `"<key>":`
// and returns the index just AFTER the opening quote of the value.
// Returns -1 if not found.
func findStringValueStart(s, key string) int {
	needle := `"` + key + `"`
	i := 0
	for i < len(s) {
		j := indexAt(s, needle, i)
		if j < 0 {
			return -1
		}
		// Skip to past the key
		k := j + len(needle)
		// Skip whitespace
		for k < len(s) && (s[k] == ' ' || s[k] == '\t' || s[k] == '\n' || s[k] == '\r') {
			k++
		}
		if k >= len(s) || s[k] != ':' {
			i = j + 1
			continue
		}
		k++ // past colon
		for k < len(s) && (s[k] == ' ' || s[k] == '\t' || s[k] == '\n' || s[k] == '\r') {
			k++
		}
		if k >= len(s) || s[k] != '"' {
			// Not a string value
			i = j + 1
			continue
		}
		return k + 1 // past opening quote
	}
	return -1
}

// findStringValueEnd scans from `start` (inside a JSON string value)
// and returns the index of the closing quote, plus whether it was found.
// Handles JSON string escapes: \" \\ \/ \b \f \n \r \t \uXXXX.
func findStringValueEnd(s string, start int) (int, bool) {
	i := start
	for i < len(s) {
		c := s[i]
		if c == '\\' {
			// Skip escape sequence
			if i+1 >= len(s) {
				return i, false
			}
			esc := s[i+1]
			if esc == 'u' {
				// \uXXXX — 4 hex digits
				if i+6 > len(s) {
					return i, false
				}
				i += 6
			} else {
				i += 2
			}
			continue
		}
		if c == '"' {
			return i, true
		}
		i++
	}
	return i, false
}

// decodeJSONStringFragment decodes a JSON string fragment (without
// surrounding quotes), handling escapes. Returns the decoded string
// and the number of source characters consumed (up to an incomplete
// escape at the end — we stop before it so next feed() can complete it).
func decodeJSONStringFragment(s string) (string, int) {
	var out []byte
	i := 0
	for i < len(s) {
		c := s[i]
		if c == '\\' {
			if i+1 >= len(s) {
				// Incomplete escape — stop before it
				return string(out), i
			}
			esc := s[i+1]
			switch esc {
			case 'n':
				out = append(out, '\n')
			case 't':
				out = append(out, '\t')
			case 'r':
				out = append(out, '\r')
			case '"':
				out = append(out, '"')
			case '\\':
				out = append(out, '\\')
			case '/':
				out = append(out, '/')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'u':
				if i+6 > len(s) {
					return string(out), i
				}
				// Decode \uXXXX
				var r rune
				for k := 0; k < 4; k++ {
					h := s[i+2+k]
					r <<= 4
					switch {
					case h >= '0' && h <= '9':
						r |= rune(h - '0')
					case h >= 'a' && h <= 'f':
						r |= rune(h-'a') + 10
					case h >= 'A' && h <= 'F':
						r |= rune(h-'A') + 10
					}
				}
				out = appendRune(out, r)
				i += 6
				continue
			default:
				out = append(out, esc)
			}
			i += 2
			continue
		}
		out = append(out, c)
		i++
	}
	return string(out), i
}

func appendRune(b []byte, r rune) []byte {
	if r < 0x80 {
		return append(b, byte(r))
	}
	var buf [4]byte
	n := encodeRune(buf[:], r)
	return append(b, buf[:n]...)
}

func encodeRune(p []byte, r rune) int {
	switch {
	case r < 0x80:
		p[0] = byte(r)
		return 1
	case r < 0x800:
		p[0] = 0xC0 | byte(r>>6)
		p[1] = 0x80 | byte(r&0x3F)
		return 2
	case r < 0x10000:
		p[0] = 0xE0 | byte(r>>12)
		p[1] = 0x80 | byte(r>>6&0x3F)
		p[2] = 0x80 | byte(r&0x3F)
		return 3
	default:
		p[0] = 0xF0 | byte(r>>18)
		p[1] = 0x80 | byte(r>>12&0x3F)
		p[2] = 0x80 | byte(r>>6&0x3F)
		p[3] = 0x80 | byte(r&0x3F)
		return 4
	}
}

// indexAt finds needle in s starting from index start, returns -1 if not found.
func indexAt(s, needle string, start int) int {
	if start >= len(s) {
		return -1
	}
	idx := indexOf(s[start:], needle)
	if idx < 0 {
		return -1
	}
	return start + idx
}

func indexOf(s, needle string) int {
	n := len(needle)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= len(s); i++ {
		if s[i:i+n] == needle {
			return i
		}
	}
	return -1
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
