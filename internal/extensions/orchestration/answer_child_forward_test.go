// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	commsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comms/api"
)

// stubComms satisfies commsapi.Service via the embedded nil interface; only
// Answer is exercised by handleAnswerChild.
type stubComms struct {
	commsapi.Service
	delivered bool
	detail    string
}

func (s stubComms) Answer(_ context.Context, _ commsapi.AnswerRequest) (commsapi.Answered, error) {
	return commsapi.Answered{Delivered: s.delivered, Detail: s.detail}, nil
}

// TestAnswerChild_ForwardInstruction pins the leanness fix: BOTH answer-child
// outcomes point the driver forward to await-child, so it stops re-answering
// defensively (the "4 no-op retries" waste). The next move is never another
// answer for the same id.
func TestAnswerChild_ForwardInstruction(t *testing.T) {
	in := json.RawMessage(`{"question_id":"q1","answer":"Blue"}`)

	// Delivered → "answered: …" plus await-child / do-not-re-answer guidance.
	p := &orchPlugin{comms: stubComms{delivered: true, detail: "Blue"}}
	got, err := p.handleAnswerChild(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	s := got.ResultString()
	if !strings.Contains(s, "await-child") || !strings.Contains(strings.ToLower(s), "do not re-answer") {
		t.Fatalf("delivered result must steer to await-child + not re-answer; got %q", s)
	}

	// Not delivered (already handled) → no-op + await-child, never a retry.
	p2 := &orchPlugin{comms: stubComms{delivered: false, detail: "question not found"}}
	got2, err := p2.handleAnswerChild(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	s2 := got2.ResultString()
	if !strings.Contains(s2, "await-child") || !strings.Contains(strings.ToLower(s2), "no action needed") {
		t.Fatalf("already-handled result must say no-action + await-child; got %q", s2)
	}
}
