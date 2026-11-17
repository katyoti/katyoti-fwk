// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// RetrySender wraps a messagebus.Sender with the A-RETRY lazy-spawn
// pattern: on the first ErrSlotNotFound (which the inner Sender maps
// from NATS's ErrNoResponders) for a targetable provider, the wrapper
// calls `<hubProvider>:/conn-mgr/ensure-spawned({name: <target>})`,
// then retries the original Send / Notify once.
//
// Cold path: 2 round-trips (ensure-spawned + retry). Warm path: 1
// round-trip (direct). The retry budget is exactly one — repeat failures
// pass through unchanged so a permanently-broken sidecar surfaces as a
// real error instead of looping forever.
//
// hubProvider is the bus identity of the binary hosting ConnMgrService
// (default `"ktw-hub"`). The ensure-spawned call uses the INNER Sender
// (not the wrapper) to avoid recursion on the unlikely case where the
// hub itself isn't responding.
type RetrySender struct {
	inner       *Sender
	hubProvider string
}

// NewRetrySender wraps a messagebus.Sender with the A-RETRY behavior.
// If hubProvider is empty, defaults to "ktw-hub".
func NewRetrySender(inner *Sender, hubProvider string) *RetrySender {
	if hubProvider == "" {
		hubProvider = "ktw-hub"
	}
	return &RetrySender{inner: inner, hubProvider: hubProvider}
}

// Send implements ktsp.Sender. On the first ErrSlotNotFound, calls
// ensure-spawned for the target provider, then retries once.
func (r *RetrySender) Call(ctx context.Context, path string, params any, opts ...ktsp.CallOption) (json.RawMessage, error) {
	// One request id for the call and its retry: the serving side answers
	// it once (replay.go).
	if RequestIDFrom(ctx) == "" {
		ctx = WithRequestID(ctx, newCallID())
	}
	out, err := r.inner.Call(ctx, path, params, opts...)
	if err == nil {
		return out, nil
	}
	target, ok := targetForRetry(r, ctx, path, err)
	if !ok {
		return out, err
	}
	if ensureErr := r.ensureSpawned(ctx, target); ensureErr != nil {
		// ensure-spawned itself failed — return the ORIGINAL error so
		// the caller sees the slot-not-found context, with the ensure
		// failure appended for diagnostics.
		return out, fmt.Errorf("%w (lazy-spawn ensure failed: %v)", err, ensureErr)
	}
	return r.inner.Call(ctx, path, params, opts...)
}

// Notify implements ktsp.Sender. Same A-RETRY behavior as Call.
//
// Notify is fire-and-forget on the wire (no reply subject), so NATS
// returns success the moment the publish lands locally — even when no
// subscriber is listening. That means we can't reliably detect "lazy
// spawn needed" from Notify alone. The retry here is best-effort: if
// the inner Notify errors AND we can extract a target provider, we
// run ensure-spawned + retry; otherwise we pass through.
func (r *RetrySender) Notify(ctx context.Context, path string, params any, opts ...ktsp.CallOption) error {
	err := r.inner.Notify(ctx, path, params, opts...)
	if err == nil {
		return nil
	}
	target, ok := targetForRetry(r, ctx, path, err)
	if !ok {
		return err
	}
	if ensureErr := r.ensureSpawned(ctx, target); ensureErr != nil {
		return fmt.Errorf("%w (lazy-spawn ensure failed: %v)", err, ensureErr)
	}
	return r.inner.Notify(ctx, path, params, opts...)
}

// targetForRetry checks whether the error is a slot-not-found AND the
// path is targetable (concrete provider that isn't the hub itself —
// retrying the hub would loop). Returns the target provider on a
// retryable failure.
func targetForRetry(r *RetrySender, ctx context.Context, path string, err error) (target string, ok bool) {
	var rpcErr *ktsp.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != ktsp.CodeSlotNotFound {
		return "", false
	}
	p, parseErr := ParsePath(ctx, path)
	if parseErr != nil {
		return "", false
	}
	// Don't retry against the hub itself — if the hub's own slot 404s,
	// retrying via ensure-spawned would just hit the same path.
	if p.Provider == r.hubProvider {
		return "", false
	}
	return p.Provider, true
}

// ensureSpawnTimeout caps the wait for the hub to spawn a cold sidecar.
// ktw-agents bootstrap is heavy (manifest hydration, plugin registration,
// service publishing, then `mbsubscribe.Subscribe` for slot routing) —
// 5-10s on a warm cache, far longer with antivirus scanning an unsigned
// executable and other agents loading the machine. The hub bounds the start
// itself (the member's readiness limit, 30 s by default, 1 min for
// ktw-agents in a universe that declares it, plus a short start back-off it
// waits out): the caller waits longer than that, so it gets the start's
// outcome instead of a request timeout while the member is still coming up
// , and still surfaces a hub that does not answer at all.
const ensureSpawnTimeout = 2 * time.Minute

// ensureSpawned issues `<hubProvider>:/ConnMgrService/EnsureSpawned({name})`
// on the INNER (non-retrying) sender so retries don't recurse if the
// hub itself is misbehaving.
//
// Wraps ctx with `ensureSpawnTimeout` because the bus adapter falls back
// to a 5s default when the caller's ctx has no deadline — too short for
// cold sidecar spawn (ktw-agents in particular).
func (r *RetrySender) ensureSpawned(ctx context.Context, target string) error {
	spawnCtx, cancel := context.WithTimeout(WithRequestID(ctx, newCallID()), ensureSpawnTimeout) // a request of its own
	defer cancel()
	path := r.hubProvider + ":ConnMgrService/EnsureSpawned"
	in := cmapi.EnsureSpawnedInput{Name: target}
	_, err := r.inner.Call(spawnCtx, path, in)
	return err
}

// Compile-time interface assertion.
var _ ktsp.Sender = (*RetrySender)(nil)
