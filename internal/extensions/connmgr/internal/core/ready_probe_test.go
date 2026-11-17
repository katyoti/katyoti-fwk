// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// busySender is a member serving others: each ping answers after delay.
type busySender struct {
	delay time.Duration
	calls atomic.Int32
}

func (b *busySender) Call(ctx context.Context, _ string, _ any, _ ...ktsp.CallOption) (json.RawMessage, error) {
	b.calls.Add(1)
	select {
	case <-time.After(b.delay):
		return json.RawMessage(`{}`), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (b *busySender) Notify(context.Context, string, any, ...ktsp.CallOption) error { return nil }

// : a member busy serving several agents answers its readiness ping
// late but is alive - its start is ready, not killed; and a member that
// declares no limit gets one a large project's start fits in.
func TestABusyMemberIsReadyNotKilled(t *testing.T) {
	busy := &busySender{delay: 600 * time.Millisecond}
	if err := waitForNATSReady(context.Background(), busy, "ktw-agents:ktsp/ping", 5*time.Second); err != nil {
		t.Fatalf("a member answering its ping in 600 ms counted as not ready: %v", err)
	}
	if cmapi.DefaultReadyTimeout < 30*time.Second {
		t.Fatalf("DefaultReadyTimeout = %s: a start reading a large project under load does not fit", cmapi.DefaultReadyTimeout)
	}

	// A member that never answers still fails, at its limit.
	dead := &busySender{delay: time.Hour}
	start := time.Now()
	err := waitForNATSReady(context.Background(), dead, "x:ktsp/ping", 1*time.Second)
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("a member that never answers = %v, want the readiness deadline", err)
	}
	if took := time.Since(start); took > 4*time.Second {
		t.Fatalf("a dead member held the start %s, past its 1 s limit", took)
	}
}
