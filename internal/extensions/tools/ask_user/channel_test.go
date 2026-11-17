// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"
	"errors"
	"sync"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// TestSelectChannel_Routing pins the UserCommsInput → impl mapping
// dispatcher routing. Empty input maps to interactive
// (safety default for programmatic callers that bypass the
// resolver).
func TestSelectChannel_Routing(t *testing.T) {
	cases := []struct {
		input    string
		wantImpl string // type name fragment in the returned interface
		wantErr  bool
	}{
		{"", "interactive", false},
		{"direct", "interactive", false},
		{"inbox", "inbox", false},
		{"none", "silent", false},
		{"bubble_to_parent", "bubble", false},
		{"via_agent", "bubble", false},
		{"via_leader", "bubble", false}, // a teammate asks its team leader
		{"garbage", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			ctx := agentapi.WithUserCommsInput(context.Background(), tc.input)
			ch, err := selectChannel(ctx)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q; got channel %T", tc.input, ch)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tc.input, err)
			}
			switch tc.wantImpl {
			case "interactive":
				if _, ok := ch.(*interactiveChannel); !ok {
					t.Errorf("input %q: got %T, want *interactiveChannel", tc.input, ch)
				}
			case "inbox":
				if _, ok := ch.(*inboxChannel); !ok {
					t.Errorf("input %q: got %T, want *inboxChannel", tc.input, ch)
				}
			case "silent":
				if _, ok := ch.(*silentChannel); !ok {
					t.Errorf("input %q: got %T, want *silentChannel", tc.input, ch)
				}
			case "bubble":
				if _, ok := ch.(*bubbleChannel); !ok {
					t.Errorf("input %q: got %T, want *bubbleChannel", tc.input, ch)
				}
			}
		})
	}
}

// TestSilentChannel_ReturnsErrNoUserChannel confirms the silent
// impl errors immediately with the canonical sentinel — callers
// that errors.Is(err, ErrNoUserChannel) detect the no-channel
// condition cleanly.
func TestSilentChannel_ReturnsErrNoUserChannel(t *testing.T) {
	c := &silentChannel{}
	_, err := c.Ask(context.Background(), AskInput{Question: "anything"})
	if err == nil {
		t.Fatal("silent channel should error")
	}
	if !errors.Is(err, ErrNoUserChannel) {
		t.Errorf("err %q does not wrap ErrNoUserChannel", err)
	}
}

// TestInboxChannel_ReturnsErrInboxNotImplemented confirms the
// fallback behavior — when the runtime has no InboxService /
// EventService / TaskService registered, the inbox impl errors
// with the specific sentinel chain so operators can diagnose
// quickly. The full real-impl path (services registered →
// InboxService.File → Suspend → Subscribe → wait → Resume) is exercised
// by integration tests; this unit test pins the missing-service
// fallback.
func TestInboxChannel_ReturnsErrInboxNotImplemented(t *testing.T) {
	c := &inboxChannel{}
	_, err := c.Ask(context.Background(), AskInput{Question: "anything"})
	if err == nil {
		t.Fatal("inbox channel should error when services are missing")
	}
	if !errors.Is(err, ErrInboxNotImplemented) {
		t.Errorf("err %q does not match ErrInboxNotImplemented", err)
	}
	if !errors.Is(err, ErrNoUserChannel) {
		t.Errorf("err %q does not wrap ErrNoUserChannel (sentinel chain broken)", err)
	}
}

// TestInFlightGuard_RejectsConcurrent pins the Cline-style
// single-in-flight invariant. The first acquire succeeds; the
// second for the same key fails until release() runs.
func TestInFlightGuard_RejectsConcurrent(t *testing.T) {
	release1, ok1 := acquireInFlight("task-A")
	if !ok1 {
		t.Fatal("first acquire should succeed")
	}
	_, ok2 := acquireInFlight("task-A")
	if ok2 {
		t.Error("second concurrent acquire for same key should fail")
	}
	release1()
	release3, ok3 := acquireInFlight("task-A")
	if !ok3 {
		t.Error("acquire after release should succeed")
	}
	release3()
}

// TestInFlightGuard_DifferentKeysIndependent confirms the invariant
// is per-key — task A and task B can ask concurrently.
func TestInFlightGuard_DifferentKeysIndependent(t *testing.T) {
	releaseA, okA := acquireInFlight("task-A")
	if !okA {
		t.Fatal("acquire A should succeed")
	}
	defer releaseA()
	releaseB, okB := acquireInFlight("task-B")
	if !okB {
		t.Error("acquire B should succeed (independent key)")
	}
	defer releaseB()
}

// TestInFlightGuard_RaceSafety pings the guard from many goroutines
// to confirm the mutex correctness (race detector will catch bad
// locking).
func TestInFlightGuard_RaceSafety(t *testing.T) {
	const N = 200
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			release, ok := acquireInFlight("race-key")
			if ok {
				// Held briefly then released.
				release()
			}
		}()
	}
	wg.Wait()
	// After the storm settles, the key should be free.
	release, ok := acquireInFlight("race-key")
	if !ok {
		t.Error("key should be free after all releases")
	}
	release()
}

// TestExecuteAskUser_RejectsConcurrent confirms the dispatcher
// wires the in-flight guard correctly — a concurrent call from the
// same loop ID returns ErrUserAskInFlight before reaching the
// channel impl.
func TestExecuteAskUser_RejectsConcurrent(t *testing.T) {
	// First acquire blocks the slot for "loop-X".
	pc := map[string]any{"_loop_id": "loop-X"}
	ctx := agentapi.WithPluginContext(context.Background(), pc)
	release, ok := acquireInFlight(loopIDFromContext(ctx))
	if !ok {
		t.Fatal("first acquire should succeed")
	}
	defer release()

	// Now the dispatcher should reject without invoking any channel.
	_, err := executeAskUser(ctx, AskInput{Question: "anything"})
	if !errors.Is(err, ErrUserAskInFlight) {
		t.Errorf("err = %v, want ErrUserAskInFlight", err)
	}
}

// TestLoopIDFromContext_FallsBackToGlobal confirms callers without
// a plugin-context `_loop_id` value still get a usable key (the
// "global" sentinel).
func TestLoopIDFromContext_FallsBackToGlobal(t *testing.T) {
	if got := loopIDFromContext(context.Background()); got != "global" {
		t.Errorf("got %q, want global", got)
	}
	pc := map[string]any{"_loop_id": "loop-Y"}
	ctx := agentapi.WithPluginContext(context.Background(), pc)
	if got := loopIDFromContext(ctx); got != "loop-Y" {
		t.Errorf("got %q, want loop-Y", got)
	}
}
