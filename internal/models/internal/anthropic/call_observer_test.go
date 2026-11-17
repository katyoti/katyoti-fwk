// SPDX-License-Identifier: Apache-2.0

package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/katyoti/katyoti-fwk/internal/models/api"
)

// recordingObserver captures ModelCallEvents (SSE events arrive from
// the stream-consumer goroutine, hence the mutex).
type recordingObserver struct {
	mu     sync.Mutex
	events []api.ModelCallEvent
}

func (r *recordingObserver) ObserveModelCall(evt api.ModelCallEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, evt)
}

func (r *recordingObserver) snapshot() []api.ModelCallEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]api.ModelCallEvent(nil), r.events...)
}

func (r *recordingObserver) byKind(kind string) []api.ModelCallEvent {
	var out []api.ModelCallEvent
	for _, e := range r.snapshot() {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// TestCallObserver_NonStreaming pins the JSONL schema for a plain JSON
// call: call_start (provider/model/method/url, scrubbed headers,
// decoded request body) + call_end (status, decoded response body,
// duration) sharing one call_id — and every event round-trips through
// JSON marshal (the file format).
func TestCallObserver_NonStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","model":"claude-test",` +
			`"content":[{"type":"text","text":"OK"}],"stop_reason":"end_turn",` +
			`"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	llm, err := New("sk-test-secret-key", "claude-test", false, option.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	obs := &recordingObserver{}
	resp, err := llm.GenerateContent(context.Background(),
		[]api.MessageContent{api.TextParts(api.ChatMessageTypeHuman, "hi")},
		api.WithMaxTokens(16),
		api.WithCallObserver(obs),
	)
	if err != nil {
		t.Fatalf("GenerateContent: %v", err)
	}
	if len(resp.Choices) == 0 || !strings.Contains(resp.Choices[0].Content, "OK") {
		t.Fatalf("unexpected response: %+v", resp)
	}

	starts := obs.byKind("call_start")
	ends := obs.byKind("call_end")
	if len(starts) != 1 || len(ends) != 1 {
		t.Fatalf("events = %d starts / %d ends, want 1/1", len(starts), len(ends))
	}
	start, end := starts[0], ends[0]

	if start.CallID == "" || start.CallID != end.CallID {
		t.Errorf("call_id grouping broken: start=%q end=%q", start.CallID, end.CallID)
	}
	if start.Provider != "anthropic" || start.Method != "POST" || !strings.Contains(start.URL, "/v1/messages") {
		t.Errorf("start identity = %+v", start)
	}
	if start.Model != "claude-test" {
		t.Errorf("model from request body = %q", start.Model)
	}
	if got := start.RequestHeaders["X-Api-Key"]; got != "***" {
		t.Errorf("api key not scrubbed: %q", got)
	}
	var reqBody map[string]any
	if err := json.Unmarshal(start.RequestBody, &reqBody); err != nil || reqBody["max_tokens"] != float64(16) {
		t.Errorf("request body not decoded JSON: %s (%v)", start.RequestBody, err)
	}
	if end.Status != 200 {
		t.Errorf("status = %d", end.Status)
	}
	var respBody map[string]any
	if err := json.Unmarshal(end.ResponseBody, &respBody); err != nil || respBody["id"] != "msg_test" {
		t.Errorf("response body not decoded JSON: %s (%v)", end.ResponseBody, err)
	}

	// The file format: every event marshals, and no marshaled line
	// contains the secret.
	for i, e := range obs.snapshot() {
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("event %d does not marshal: %v", i, err)
		}
		if strings.Contains(string(line), "sk-test-secret-key") {
			t.Fatalf("event %d leaks the API key: %s", i, line)
		}
	}
}

// TestCallObserver_Streaming pins the SSE handling: structural frames
// become sse_event lines, per-token deltas are aggregated into
// call_end.sse_summary (count + assembled text), and the consumer still
// receives the full stream.
func TestCallObserver_Streaming(t *testing.T) {
	frames := []string{
		`{"type":"message_start","message":{"model":"claude-test","usage":{"input_tokens":3,"output_tokens":1}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"file-write","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"x.md\"}"}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, f := range frames {
			var probe struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal([]byte(f), &probe)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", probe.Type, f)
		}
	}))
	defer srv.Close()

	llm, err := New("sk-test-secret-key", "claude-test", false, option.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	obs := &recordingObserver{}
	stream, err := llm.GenerateContentStream(context.Background(),
		[]api.MessageContent{api.TextParts(api.ChatMessageTypeHuman, "hi")},
		api.WithMaxTokens(16),
		api.WithCallObserver(obs),
	)
	if err != nil {
		t.Fatalf("GenerateContentStream: %v", err)
	}
	var text string
	for stream.Next() {
		ev := stream.Event()
		if ev.Type == api.StreamEventTextDelta {
			text += ev.Delta
		}
	}
	if text != "Hello" {
		t.Fatalf("consumer text = %q, want Hello (observer must not eat the stream)", text)
	}

	// call_end fires from the tee goroutine when the stream drains —
	// poll briefly.
	var ends []api.ModelCallEvent
	for i := 0; i < 200 && len(ends) == 0; i++ {
		ends = obs.byKind("call_end")
	}
	if len(ends) != 1 {
		t.Fatalf("call_end events = %d, want 1", len(ends))
	}
	end := ends[0]
	if end.SSESummary == nil {
		t.Fatal("call_end missing sse_summary")
	}
	if end.SSESummary.DeltaCount != 4 || end.SSESummary.Text != "Hello" {
		t.Errorf("sse_summary = %+v, want 4 deltas / text Hello", end.SSESummary)
	}

	// Contents: one entry per received block, in order — the text block
	// with its full text, the tool_use block with name + assembled
	// input JSON.
	contents := end.SSESummary.Contents
	if len(contents) != 2 {
		t.Fatalf("contents = %+v, want 2 blocks", contents)
	}
	if contents[0].Type != "text" || contents[0].Text != "Hello" {
		t.Errorf("contents[0] = %+v, want text 'Hello'", contents[0])
	}
	if contents[1].Type != "tool_use" || contents[1].Name != "file-write" || contents[1].ID != "toolu_1" {
		t.Errorf("contents[1] identity = %+v", contents[1])
	}
	var toolInput map[string]any
	if err := json.Unmarshal(contents[1].Input, &toolInput); err != nil || toolInput["path"] != "x.md" {
		t.Errorf("contents[1] assembled input = %s (%v)", contents[1].Input, err)
	}

	sseTypes := map[string]bool{}
	for _, e := range obs.byKind("sse_event") {
		sseTypes[e.SSEType] = true
		if e.SSEType == "content_block_delta" {
			t.Error("delta frames must aggregate, not emit sse_event lines")
		}
	}
	for _, want := range []string{"message_start", "content_block_start", "content_block_stop", "message_delta", "message_stop"} {
		if !sseTypes[want] {
			t.Errorf("missing structural sse_event %q (got %v)", want, sseTypes)
		}
	}
}

// TestWithoutObserver_NoRequestOptions pins that the option is fully
// inert when unset.
func TestWithoutObserver_NoRequestOptions(t *testing.T) {
	if got := perCallRequestOptions(&api.CallOptions{}); got != nil {
		t.Errorf("expected nil request options, got %d", len(got))
	}
}
