// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	kcontainer "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// A CLIENT OF THE RUNNING UNIVERSE: with a remote
// dispatcher set, a generated verb goes to it — provider, slot, method, the
// JSON built from the flags, the method's timeout — and its answer comes back
// as is; the in-process handler is never run, and an error from the remote is
// the verb's error, never a fallback to local dispatch.
func TestDispatchCall_RemoteDispatcherRoutesEveryVerb(t *testing.T) {
	c := kcontainer.NewEndpoint("ktw-agents", "Agents", "1.0.0")
	slot := kcontainer.NewSlot("cwm", kcontainer.DefaultInstance, "CWM", nil)
	localRuns := 0
	slot.Add(kcontainer.RawMethod("object-get", kcontainer.MethodOpts{Desc: "Get", TimeoutS: 42}, func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
		localRuns++
		return json.Marshal("local")
	}))
	c.AddSlot(slot)

	type call struct {
		provider, slot, method string
		timeout                time.Duration
		data                   string
	}
	var got call
	SetRemoteDispatcher(func(_ context.Context, provider, slot, method string, timeout time.Duration, data []byte) ([]byte, error) {
		got = call{provider, slot, method, timeout, string(data)}
		return []byte(`"served"`), nil
	})
	t.Cleanup(func() { SetRemoteDispatcher(nil) })

	out, err := dispatchCall(context.Background(), c, "cwm", "object-get", []byte(`{"id":"x"}`))
	if err != nil {
		t.Fatalf("remote dispatch: %v", err)
	}
	if string(out) != `"served"` {
		t.Fatalf("answer = %s, want the remote's", out)
	}
	want := call{"ktw-agents", "cwm", "object-get", 42 * time.Second, `{"id":"x"}`}
	if got != want {
		t.Fatalf("remote received %+v, want %+v", got, want)
	}
	if localRuns != 0 {
		t.Fatalf("the local handler ran %d times: a client never dispatches in-process", localRuns)
	}

	// The remote's error is the verb's error: no fallback to local.
	boom := errors.New("the serving process refused")
	SetRemoteDispatcher(func(context.Context, string, string, string, time.Duration, []byte) ([]byte, error) {
		return nil, boom
	})
	if _, err := dispatchCall(context.Background(), c, "cwm", "object-get", []byte(`{}`)); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the remote's error", err)
	}
	if localRuns != 0 {
		t.Fatal("a remote error fell back to the local handler")
	}

	// Without a dispatcher, the verb runs here.
	SetRemoteDispatcher(nil)
	out, err = dispatchCall(context.Background(), c, "cwm", "object-get", []byte(`{}`))
	if err != nil || string(out) != `"local"` || localRuns != 1 {
		t.Fatalf("local dispatch: out=%s err=%v runs=%d", out, err, localRuns)
	}
}
