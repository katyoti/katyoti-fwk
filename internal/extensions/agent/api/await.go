// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"time"
)

// ErrAwait is what a tool returns when its answer comes later — from a person,
// another process — instead of blocking the run: "wait for this event, then
// call me again". The agent loop then waits as a machine (the run holds no
// slot, its state is saved and survives a restart); when the event comes, or
// after a restart, it calls the same tool call again with Resume on its
// context (AwaitResumeFrom). The tool checks: answered → its result; not yet →
// ErrAwait again.
//
// A tool returns it only when MachineWaitsFrom(ctx) is true; otherwise it waits
// in place as before.
type ErrAwait struct {
	// EventURI is the event that may bring the answer (an EventURI, e.g. an
	// inbox request updated) — an event of another process.
	EventURI string
	// Topic is, instead, an in-process topic that may bring it (a child run's
	// end, engine.RunEndedTopic). One of EventURI and Topic is set.
	Topic string
	// Resume is what the tool needs to check again: an id, a key. It is saved
	// with the run, so it is JSON.
	Resume map[string]any
	// Reason says what the run waits for, for people reading its state.
	Reason string
	// RecheckEvery, when set, calls the tool again at that period while it
	// awaits, event or not: for an answer that may come without its event, or
	// a wait that may never be answered (its request gone).
	RecheckEvery time.Duration
}

func (e *ErrAwait) Error() string {
	if e.Reason != "" {
		return "waiting: " + e.Reason
	}
	return fmt.Sprintf("waiting on %s", e.EventURI)
}

type machineWaitsKey struct{}
type awaitResumeKey struct{}

// WithMachineWaits marks a tool-dispatch context whose loop waits as a
// machine: a tool may return ErrAwait instead of blocking. It belongs to the
// one call the loop dispatches: a tool that calls ANOTHER tool must not pass
// this context on (the loop would call the outer tool again, not the inner
// one) — no tool does today.
func WithMachineWaits(ctx context.Context) context.Context {
	return context.WithValue(ctx, machineWaitsKey{}, true)
}

// MachineWaitsFrom reports whether a tool may return ErrAwait.
func MachineWaitsFrom(ctx context.Context) bool {
	v, _ := ctx.Value(machineWaitsKey{}).(bool)
	return v
}

// WithAwaitResume carries, to a tool called again, the Resume of the ErrAwait
// it returned.
func WithAwaitResume(ctx context.Context, resume map[string]any) context.Context {
	return context.WithValue(ctx, awaitResumeKey{}, resume)
}

// AwaitResumeFrom returns the Resume a tool called again saved; ok is false on
// a first call.
func AwaitResumeFrom(ctx context.Context) (map[string]any, bool) {
	v, ok := ctx.Value(awaitResumeKey{}).(map[string]any)
	return v, ok && v != nil
}
