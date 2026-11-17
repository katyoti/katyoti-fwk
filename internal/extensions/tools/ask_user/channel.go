// SPDX-License-Identifier: Apache-2.0

// Package ask_user — channel.go defines the 3-impl interface seam.
//
// The spawn dimensions carry a resolved channel value
// (`UserComms.Input`) per run — `direct` / `inbox` /
// `bubble_to_parent` / `via_leader` / `none`. The tool dispatches
// to the right impl based on that value:
//
//   - `direct` → interactive_ask.go (HumanGate path)
//   - `inbox`  → inbox_ask.go (async via approval-request items)
//   - `none`   → silent_ask.go (returns ErrNoUserChannel immediately)
//   - `bubble_to_parent` / `via_agent` / `via_leader` → bubble_ask.go:
//     a human task on the holding list of the run that spawned this one
//     (the parent, or a teammate's team leader), answered by that run
//
// Single-in-flight invariant: the dispatcher wraps the selected
// impl with a `taskID` → in-flight-flag map. A concurrent second
// `ask-user` call from the same task returns `ErrUserAskInFlight`
// immediately instead of stacking gates. The flag is keyed by the
// loop's `_loop_id` plugin context value when available, falling
// back to a process-wide global guard otherwise.
package ask_user

import (
	"context"
	"errors"
	"fmt"
	"sync"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// AskUserChannel is the per-impl interface — three implementations
// exist in this package (interactive / inbox / silent) plus two stub
// errors (bubble / via_leader) routed from selectChannel().
//
// Ask is called once per ask-user invocation. The implementation
// owns prompt-style branching (text/select/confirm/form) internally
// because each channel has different capabilities — `inbox` may not
// support `form`, `silent` doesn't support any prompt type, etc.
type AskUserChannel interface {
	Ask(ctx context.Context, in AskInput) (AskOutput, error)
}

// ErrNoUserChannel is returned by silent_ask + by the dispatcher's
// stub branches for `bubble_to_parent` / `via_leader` until those
// channels ship. Distinct error sentinel so callers can detect the
// "no channel available" condition vs other failures.
var ErrNoUserChannel = errors.New("ask-user: no user channel available")

// ErrUserAskInFlight is returned when a task tries to call ask-user
// while a previous call from the same task is still waiting on a
// user response. Per the Cline race-bug lesson: stacking gates
// produces inconsistent UI state + race conditions in the inbox-item
// suspension path; we reject the second call instead.
var ErrUserAskInFlight = errors.New("ask-user: previous request still pending for this task")

// selectChannel picks the AskUserChannel implementation for the run
// based on the resolved UserCommsInput. Reading happens at execute
// time, not at tool construction, so the same registered tool serves
// every channel — the loop's per-run ctx carries the value.
//
// Empty / unrecognized input → interactive (the safe default, matches
// existing behavior so programmatic callers that bypass the resolver
// don't break).
func selectChannel(ctx context.Context) (AskUserChannel, error) {
	input := agentapi.UserCommsInputFromContext(ctx)
	switch input {
	case "", "direct":
		return &interactiveChannel{}, nil
	case "inbox":
		return &inboxChannel{}, nil
	case "none":
		return &silentChannel{}, nil
	case "bubble_to_parent", "via_agent", "via_leader":
		// The "agent as human" channel: the question is a human
		// task on the holding list of the run that spawned this one - the
		// parent, or a teammate's team leader - its one member answering.
		return &bubbleChannel{}, nil
	default:
		return nil, fmt.Errorf("ask-user: unknown UserCommsInput %q (expected direct/inbox/bubble_to_parent/via_agent/via_leader/none)", input)
	}
}

// ── single in-flight guard ─────────────────────────────────────────

// inFlightGuard is the process-global ask-user in-flight tracker.
// Keyed by the loop's run ID (read from the plugin context's
// `_loop_id` value when set, falling back to a sentinel "global"
// scope). A second concurrent acquire for the same key returns
// false; the dispatcher rejects with ErrUserAskInFlight.
var inFlightGuard = struct {
	mu     sync.Mutex
	active map[string]bool
}{active: map[string]bool{}}

// loopIDFromContext extracts the task / loop identifier used as the
// in-flight key. Plugin context carries the loop ID under
// `_loop_id` (the convention used by every other per-run tracker in
// the agent runtime). When unset (legacy tests / programmatic
// callers) we fall back to the sentinel "global" so concurrent
// callers across different runs still get isolation in practice
// (the chat scenario is single-loop anyway).
func loopIDFromContext(ctx context.Context) string {
	pc := agentapi.PluginContextFrom(ctx)
	if pc != nil {
		if v, ok := pc["_loop_id"].(string); ok && v != "" {
			return v
		}
	}
	// `_loop_id` is not currently populated, so without this the guard keyed
	// every run as "global" → one parallel chat's pending ask-user blocked
	// EVERY other chat ("previous request still pending"). The run id is the
	// correct per-run key (stamped onto the dispatch ctx each turn).
	if rid := agentapi.RunIDFromContext(ctx); rid != "" {
		return rid
	}
	return "global"
}

// acquireInFlight returns (release-func, false) if the key was
// already busy. The release closure is a no-op in that case so
// callers can `defer release()` unconditionally.
func acquireInFlight(key string) (func(), bool) {
	inFlightGuard.mu.Lock()
	defer inFlightGuard.mu.Unlock()
	if inFlightGuard.active[key] {
		return func() {}, false
	}
	inFlightGuard.active[key] = true
	return func() {
		inFlightGuard.mu.Lock()
		defer inFlightGuard.mu.Unlock()
		delete(inFlightGuard.active, key)
	}, true
}
