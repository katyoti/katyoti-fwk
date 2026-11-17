// SPDX-License-Identifier: Apache-2.0

package httputil

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/models/api"
)

// callSeq numbers calls within the process — the grouping key for the
// JSONL event stream (one process writes one session file).
var callSeq atomic.Int64

// ObserveRoundTrip emits structured ModelCallEvents for one model call
// and forwards the request via next. Shared by the provider SDK
// middlewares (the Stainless middleware shape is identical modulo the
// package-local Next type).
//
// Emission contract (the llm-model-calls.jsonl schema — see
// api.ModelCallEvent):
//   - call_start before the request leaves: method/url/model, scrubbed
//     headers, decoded JSON body.
//   - sse_event per STRUCTURAL streamed frame as the consumer reads it;
//     incremental delta frames are aggregated, not emitted.
//   - call_end when the response completes (non-SSE: immediately, with
//     the decoded body; SSE: when the stream is drained or closed, with
//     the delta aggregation) or when transport fails.
func ObserveRoundTrip(obs api.ModelCallObserver, provider string, req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	callID := fmt.Sprintf("c-%04d", callSeq.Add(1))
	start := time.Now()

	reqBody := readRequestBody(req)
	obs.ObserveModelCall(api.ModelCallEvent{
		Kind:           "call_start",
		CallID:         callID,
		TS:             start.UTC(),
		Provider:       provider,
		Model:          modelFromBody(reqBody),
		Method:         req.Method,
		URL:            req.URL.String(),
		RequestHeaders: scrubHeaderMap(req.Header),
		RequestBody:    rawJSONOrNil(reqBody),
	})

	resp, err := next(req)
	if err != nil {
		obs.ObserveModelCall(api.ModelCallEvent{
			Kind:       "call_end",
			CallID:     callID,
			TS:         time.Now().UTC(),
			DurationMs: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		})
		return resp, err
	}

	end := api.ModelCallEvent{
		Kind:            "call_end",
		CallID:          callID,
		Status:          resp.StatusCode,
		ResponseHeaders: scrubHeaderMap(resp.Header),
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") && resp.Body != nil {
		// Streamed response: parse SSE frames as the consumer reads.
		// call_end fires when the stream is drained / closed.
		resp.Body = newSSEObserverBody(resp.Body, obs, callID, start, end)
		return resp, nil
	}

	// Non-SSE: buffer the body so we can both log and replay it.
	if resp.Body != nil {
		data, rerr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if rerr == nil {
			resp.Body = io.NopCloser(bytes.NewReader(data))
			end.ResponseBody = rawJSONOrNil(data)
		} else {
			end.Error = fmt.Sprintf("read response: %v", rerr)
		}
	}
	end.TS = time.Now().UTC()
	end.DurationMs = time.Since(start).Milliseconds()
	obs.ObserveModelCall(end)
	return resp, nil
}

// readRequestBody returns the request body bytes, restoring req.Body
// for the actual send (outgoing requests carry GetBody when replayable;
// fall back to a read+replace).
func readRequestBody(req *http.Request) []byte {
	if req.Body == nil {
		return nil
	}
	if req.GetBody != nil {
		if rc, err := req.GetBody(); err == nil {
			data, _ := io.ReadAll(rc)
			_ = rc.Close()
			return data
		}
	}
	data, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil
	}
	req.Body = io.NopCloser(bytes.NewReader(data))
	return data
}

// rawJSONOrNil returns data as json.RawMessage when it is valid JSON,
// nil otherwise (never double-encode payloads as strings).
func rawJSONOrNil(data []byte) json.RawMessage {
	if len(data) == 0 || !json.Valid(data) {
		return nil
	}
	return json.RawMessage(data)
}

// modelFromBody extracts the request's "model" field.
func modelFromBody(body []byte) string {
	var probe struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &probe) == nil {
		return probe.Model
	}
	return ""
}

// scrubHeaderMap flattens headers to a map with credential values
// masked (same catalog as ScrubAuthHeaders).
func scrubHeaderMap(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, vals := range h {
		v := strings.Join(vals, ", ")
		switch strings.ToLower(k) {
		case "x-api-key", "authorization", "api-key":
			v = "***"
		}
		out[k] = v
	}
	return out
}

// ── SSE observing body ────────────────────────────────────────────────

// structuralSSE lists the frame types emitted as individual sse_event
// lines. Everything else (per-token text / input_json deltas, pings)
// is aggregated into the call_end SSESummary.
var structuralSSE = map[string]bool{
	// Anthropic
	"message_start":       true,
	"content_block_start": true,
	"content_block_stop":  true,
	"message_delta":       true,
	"message_stop":        true,
	"error":               true,
}

// newSSEObserverBody wraps an SSE body: frames are parsed as the
// consumer reads; structural frames emit sse_event, deltas aggregate.
func newSSEObserverBody(rc io.ReadCloser, obs api.ModelCallObserver, callID string, start time.Time, end api.ModelCallEvent) io.ReadCloser {
	pr, pw := io.Pipe()
	out := &sseObserverBody{reader: pr, closer: rc}

	go func() {
		defer func() { _ = pw.Close() }()
		summary := &api.SSESummary{}
		blocks := newSSEContentTracker()
		var firstByte time.Time
		scanner := bufio.NewScanner(io.TeeReader(rc, pw))
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if firstByte.IsZero() {
				firstByte = time.Now()
			}
			data, ok := strings.CutPrefix(line, "data: ")
			if !ok {
				continue
			}
			summary.EventCount++
			sseType := sseTypeOf(data)
			blocks.observe(sseType, data)
			if structuralSSE[sseType] {
				obs.ObserveModelCall(api.ModelCallEvent{
					Kind:    "sse_event",
					CallID:  callID,
					TS:      time.Now().UTC(),
					SSEType: sseType,
					SSEData: rawJSONOrNil([]byte(data)),
				})
				continue
			}
			summary.DeltaCount++
			summary.Text += deltaTextOf(data)
		}

		end.TS = time.Now().UTC()
		end.DurationMs = time.Since(start).Milliseconds()
		if !firstByte.IsZero() {
			end.FirstByteMs = firstByte.Sub(start).Milliseconds()
		}
		summary.Contents = blocks.finish()
		end.SSESummary = summary
		obs.ObserveModelCall(end)
	}()

	return out
}

type sseObserverBody struct {
	reader io.Reader
	closer io.Closer
}

func (b *sseObserverBody) Read(p []byte) (int, error) { return b.reader.Read(p) }
func (b *sseObserverBody) Close() error               { return b.closer.Close() }

// sseContentTracker reassembles the response's content blocks from the
// frame stream: anthropic content_block_start/delta/stop sequences
// (text + tool_use + thinking), and openai chat-completion delta chunks
// (single implicit text block + tool_call deltas).
type sseContentTracker struct {
	open  map[int]*api.SSEContent // anthropic: by block index
	order []int
	text  *api.SSEContent // openai: implicit text block
}

func newSSEContentTracker() *sseContentTracker {
	return &sseContentTracker{open: map[int]*api.SSEContent{}}
}

func (t *sseContentTracker) observe(sseType, data string) {
	switch sseType {
	case "content_block_start":
		var f struct {
			Index int `json:"index"`
			Block struct {
				Type string `json:"type"`
				Name string `json:"name"`
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"content_block"`
		}
		if json.Unmarshal([]byte(data), &f) != nil {
			return
		}
		t.open[f.Index] = &api.SSEContent{Type: f.Block.Type, Name: f.Block.Name, ID: f.Block.ID, Text: f.Block.Text}
		t.order = append(t.order, f.Index)

	case "content_block_delta":
		var f struct {
			Index int `json:"index"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &f) != nil {
			return
		}
		b, ok := t.open[f.Index]
		if !ok {
			return
		}
		switch f.Delta.Type {
		case "text_delta":
			b.Text += f.Delta.Text
		case "thinking_delta":
			b.Text += f.Delta.Thinking
		case "input_json_delta":
			b.Input = append(b.Input, []byte(f.Delta.PartialJSON)...)
		}

	default:
		// OpenAI chat chunks have no SSE type; reassemble text +
		// tool-call deltas from the choices array.
		var f struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int `json:"index"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
						ID string `json:"id"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &f) != nil || len(f.Choices) == 0 {
			return
		}
		d := f.Choices[0].Delta
		if d.Content != "" {
			if t.text == nil {
				t.text = &api.SSEContent{Type: "text"}
				t.open[-1] = t.text
				t.order = append(t.order, -1)
			}
			t.text.Text += d.Content
		}
		for _, tc := range d.ToolCalls {
			idx := 1000 + tc.Index // offset to avoid colliding with -1 / anthropic indices
			b, ok := t.open[idx]
			if !ok {
				b = &api.SSEContent{Type: "tool_use"}
				t.open[idx] = b
				t.order = append(t.order, idx)
			}
			if tc.Function.Name != "" {
				b.Name = tc.Function.Name
			}
			if tc.ID != "" {
				b.ID = tc.ID
			}
			b.Input = append(b.Input, []byte(tc.Function.Arguments)...)
		}
	}
}

func (t *sseContentTracker) finish() []api.SSEContent {
	out := make([]api.SSEContent, 0, len(t.order))
	for _, idx := range t.order {
		b := t.open[idx]
		if b == nil {
			continue
		}
		// Assembled tool input must stay valid JSON for the consumer;
		// drop partial fragments from a truncated stream.
		if len(b.Input) > 0 && !json.Valid(b.Input) {
			b.Input = nil
		}
		out = append(out, *b)
	}
	return out
}

func sseTypeOf(data string) string {
	var probe struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal([]byte(data), &probe)
	return probe.Type
}

// deltaTextOf extracts incremental text from a delta frame (anthropic
// content_block_delta text_delta; openai chat chunks).
func deltaTextOf(data string) string {
	// Anthropic: {"type":"content_block_delta","delta":{"type":"text_delta","text":"…"}}
	var a struct {
		Delta struct {
			Text string `json:"text"`
		} `json:"delta"`
	}
	if json.Unmarshal([]byte(data), &a) == nil && a.Delta.Text != "" {
		return a.Delta.Text
	}
	// OpenAI: {"choices":[{"delta":{"content":"…"}}]}
	var o struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if json.Unmarshal([]byte(data), &o) == nil && len(o.Choices) > 0 {
		return o.Choices[0].Delta.Content
	}
	return ""
}
