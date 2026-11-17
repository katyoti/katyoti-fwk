// SPDX-License-Identifier: Apache-2.0

// Package ask_user — bubble_ask.go is the "agent as human" channel, used when
// UserCommsInput is `bubble_to_parent`, `via_agent` or `via_leader`.
//
// It is a THIN WRAPPER over commsapi.Service: the child↔parent conversation
// (the question a human task on the parent's holding list, the parent woken,
// the answer the task's result -) lives in the comms service so
// non-agent tasks (the simulator) drive the exact same channel. This tool just
// adapts AskInput/AskOutput to the service's request/answer shapes.
package ask_user

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	commsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comms/api"
)

// ErrNoParentChannel surfaces when a run asks to bubble to a parent but has no
// parent (a root run) or the comms service is unreachable. Wraps
// ErrNoUserChannel so callers detecting "no channel" still match.
var ErrNoParentChannel = fmt.Errorf("%w (bubble_to_parent requires a parent run + comms service)", ErrNoUserChannel)

type bubbleChannel struct{}

func (c *bubbleChannel) Ask(ctx context.Context, in AskInput) (AskOutput, error) {
	svc, err := application.UseService[commsapi.Service](ctx)
	if err != nil {
		return AskOutput{}, fmt.Errorf("%w [comms.Service: %v]", ErrNoParentChannel, err)
	}
	ans, err := svc.Ask(ctx, commsapi.AskRequest{
		Question: in.PromptText(),
		Options:  in.Options,
	})
	if err != nil {
		return AskOutput{}, err
	}
	typ := ans.Type
	if typ == "" {
		typ = "text"
	}
	return AskOutput{Type: typ, Answer: ans.Text}, nil
}
