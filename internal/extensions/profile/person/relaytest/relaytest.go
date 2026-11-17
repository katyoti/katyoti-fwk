// SPDX-License-Identifier: Apache-2.0

// Package relaytest sets, for one test, whether the person has relaying on and
// the cause of an agent's session relaying a person's act — what the guards of
// the person's acts (person.AcceptRelayed) read.
package relaytest

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/person"
)

// Set turns relaying on or off for the rest of tb, restored after it.
func Set(tb testing.TB, on bool) {
	tb.Helper()
	was := person.RelaySwitch
	person.RelaySwitch = func() bool { return on }
	tb.Cleanup(func() { person.RelaySwitch = was })
}

// Relaying is ctx caused by an agent's session (agent) relaying who's act
// (its key), confirmed at the prompt.
func Relaying(ctx context.Context, agent, who, act string) context.Context {
	return cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: agent,
		Relay: &cause.Relay{Person: who, Act: act, Confirmed: cause.ConfirmedPrompt, At: "2026-10-11T16:00:00Z"}})
}
