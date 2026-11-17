// SPDX-License-Identifier: Apache-2.0

package display_ui_component

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// streamPlugin watches the LLM stream for `display-ui-component` tool-use
// deltas and emits artifact-store events progressively as the JSON args
// stream in. Per decision (modules/display/decisions), this
// is the streaming mechanism — no separate wire protocol; we ride the
// LLM SDK's native tool-call arg streaming.
//
// Per D5 of display-dsl-framework's ANALYZE phase, we use TREE-LEVEL
// granularity for v1: try to parse the partial JSON on each delta (with
// auto-close to repair unclosed brackets); when parse succeeds, mount
// the artifact (first time) or replace its def (subsequent updates).
// Fine-grained array-element streaming is deferred to a follow-up.

type streamPlugin struct {
	ctx    context.Context
	run    run
	mu     sync.Mutex
	accums map[string]*displayState // keyed by tool_id
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

func newStreamPlugin() *streamPlugin {
	return &streamPlugin{accums: make(map[string]*displayState)}
}

// AgentPlugin interface implementation (we only contribute OnModelStream).

func (p *streamPlugin) Name() string                 { return "display-ui-component-stream" }
func (p *streamPlugin) SystemMessage() string        { return "" }
func (p *streamPlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (p *streamPlugin) Stable() bool                 { return true }
func (p *streamPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	return nil, nil
}
func (p *streamPlugin) Hooks() agentapi.PluginHooks {
	return agentapi.PluginHooks{OnModelStream: p.onStream}
}

// displayState — per-tool-call accumulator.
type displayState struct {
	toolID string
	// raw is the accumulated JSON-args string. The LLM streams pieces
	// of the input JSON across tool_use_delta chunks; we concatenate.
	raw strings.Builder
	// mounted is true after we've published a UIArtifactMountEvent for
	// this artifact. Subsequent successful re-parses publish patches.
	mounted bool
	// artifactID is captured on first successful parse so we can patch
	// even if later parses temporarily fail.
	artifactID string
	// kind is captured similarly. Once set, it shouldn't change (the
	// LLM emits a single tool call for one artifact).
	kind string
	// lastEmittedDef is the last def we sent (mount payload or replace
	// patch value). Used to diff: if the new parse equals the last,
	// suppress the emit. JSON-marshaled for stable byte comparison.
	lastEmittedDef []byte
	// persistent — if true, omit the auto-complete on tool_use_done.
	persistent bool
	// file-display cache: a drawio `file` ref is read host-side ONCE and the
	// XML reused across deltas (tryEmit fires per delta).
	fileResolvedPath string
	fileXML          string
	fileName         string
}

// resolveFile reads a drawio `file` ref once and caches the result, so the LLM
// can display an existing diagram by path without emitting its content.
func (s *displayState) resolveFile(ctx context.Context, path string) (xml, filename string) {
	if s.fileResolvedPath == path {
		return s.fileXML, s.fileName
	}
	s.fileResolvedPath = path
	xml, fname, err := resolveDocumentFile(ctx, path)
	if err != nil {
		s.fileXML, s.fileName = "", ""
		return "", ""
	}
	s.fileXML, s.fileName = xml, fname
	return xml, fname
}

func newDisplayState(toolID string) *displayState {
	return &displayState{toolID: toolID}
}

// onStream is called for every LLM stream chunk. We only act on chunks
// whose tool name matches `display-ui-component`.
func (p *streamPlugin) onStream(loop agentapi.LoopContext, chunk agentapi.StreamChunk) error {
	if loop != nil {
		p.run.set(loop.RunID())
	}
	switch chunk.Type {
	case "tool_use_start":
		if chunk.ToolName == "display-ui-component" {
			p.mu.Lock()
			p.accums[chunk.ToolID] = newDisplayState(chunk.ToolID)
			p.mu.Unlock()
		}
		return nil

	case "tool_use_delta":
		p.mu.Lock()
		state, ok := p.accums[chunk.ToolID]
		p.mu.Unlock()
		if !ok {
			return nil
		}
		state.raw.WriteString(chunk.ToolInput)
		p.tryEmit(state)
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
		// Final emit — by now the JSON should parse cleanly without
		// auto-close. Mount if not already, then auto-complete unless
		// persistent.
		p.tryEmit(state)
		if state.mounted && !state.persistent {
			p.publishComplete(state.artifactID)
		}
		return nil
	}
	return nil
}

// tryEmit attempts to parse the accumulated args (with auto-close
// fallback) and emit mount/patch events when the parsed def differs
// from the last emission.
func (p *streamPlugin) tryEmit(state *displayState) {
	parsed, ok := tryParseWithAutoClose(state.raw.String())
	if !ok {
		return
	}

	// Pull discriminator fields. We need at least artifactId + kind
	// before any emission is meaningful.
	artifactID, _ := parsed["artifactId"].(string)
	kind, _ := parsed["kind"].(string)
	if artifactID == "" || kind == "" {
		return
	}
	if state.artifactID == "" {
		state.artifactID = artifactID
	}
	if state.kind == "" {
		state.kind = kind
	}
	if persistent, _ := parsed["persistent"].(bool); persistent {
		state.persistent = true
	}

	// Coerce structured fields the LLM sometimes emits as a JSON STRING
	// instead of an inline object/array. Without this, a stringified
	// dslTree flows downstream unchanged and the page renderer reports
	// "Unknown node kind" because `node.kind` is undefined on a string.
	for _, k := range []string{"dslTree", "data", "schema", "ui"} {
		if val, ok := parsed[k]; ok {
			parsed[k] = coerceStructuredField(val)
		}
	}

	// File-display: for a drawio `file` ref with no inline xml, read the file
	// host-side (cached) and inline the XML so the artifact renders. The LLM
	// passes only the path — no token spend on the diagram content.
	if state.kind == "drawio" {
		if f, _ := parsed["file"].(string); strings.TrimSpace(f) != "" && !hasDrawioXML(parsed["data"]) {
			if xml, fname := state.resolveFile(p.ctx, f); xml != "" {
				parsed["data"] = map[string]any{"xml": xml, "filename": fname}
			}
		}
	}
	// A deck `file` ref: the markdown is read host-side the same way.
	if state.kind == "deck" {
		if f, _ := parsed["file"].(string); strings.TrimSpace(f) != "" && parsed["data"] == nil {
			if md, _ := state.resolveFile(p.ctx, f); md != "" {
				parsed["data"] = md
			}
		}
	}

	// Build the def payload — what the artifact-store mounts/replaces.
	// We pass through the parsed map (minus artifactId/persistent/title
	// which are mount metadata, not part of the renderable def).
	def := buildDefPayload(parsed)

	// Diff against last emission to suppress no-op emits.
	defBytes, err := json.Marshal(def)
	if err != nil {
		return
	}
	if bytes.Equal(defBytes, state.lastEmittedDef) {
		return
	}

	if !state.mounted {
		p.publishMount(state, parsed, def)
		state.mounted = true
	} else {
		p.publishPatch(state.artifactID, def)
	}
	state.lastEmittedDef = defBytes
}

// coerceStructuredField returns v unchanged unless it is a string that
// looks like a JSON object or array — in that case it parses it and
// returns the structure. LLMs occasionally emit a nested structured
// field (dslTree / data / schema / ui) as a JSON STRING
// rather than an inline value; this recovers the intended object so the
// renderer sees a real node tree, not a string. A parse failure (e.g.
// a still-incomplete partial during streaming) leaves v untouched.
func coerceStructuredField(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	t := strings.TrimSpace(s)
	if t == "" || (t[0] != '{' && t[0] != '[') {
		return v
	}
	var parsed any
	if err := json.Unmarshal([]byte(t), &parsed); err != nil {
		return v
	}
	return parsed
}

// buildDefPayload extracts the renderable def from the parsed args,
// stripping mount-metadata fields that don't belong in the def.
func buildDefPayload(parsed map[string]any) map[string]any {
	def := make(map[string]any, len(parsed))
	for k, v := range parsed {
		switch k {
		case "artifactId", "persistent", "title", "file":
			// mount metadata / input-only fields — not part of the def.
			// `file` is resolved into data before this point.
		default:
			def[k] = v
		}
	}
	return def
}

// hasDrawioXML reports whether a parsed data already carries a non-empty
// xml string (so a `file` ref doesn't clobber an explicitly-provided diagram).
func hasDrawioXML(data any) bool {
	m, ok := data.(map[string]any)
	if !ok {
		return false
	}
	xml, _ := m["xml"].(string)
	return strings.TrimSpace(xml) != ""
}

// publishMount emits a UIArtifactMountEvent. The translator hands it to
// the chat panel bridge, which adds the artifact's node to the run's chat
// panel.
func (p *streamPlugin) publishMount(state *displayState, parsed, def map[string]any) {
	bus := p.bus()
	if bus == nil {
		return
	}
	title, _ := parsed["title"].(string)
	mode, _ := parsed["mode"].(string)
	dslTree := parsed["dslTree"]
	// A component's one `data` field is a page's outer data tree (its
	// dataField bindings read it) and any other component's value.
	var value, data any
	if state.kind == "page" {
		data = def["data"]
	} else {
		value = def["data"]
	}
	variant, _ := def["variant"].(string)
	ui, _ := def["ui"].(map[string]any)

	evt := agentapi.UIArtifactMountEvent{
		ArtifactID: state.artifactID,
		Kind:       state.kind,
		Schema:     def["schema"], // an AST inline, or a data-type URI the GUI resolves
		Variant:    variant,
		UI:         ui,
		Value:      value,
		DSLTree:    dslTree,
		Data:       data,
		Mode:       mode,
		Title:      title,
		Actions:    parsed["actions"], // Mode-B declarative shortcuts (US-S3)
	}
	evt.StampEvent(agentapi.EventUIArtifactMount, p.run.get())
	bus.Publish(agentapi.EventUIArtifactMount, evt)
}

// publishPatch emits a UIArtifactPatchEvent that REPLACES the entire
// def. Tree-level granularity (D5): we always emit a full-replace, never
// per-field patches. Frontend's React diff handles the re-render
// efficiently.
func (p *streamPlugin) publishPatch(artifactID string, def map[string]any) {
	bus := p.bus()
	if bus == nil {
		return
	}
	// Use a single replace op against the artifact's "" path (i.e. the
	// whole def root). The chat panel bridge applies it as a def replace.
	evt := agentapi.UIArtifactPatchEvent{
		ArtifactID: artifactID,
		Ops: []any{
			map[string]any{
				"kind":  "replace",
				"path":  "",
				"value": def,
			},
		},
	}
	evt.StampEvent(agentapi.EventUIArtifactPatch, p.run.get())
	bus.Publish(agentapi.EventUIArtifactPatch, evt)
}

// publishComplete emits a UIArtifactCompleteEvent on tool_use_done
// when the artifact is non-persistent.
func (p *streamPlugin) publishComplete(artifactID string) {
	bus := p.bus()
	if bus == nil {
		return
	}
	evt := agentapi.UIArtifactCompleteEvent{ArtifactID: artifactID}
	evt.StampEvent(agentapi.EventUIArtifactComplete, p.run.get())
	bus.Publish(agentapi.EventUIArtifactComplete, evt)
}

func (p *streamPlugin) bus() engine.EventBus {
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	bus, err := application.UseService[engine.EventBus](ctx)
	if err != nil {
		return nil
	}
	return bus
}

// ── Partial-JSON parsing with auto-close ─────────────────────────────────
//
// The LLM SDK streams tool-call args as partial JSON. We attempt to parse
// the accumulated raw bytes on each delta. If the raw bytes form a valid
// JSON object, parse directly. Otherwise we scan the bytes tracking
// bracket depth + string state, then append the right closing characters
// to form a candidate. If the candidate parses, we use it.
//
// This is a best-effort heuristic. Failed parses simply skip the emission
// and wait for the next delta. The final tool_use_done always fires with
// the complete (well-formed) JSON, so the worst case is "artifact appears
// at end" — same UX as a non-streaming tool.

// tryParseWithAutoClose attempts to parse `raw` as JSON. Returns the
// parsed object and a success flag.
func tryParseWithAutoClose(raw string) (map[string]any, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return nil, false
	}

	// Direct parse first — when the LLM's stream happens to land at a
	// natural close.
	if out, ok := tryParse(raw); ok {
		return out, true
	}

	// Auto-close: walk the bytes, track bracket depth + string state,
	// append closing chars to form a candidate.
	candidate, ok := buildAutoClosed(raw)
	if !ok {
		return nil, false
	}
	if out, ok := tryParse(candidate); ok {
		return out, true
	}
	return nil, false
}

func tryParse(s string) (map[string]any, bool) {
	var out map[string]any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, false
	}
	return out, true
}

// buildAutoClosed scans the raw string for unclosed brackets and string
// state, then constructs a balanced candidate that is most-likely JSON.
//
// Heuristics applied:
//   - Track a stack of expected closers ('}' for {, ']' for [).
//   - Track string state to ignore brackets inside string literals.
//   - If the scan ends mid-string, drop the trailing partial string +
//     trailing `:"key"` if present (we can't auto-close a string mid-
//     escape sequence reliably).
//   - Strip trailing `,` (a value-after-comma is missing).
//   - If the scan ends right after `:` (a value is expected), append
//     `null` then close.
//   - Append the stack's closers in reverse.
//
// On parser fragility: if the input ends inside a top-level value being
// emitted (e.g. `{"artifactId": "art-` mid-string), we strip back to the
// last preceding `,` or opening `{`/`[` and close. The trade-off: we
// might lose the LAST partial value but keep all earlier complete ones.
func buildAutoClosed(raw string) (string, bool) {
	stack := make([]byte, 0, 16) // closing characters expected
	inString := false
	escape := false

	// Track the last "safe truncate" position — the index at which the
	// JSON would still be parseable (after appropriate close chars).
	// Updated when we close a value or finish an object/array.
	safeTruncate := -1
	strStart := -1 // index of the opening quote of the string being scanned

	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if escape {
			escape = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escape = true
			case '"':
				inString = false
				safeTruncate = i + 1 // string just closed
			}
			continue
		}
		switch c {
		case '"':
			inString = true
			strStart = i
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 && stack[len(stack)-1] == c {
				stack = stack[:len(stack)-1]
			} else {
				return "", false // bracket mismatch — give up
			}
			safeTruncate = i + 1
		case ',', ':':
			// punctuation — no nesting change
		default:
			// numbers, booleans, null, whitespace
			if !isJSONLiteralByte(c) && !isWhitespace(c) {
				// Unknown char outside a string — not valid JSON
				return "", false
			}
		}
	}

	// A document kind's content is ONE string — a deck's markdown — in the
	// top-level `data`. Dropping it while it is open would show nothing until
	// the whole document has streamed, so that one string is closed where it
	// stands (a dangling escape trimmed first). No other string is: a partial
	// key, or a partial `kind`, must never be emitted.
	if inString && len(stack) == 1 && isTopLevelDataValue(raw, strStart) {
		body := raw
		if escape {
			body = body[:len(body)-1] // a lone trailing backslash
		}
		body = partialUnicodeEscape.ReplaceAllString(body, "")
		return body + `"}`, true
	}

	// Find a candidate to truncate to. If we ended in a string, drop the
	// partial string (and any preceding key:colon if applicable). If we
	// ended after an unfinished value (e.g. `:`), append null.
	candidate := raw
	if inString {
		// Strip back to safeTruncate (the last position before the
		// string opened). Then strip a trailing `:"key"` or `:` so we
		// don't leave an orphan key.
		if safeTruncate < 0 {
			return "", false
		}
		candidate = raw[:safeTruncate]
		// stack at this point reflects the OPEN brackets at i==len(raw),
		// but we need to recompute since we truncated. Simplest: re-scan
		// the truncated candidate.
		stack = nil
		inString = false
		escape = false
		for i := 0; i < len(candidate); i++ {
			c := candidate[i]
			if escape {
				escape = false
				continue
			}
			if inString {
				if c == '\\' {
					escape = true
				} else if c == '"' {
					inString = false
				}
				continue
			}
			switch c {
			case '"':
				inString = true
			case '{':
				stack = append(stack, '}')
			case '[':
				stack = append(stack, ']')
			case '}', ']':
				if len(stack) > 0 && stack[len(stack)-1] == c {
					stack = stack[:len(stack)-1]
				}
			}
		}
	}

	// Strip trailing whitespace + dangling comma + dangling key:
	candidate = strings.TrimRightFunc(candidate, isAnyTrailing)
	// If the candidate ends with `:` we owe a value — append null.
	if strings.HasSuffix(candidate, ":") {
		candidate += "null"
	}
	// If the candidate ends with a `"key"` (key without colon), drop it.
	// stripDanglingKey may leave a trailing `,` (the comma BEFORE the
	// dropped key) — re-trim afterwards.
	candidate = stripDanglingKey(candidate)
	candidate = strings.TrimRightFunc(candidate, isAnyTrailing)

	// Append closers in reverse stack order.
	closers := make([]byte, len(stack))
	for i := range stack {
		closers[i] = stack[len(stack)-1-i]
	}
	candidate += string(closers)
	return candidate, true
}

// partialUnicodeEscape is a `\uXXXX` escape cut before its fourth hex digit
// at the end of a streamed string.
var partialUnicodeEscape = regexp.MustCompile(`\\u[0-9a-fA-F]{0,3}$`)

// isTopLevelDataValue reports whether the string opening at raw[quote] is the
// value of the top-level `"data"` key: `"data"` then `:` before it, spaces
// allowed.
func isTopLevelDataValue(raw string, quote int) bool {
	if quote <= 0 {
		return false
	}
	i := quote - 1
	for i >= 0 && isWhitespace(raw[i]) {
		i--
	}
	if i < 0 || raw[i] != ':' {
		return false
	}
	i--
	for i >= 0 && isWhitespace(raw[i]) {
		i--
	}
	return i >= 5 && raw[i-5:i+1] == `"data"`
}

func isJSONLiteralByte(c byte) bool {
	// Loose check — covers digits, +/-, .eE, and letters of true/false/null.
	return (c >= '0' && c <= '9') || c == '-' || c == '+' || c == '.' ||
		c == 'e' || c == 'E' ||
		c == 't' || c == 'r' || c == 'u' || c == 'f' || c == 'a' || c == 'l' || c == 's' || c == 'n'
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isAnyTrailing(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', ',':
		return true
	}
	return false
}

// stripDanglingKey removes a trailing `"key"` (an open key without its
// `:value`). Pattern: any whitespace, then `"key"` after a `,` or `{`.
func stripDanglingKey(s string) string {
	// Find the last unescaped `"`. If the substring after the matching
	// open quote contains no `:` outside strings, it's a dangling key.
	if !strings.HasSuffix(s, "\"") {
		return s
	}
	// Scan backwards to find the opening quote.
	for i := len(s) - 2; i >= 0; i-- {
		if s[i] == '"' && (i == 0 || s[i-1] != '\\') {
			// s[i:] is the candidate key. Check what precedes whitespace.
			j := i - 1
			for j >= 0 && (s[j] == ' ' || s[j] == '\t') {
				j--
			}
			if j < 0 {
				return s // can't strip without breaking
			}
			if s[j] == ',' || s[j] == '{' {
				return s[:j+1] // strip the dangling key
			}
			return s
		}
	}
	return s
}

// ensure reflect is referenced (kept for future deep-equal of defs).
var _ = reflect.DeepEqual

// ensure fmt is referenced (used by the tool's describer indirectly).
var _ = fmt.Sprintf
