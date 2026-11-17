// SPDX-License-Identifier: Apache-2.0

package session_read

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestSessionRead_RealJournal is a best-effort smoke on an ACTUAL run journal:
// it parses a real `llm-model-calls.jsonl` and logs the overview + a zoom, so a
// human can eyeball that the parsing holds on live streaming data. Skips when no
// journal is present (CI / fresh checkout). Pin one with KTW_TEST_JOURNAL=<path>.
func TestSessionRead_RealJournal(t *testing.T) {
	path := os.Getenv("KTW_TEST_JOURNAL")
	if path == "" {
		home, _ := os.UserHomeDir()
		for _, pat := range []string{
			filepath.Join(home, "AppData", "Local", "katyoti", "universes", "*", "sessions", "*", "llm-model-calls.jsonl"),
			filepath.Join(home, ".local", "share", "katyoti", "universes", "*", "sessions", "*", "llm-model-calls.jsonl"),
		} {
			if m, _ := filepath.Glob(pat); len(m) > 0 {
				path = m[0]
				break
			}
		}
	}
	if path == "" {
		t.Skip("no real session journal found (set KTW_TEST_JOURNAL to pin one)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip(err.Error())
	}
	r := parseRun(splitLines(string(data)))
	if len(r.turns) == 0 {
		t.Skip("empty journal: " + path)
	}
	ov := overviewMode(r)
	t.Logf("journal: %s", path)
	t.Logf("OVERVIEW: %d turns · total tool calls=%d · distinct=%d · system_prompt=%d chars",
		ov.Turns, ov.ToolCallTotal, ov.ToolsDistinct, ov.SystemPromptLen)
	t.Logf("histogram: %v", ov.Histogram)
	if ov.Toolset != nil {
		t.Logf("TOOLSET: given=%d used=%d never_used=%d", ov.Toolset.Given, ov.Toolset.Used, len(ov.Toolset.NeverUsed))
	}
	for i, b := range ov.Index {
		if i >= 6 {
			t.Logf("  … (%d more turns)", len(ov.Index)-6)
			break
		}
		t.Logf("  turn %d (%dms) tools=%v %q", b.Turn, b.DurationMs, b.Tools, b.Summary)
	}
	if d, err := detailMode(r, "0"); err == nil && len(d.Detail) == 1 {
		t.Logf("ZOOM turn 0: system_prompt=%d chars, catalog=%d tools, %d tool calls",
			len(d.Detail[0].SystemPrompt), len(d.Detail[0].Tools), len(d.Detail[0].ToolCalls))
	}
}

func TestDecodeReqBody(t *testing.T) {
	// Plain object.
	if m := decodeReqBody(json.RawMessage(`{"model":"x"}`)); m["model"] != "x" {
		t.Errorf("object decode failed: %v", m)
	}
	// JSON string carrying JSON (provider variant).
	if m := decodeReqBody(json.RawMessage(`"{\"model\":\"y\"}"`)); m["model"] != "y" {
		t.Errorf("string-wrapped decode failed: %v", m)
	}
	if decodeReqBody(nil) != nil {
		t.Error("empty should be nil")
	}
}

func TestExtractSystem(t *testing.T) {
	if got := extractSystem(map[string]any{"system": "hello"}); got != "hello" {
		t.Errorf("string system = %q", got)
	}
	blocks := map[string]any{"system": []any{
		map[string]any{"type": "text", "text": "a"},
		map[string]any{"type": "text", "text": "b"},
	}}
	if got := extractSystem(blocks); got != "a\nb" {
		t.Errorf("block system = %q", got)
	}
	openai := map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": "sys"},
		map[string]any{"role": "user", "content": "hi"},
	}}
	if got := extractSystem(openai); got != "sys" {
		t.Errorf("message-role system = %q", got)
	}
}

func TestExtractToolNames(t *testing.T) {
	anthropic := []any{
		map[string]any{"name": "file-read"},
		map[string]any{"name": "complete"},
	}
	if got := extractToolNames(anthropic); !reflect.DeepEqual(got, []string{"file-read", "complete"}) {
		t.Errorf("anthropic tools = %v", got)
	}
	openai := []any{
		map[string]any{"function": map[string]any{"name": "web-search"}},
	}
	if got := extractToolNames(openai); !reflect.DeepEqual(got, []string{"web-search"}) {
		t.Errorf("openai tools = %v", got)
	}
}

func TestCallEndContents(t *testing.T) {
	// Streamed: from the SSE summary.
	streamed := callEvent{Kind: "call_end", SSESummary: &sseSummary{Contents: []sseContent{
		{Type: "text", Text: "ok"},
		{Type: "tool_use", Name: "file-read", Input: json.RawMessage(`{"path":"a"}`)},
	}}}
	c := callEndContents(streamed)
	if len(c) != 2 || c[1].Name != "file-read" {
		t.Errorf("streamed contents = %v", c)
	}
	// Non-streamed: from the raw Anthropic response body.
	raw := callEvent{Kind: "call_end", ResponseBody: json.RawMessage(
		`{"content":[{"type":"tool_use","name":"complete","input":{}}]}`)}
	c = callEndContents(raw)
	if len(c) != 1 || c[0].Name != "complete" {
		t.Errorf("raw contents = %v", c)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Error("short unchanged")
	}
	if got := truncate("hello world", 5); got != "hello…" {
		t.Errorf("truncate = %q", got)
	}
}

func TestParseRange(t *testing.T) {
	cases := []struct {
		spec    string
		n       int
		lo, hi  int
		wantErr bool
	}{
		{"41", 100, 41, 41, false},
		{"40-60", 100, 40, 60, false},
		{"60-40", 100, 40, 60, false},  // swapped
		{"40-200", 100, 40, 99, false}, // hi clamped
		{"500", 100, 0, 0, true},       // lo out of range
		{"x", 100, 0, 0, true},
	}
	for _, c := range cases {
		lo, hi, err := parseRange(c.spec, c.n)
		if (err != nil) != c.wantErr {
			t.Errorf("%q: err=%v wantErr=%v", c.spec, err, c.wantErr)
			continue
		}
		if !c.wantErr && (lo != c.lo || hi != c.hi) {
			t.Errorf("%q: got [%d,%d] want [%d,%d]", c.spec, lo, hi, c.lo, c.hi)
		}
	}
}

// fixtureRun is a 2-turn journal: turn 0 calls file-read; turn 1 calls
// file-read (again — redundancy) + complete.
var fixtureRun = []string{
	`{"kind":"call_start","call_id":"c0","model":"claude","request_body":{"system":"You are X","tools":[{"name":"file-read"},{"name":"complete"}]}}`,
	`{"kind":"call_end","call_id":"c0","duration_ms":100,"sse_summary":{"contents":[{"type":"text","text":"reading"},{"type":"tool_use","name":"file-read","input":{"path":"a"}}]}}`,
	`{"kind":"call_start","call_id":"c1","model":"claude","request_body":{}}`,
	`{"kind":"call_end","call_id":"c1","duration_ms":50,"sse_summary":{"contents":[{"type":"tool_use","name":"file-read","input":{"path":"a"}},{"type":"tool_use","name":"complete","input":{}}]}}`,
}

func TestSessionRead_Overview(t *testing.T) {
	r := parseRun(fixtureRun)
	if len(r.turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(r.turns))
	}
	out := overviewMode(r)
	if out.ToolCallTotal != 3 || out.ToolsDistinct != 2 {
		t.Errorf("total=%d distinct=%d, want 3/2 (redundancy)", out.ToolCallTotal, out.ToolsDistinct)
	}
	if out.Histogram["file-read"] != 2 {
		t.Errorf("histogram file-read = %d, want 2", out.Histogram["file-read"])
	}
	if len(out.Index) != 2 || out.Index[0].Summary != "reading" {
		t.Errorf("index = %+v", out.Index)
	}
	if out.SystemPromptLen != len("You are X") {
		t.Errorf("system_prompt_chars = %d", out.SystemPromptLen)
	}
	if len(out.Detail) != 0 {
		t.Error("overview must not include detail")
	}
	// Both catalog tools (file-read, complete) were used → no over-provisioning.
	if out.Toolset == nil || out.Toolset.Given != 2 || out.Toolset.Used != 2 || len(out.Toolset.NeverUsed) != 0 {
		t.Errorf("toolset = %+v, want given=2 used=2 never_used=[]", out.Toolset)
	}
}

// overProvisionedRun gives a 3-tool catalog (file-read, complete, web-search)
// but only ever calls file-read — web-search + complete are dead weight.
var overProvisionedRun = []string{
	`{"kind":"call_start","call_id":"c0","model":"claude","request_body":{"tools":[{"name":"file-read"},{"name":"complete"},{"name":"web-search"}]}}`,
	`{"kind":"call_end","call_id":"c0","duration_ms":10,"sse_summary":{"contents":[{"type":"tool_use","name":"file-read","input":{"path":"a"}}]}}`,
}

func TestSessionRead_Toolset(t *testing.T) {
	out := overviewMode(parseRun(overProvisionedRun))
	ts := out.Toolset
	if ts == nil {
		t.Fatal("toolset missing")
	}
	if ts.Given != 3 || ts.Used != 1 {
		t.Errorf("given=%d used=%d, want 3/1", ts.Given, ts.Used)
	}
	want := []string{"complete", "web-search"}
	if !reflect.DeepEqual(ts.NeverUsed, want) {
		t.Errorf("never_used = %v, want %v", ts.NeverUsed, want)
	}
}

func TestSessionRead_Detail(t *testing.T) {
	r := parseRun(fixtureRun)
	out, err := detailMode(r, "0")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Detail) != 1 {
		t.Fatalf("detail turns = %d", len(out.Detail))
	}
	d := out.Detail[0]
	if d.SystemPrompt != "You are X" || len(d.Tools) != 2 {
		t.Errorf("turn-0 detail missing cold prompt: %+v", d)
	}
	if len(d.ToolCalls) != 1 || d.ToolCalls[0].Name != "file-read" {
		t.Errorf("turn-0 tool calls = %+v", d.ToolCalls)
	}
}

func TestSessionRead_Filter(t *testing.T) {
	r := parseRun(fixtureRun)
	// Only turn 1 invoked `complete`.
	out := filterMode(r, "complete", "")
	if len(out.Index) != 1 || out.Index[0].Turn != 1 {
		t.Errorf("filter by tool = %+v", out.Index)
	}
	// grep on a tool input.
	out = filterMode(r, "", "path")
	if len(out.Index) != 2 {
		t.Errorf("grep 'path' should hit both turns, got %+v", out.Index)
	}
}
