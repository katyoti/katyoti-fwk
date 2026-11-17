// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// flakyTransport fails its first Send (a lost answer), then answers; it
// records the id of every request.
type flakyTransport struct {
	ids   []any
	calls int
}

func (f *flakyTransport) Send(_ context.Context, req *Request) (*Response, error) {
	f.ids = append(f.ids, req.ID)
	f.calls++
	if f.calls == 1 {
		return nil, errors.New("connection reset: the answer was lost")
	}
	return &Response{ID: req.ID, Result: json.RawMessage(`"ok"`)}, nil
}
func (f *flakyTransport) Notify(context.Context, *Notification) error { return nil }
func (f *flakyTransport) Close() error                                { return nil }

// A CALL KEEPS ONE REQUEST ID ACROSS ITS RETRIES: the
// protocol sender's transient retry sends the same request again; a new call
// gets a new id.
func TestARetriedCallKeepsItsRequestID(t *testing.T) {
	tr := &flakyTransport{}
	s := NewRetrySender(tr, RetryPolicy{MaxRetries: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})
	if _, err := s.Call(context.Background(), "c:/slot/method", nil); err != nil {
		t.Fatal(err)
	}
	if len(tr.ids) != 2 || tr.ids[0] != tr.ids[1] {
		t.Fatalf("the call and its retry carried ids %v; want one id, sent twice", tr.ids)
	}
	if _, err := s.Call(context.Background(), "c:/slot/method", nil); err != nil {
		t.Fatal(err)
	}
	if tr.ids[2] == tr.ids[0] {
		t.Fatalf("a new call reused the id: %v", tr.ids)
	}
}
