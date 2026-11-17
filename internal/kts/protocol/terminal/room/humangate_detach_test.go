// SPDX-License-Identifier: Apache-2.0

package room

import (
	"context"
	"errors"
	"testing"
	"time"

	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
)

// TestHumanGate_DetachReturnsSentinel pins the fall-over trigger: a
// pending Prompt whose last UI client disconnects (past the grace
// period) returns ErrUserDetached so the ask-user tool can re-route
// the question to the inbox instead of hanging forever.
func TestHumanGate_DetachReturnsSentinel(t *testing.T) {
	g := NewHumanGate()
	g.SetDetachGrace(30 * time.Millisecond)
	g.SetAttachedClients(1)

	done := make(chan error, 1)
	go func() {
		_, err := g.Prompt(context.Background(), hgapi.PromptRequest{Type: "text", Question: "q?"})
		done <- err
	}()
	time.Sleep(20 * time.Millisecond) // let the wait start
	g.SetAttachedClients(0)           // last client gone → grace timer arms

	select {
	case err := <-done:
		if !errors.Is(err, hgapi.ErrUserDetached) {
			t.Fatalf("err = %v, want ErrUserDetached", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt did not return after detach grace expired")
	}
}

// TestHumanGate_ReattachWithinGraceKeepsWaiting pins the reconnect
// path: a client returning within the grace period disarms the
// timer; the pending prompt keeps waiting and a delivered answer
// still lands.
func TestHumanGate_ReattachWithinGraceKeepsWaiting(t *testing.T) {
	g := NewHumanGate()
	g.SetDetachGrace(80 * time.Millisecond)
	g.SetAttachedClients(1)

	done := make(chan string, 1)
	errs := make(chan error, 1)
	go func() {
		resp, err := g.Prompt(context.Background(), hgapi.PromptRequest{Type: "text", Question: "q?"})
		if err != nil {
			errs <- err
			return
		}
		done <- resp.Answer
	}()
	time.Sleep(20 * time.Millisecond)
	g.SetAttachedClients(0) // detach…
	time.Sleep(20 * time.Millisecond)
	g.SetAttachedClients(1) // …reattach within grace
	time.Sleep(100 * time.Millisecond)
	g.DeliverAnswer("still here")

	select {
	case ans := <-done:
		if ans != "still here" {
			t.Fatalf("answer = %q, want %q", ans, "still here")
		}
	case err := <-errs:
		t.Fatalf("prompt errored after reattach: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("prompt never returned")
	}
}

// TestHumanGate_AlreadyDetachedFailsFast pins the cold-start case: a
// prompt that begins AFTER the grace has already expired returns the
// sentinel immediately — the ask goes straight to the inbox.
func TestHumanGate_AlreadyDetachedFailsFast(t *testing.T) {
	g := NewHumanGate()
	g.SetDetachGrace(10 * time.Millisecond)
	g.SetAttachedClients(1)
	g.SetAttachedClients(0)
	time.Sleep(50 * time.Millisecond) // grace expires before the ask

	_, err := g.Prompt(context.Background(), hgapi.PromptRequest{Type: "text", Question: "q?"})
	if !errors.Is(err, hgapi.ErrUserDetached) {
		t.Fatalf("err = %v, want ErrUserDetached", err)
	}
}

// TestHumanGate_ReattachAfterExpiryResetsSignal pins the recovery
// path: after a grace expiry closed the detach signal, a fresh
// client connection resets it so NEW prompts wait normally again.
func TestHumanGate_ReattachAfterExpiryResetsSignal(t *testing.T) {
	g := NewHumanGate()
	g.SetDetachGrace(10 * time.Millisecond)
	g.SetAttachedClients(1)
	g.SetAttachedClients(0)
	time.Sleep(50 * time.Millisecond) // signal closed
	g.SetAttachedClients(1)           // user is back

	done := make(chan string, 1)
	go func() {
		resp, err := g.Prompt(context.Background(), hgapi.PromptRequest{Type: "text", Question: "q?"})
		if err == nil {
			done <- resp.Answer
		}
	}()
	time.Sleep(20 * time.Millisecond)
	g.DeliverAnswer("recovered")

	select {
	case ans := <-done:
		if ans != "recovered" {
			t.Fatalf("answer = %q, want %q", ans, "recovered")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prompt after re-attach did not behave normally")
	}
}
