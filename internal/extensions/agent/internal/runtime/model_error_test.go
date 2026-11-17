// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// failingModel fails its model call: at the call (callErr) or while its
// answer streams (streamErr).
type failingModel struct{ callErr, streamErr error }

func (f *failingModel) Info() llm.ModelInfo {
	return llm.ModelInfo{Provider: "anthropic", ModelID: "foundry-haiku"}
}
func (f *failingModel) GenerateContent(context.Context, []llm.MessageContent, ...llm.CallOption) (*llm.ContentResponse, error) {
	return nil, f.callErr
}
func (f *failingModel) GenerateContentStream(context.Context, []llm.MessageContent, ...llm.CallOption) (llm.Stream, error) {
	if f.callErr != nil {
		return nil, f.callErr
	}
	return &erroredStream{err: f.streamErr}, nil
}

type erroredStream struct{ err error }

func (s *erroredStream) Next() bool                     { return false }
func (s *erroredStream) Event() llm.StreamEvent         { return llm.StreamEvent{} }
func (s *erroredStream) Err() error                     { return s.err }
func (s *erroredStream) Close()                         {}
func (s *erroredStream) Response() *llm.ContentResponse { return nil }

// : a run whose model call fails during the run - no key at its
// first turn, a refused key, an unreachable endpoint, an unknown model - ends
// failed, never completed: its error event (the chat and the transcript), its
// end event and its result carry the error's text and the message key of its
// cause, which the GUI says in the person's language with the next step.
func TestAModelCallThatFailsEndsTheRunFailedSayingWhy(t *testing.T) {
	cases := []struct {
		name string
		m    *failingModel
		key  string
	}{
		{"no key", &failingModel{callErr: errors.New("anthropic: key ANTHROPIC_FOUNDRY_API_KEY, ANTHROPIC_FOUNDRY_BASE_URL is not set")}, MsgModelKeyMissing},
		{"refused key", &failingModel{streamErr: llm.NewError(llm.ErrCodeAuthentication, "anthropic", "invalid x-api-key")}, MsgModelKeyRefused},
		{"unreachable", &failingModel{callErr: errors.New(`Post "https://foundry.example/v1/messages": dial tcp: lookup foundry.example: no such host`)}, MsgModelUnreachable},
		{"unknown model", &failingModel{streamErr: errors.New("anthropic: 404 not_found_error: model: claude-nope")}, MsgModelUnknown},
		{"other", &failingModel{streamErr: errors.New("anthropic: stream ended unexpectedly")}, MsgModelCallFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := &AgentLoopMachine{model: c.m, plugins: NewPluginSet(), hooks: &HookChain{}, loop: NewLoopContext()}
			as := &agentState{StartedAt: time.Now(), MaxTurns: 50}
			action, err := m.Step(context.Background(), &machine.State{Name: "thinking", Data: as}, machine.Event{Type: machine.TickEvent})
			if err != nil {
				t.Fatal(err)
			}
			res, err := action(context.Background())
			if err != nil || res == nil {
				t.Fatalf("the step: %+v %v", res, err)
			}
			if res.NextState != "failed" || res.Failure == nil || !strings.Contains(res.Failure.Reason, "LLM error on turn 1") {
				t.Fatalf("the run ends %q with %+v; want failed, saying why", res.NextState, res.Failure)
			}
			r, ok := res.Output.(*api.RunResult)
			if !ok || r.Status != api.RunError || !usermsg.Is(r.Error, c.key) {
				t.Fatalf("its result %+v; want an error keyed %s", res.Output, c.key)
			}
			raw, _ := json.Marshal(r)
			if !strings.Contains(string(raw), `"Error":"LLM error on turn 1`) {
				t.Fatalf("its written result %s does not say the error", raw)
			}
			var errEv *api.ErrorEvent
			var end *api.CompleteEvent
			for _, e := range res.Emit {
				switch p := e.Payload.(type) {
				case api.ErrorEvent:
					errEv = &p
				case api.CompleteEvent:
					end = &p
				}
			}
			if errEv == nil || errEv.Error == "" || errEv.Msg == nil || errEv.Msg.Key != c.key || errEv.Msg.Params["model"] != "foundry-haiku" || errEv.Msg.Params["reason"] != errEv.Error {
				t.Fatalf("its error event %+v; want the error and its key %s", errEv, c.key)
			}
			if end == nil || end.Status != "error" || !strings.Contains(end.Error, errEv.Error) || end.Msg == nil || end.Msg.Key != c.key {
				t.Fatalf("its end event %+v; want failed with the error and its key", end)
			}
		})
	}
}
