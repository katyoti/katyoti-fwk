// SPDX-License-Identifier: Apache-2.0

package diagram_emit

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	drawiotesting "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/testing"
)

func TestCompleteJSON(t *testing.T) {
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{`{"diagram":{"viewpoint":"v","nodes":[{"id":"a"`, "", false},
		{`{"diagram":{"viewpoint":"v","nodes":[{"id":"a"},{"id":"b`, `{"diagram":{"viewpoint":"v","nodes":[{"id":"a"}]}}`, true},
		// An object inside an element (attributes) is not a cut point.
		{`{"diagram":{"nodes":[{"id":"a","attributes":{"x":1}},{"id":"b","attributes":{"y":[{"c":1}]`, `{"diagram":{"nodes":[{"id":"a","attributes":{"x":1}}]}}`, true},
		// Braces inside strings do not count.
		{`{"diagram":{"nodes":[{"id":"a","label":"}{]["},{"id`, `{"diagram":{"nodes":[{"id":"a","label":"}{]["}]}}`, true},
	} {
		got, ok := completeJSON(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("completeJSON(%s) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
		if ok {
			var v any
			if err := json.Unmarshal([]byte(got), &v); err != nil {
				t.Errorf("not valid JSON: %s: %v", got, err)
			}
		}
	}
}

// TestStream_DrawsAsItStreamsAndHandsTheToolTheSameDrawing: a diagram-emit
// call fed in small chunks is drawn more than once while it streams, and
// the tool returns exactly the drawing the stream finished with.
func TestStream_DrawsAsItStreamsAndHandsTheToolTheSameDrawing(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun not on PATH: the first drawing is laid out by ELK")
	}
	raw, err := os.ReadFile("../../drawio/render/testdata/corpus/c4-internet-banking.json")
	if err != nil {
		t.Fatal(err)
	}
	var ld drawioapi.LogicalDiagram
	if err := json.Unmarshal(raw, &ld); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(Input{Diagram: &ld})

	p := newStreamPlugin()
	p.ctx = drawiotesting.WithCatalog(t, drawiotesting.TestCatalog(t))
	_ = p.onStream(nil, agentapi.StreamChunk{Type: "tool_use_start", ToolID: "t1", ToolName: "diagram-emit"})
	drawings, seen := 0, ""
	for i := 0; i < len(input); i += 40 {
		end := min(i+40, len(input))
		_ = p.onStream(nil, agentapi.StreamChunk{Type: "tool_use_delta", ToolID: "t1", ToolInput: string(input[i:end])})
		p.mu.Lock()
		st := p.states["t1"]
		p.mu.Unlock()
		if st.lastXML != seen {
			drawings++
			seen = st.lastXML
			st.lastAt = st.lastAt.Add(-checkpointEvery) // no wall-clock wait in a test
		}
	}
	if drawings < 3 {
		t.Errorf("drawn %d times while streaming, want several", drawings)
	}
	_ = p.onStream(nil, agentapi.StreamChunk{Type: "tool_use_done", ToolID: "t1"})

	var in Input
	_ = json.Unmarshal(input, &in)
	final, ok := recall(in)
	if !ok || final.XML == "" {
		t.Fatal("the stream left no drawing for the tool")
	}
	remember(in, final)
	out, err := executeDiagramEmit(p.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if out.XML != final.XML {
		t.Error("the tool returned a different drawing from the one the tab shows")
	}
}
