// SPDX-License-Identifier: Apache-2.0

// Package ask_user — silent_ask.go is the no-user-channel impl.
// Used when UserCommsInput is `none`.
//
// Returns ErrNoUserChannel immediately. The error surfaces to the
// agent loop as a tool error; the LLM sees a clear message that it
// has no way to ask the user and must proceed with what it has.
// The user_comms_adapter plugin's `none` system-prompt block warns
// the LLM about this in advance — see
// internal/extensions/agent/local/user_comms_adapter/.
package ask_user

import (
	"context"
	"fmt"
)

type silentChannel struct{}

func (c *silentChannel) Ask(_ context.Context, _ AskInput) (AskOutput, error) {
	return AskOutput{}, fmt.Errorf("%w (UserCommsInput=none — this task has no user channel by design)", ErrNoUserChannel)
}
