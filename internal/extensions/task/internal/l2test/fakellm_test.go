// SPDX-License-Identifier: Apache-2.0

//go:build tasks_l2

package l2test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// THE FAKE MODEL — an OpenAI-compatible chat-completions endpoint served by
// the harness, so an agent loop runs on the real binaries with no LLM. It is
// stateless and scripted by the conversation it is sent:
//   - no tool result yet, `ask-user` offered → it calls ask-user;
//   - a tool result in the conversation → it calls `complete`, its summary
//     quoting the last tool result (the person's answer);
//   - otherwise it answers a line of text (the loop ends).
// Each request is recorded: a restarted agent that waits again must not call
// the model again.

type fakeLLM struct {
	srv  *http.Server
	url  string
	mu   sync.Mutex
	logs []string // one line per request: what it answered
}

func startFakeLLM(t *testing.T) *fakeLLM {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeLLM{url: "http://" + ln.Addr().String() + "/v1"}
	mux := http.NewServeMux()
	mux.HandleFunc("/", f.handle)
	f.srv = &http.Server{Handler: mux}
	go func() { _ = f.srv.Serve(ln) }()
	t.Cleanup(func() { _ = f.srv.Close() })
	return f
}

// Requests answers what the model was asked so far, one line each.
func (f *fakeLLM) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.logs...)
}

type chatRequest struct {
	Stream   bool `json:"stream"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
}

// text is a message's content, a string or a list of parts.
func text(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Text)
		}
		return b.String()
	}
	return string(raw)
}

func (f *fakeLLM) handle(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		http.Error(w, "the L2 fake model serves chat/completions only", http.StatusNotFound)
		return
	}
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tools := map[string]bool{}
	for _, tl := range req.Tools {
		tools[tl.Function.Name] = true
	}
	lastTool, seenTool := "", false
	for _, m := range req.Messages {
		if m.Role == "tool" {
			seenTool, lastTool = true, text(m.Content)
		}
	}
	var call, args, content string
	switch {
	case !seenTool && tools["ask-user"]:
		call, args = "ask-user", `{"question":"L2 harness: may the run go on?"}`
	case seenTool && tools["complete"]:
		b, _ := json.Marshal(map[string]any{"summary": "the person answered: " + lastTool})
		call, args = "complete", string(b)
	default:
		content = "L2 fake model: nothing to do (tools offered: " + strings.Join(keys(tools), ",") + ")"
	}
	f.mu.Lock()
	f.logs = append(f.logs, fmt.Sprintf("call=%q content=%q (after tool result %q)", call, content, lastTool))
	n := len(f.logs)
	f.mu.Unlock()

	id := fmt.Sprintf("l2-%d", n)
	finish := "stop"
	var toolCalls []map[string]any
	if call != "" {
		finish = "tool_calls"
		toolCalls = []map[string]any{{"index": 0, "id": "call_" + id, "type": "function",
			"function": map[string]any{"name": call, "arguments": args}}}
	}
	usage := map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
	if !req.Stream {
		msg := map[string]any{"role": "assistant", "content": content}
		if toolCalls != nil {
			msg["tool_calls"] = toolCalls
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "object": "chat.completion", "created": 0, "model": "l2-fake",
			"choices": []any{map[string]any{"index": 0, "message": msg, "finish_reason": finish}}, "usage": usage})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	chunk := func(choices []any, extra map[string]any) {
		c := map[string]any{"id": id, "object": "chat.completion.chunk", "created": 0, "model": "l2-fake", "choices": choices}
		for k, v := range extra {
			c[k] = v
		}
		b, _ := json.Marshal(c)
		fmt.Fprintf(w, "data: %s\n\n", b)
	}
	delta := map[string]any{"role": "assistant"}
	if content != "" {
		delta["content"] = content
	}
	if toolCalls != nil {
		delta["tool_calls"] = toolCalls
	}
	chunk([]any{map[string]any{"index": 0, "delta": delta}}, nil)
	chunk([]any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}, nil)
	chunk([]any{}, map[string]any{"usage": usage})
	fmt.Fprint(w, "data: [DONE]\n\n")
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
