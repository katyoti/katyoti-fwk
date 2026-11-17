// SPDX-License-Identifier: Apache-2.0

package approval

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// Service is the ApprovalService over a page server and Tailscale.
type Service struct {
	S  *Server
	TS *Tailscale
}

var _ api.ApprovalService = Service{}

// mine refuses the person's own approval acts from an agent's session.
func mine(ctx context.Context, what string) error {
	if c, ok := cause.CauseFrom(ctx); ok && c.Kind == cause.KindAgent {
		return fmt.Errorf("approval.%s: the person does it themselves (their own command) - not an agent's session (%s)", what, c.ID)
	}
	return nil
}

// Open opens the approval of one act.
func (v Service) Open(ctx context.Context, in api.ApprovalOpenInput) (api.ApprovalOpenOutput, error) {
	return v.S.Open(ctx, in)
}

// Status says where an approval stands.
func (v Service) Status(_ context.Context, in api.ApprovalStatusInput) (api.ApprovalStatusOutput, error) {
	return v.S.Status(in)
}

// Publish publishes the page to the tailnet: the person's own act.
func (v Service) Publish(ctx context.Context, in api.ApprovalPublishInput) (api.ApprovalPageOutput, error) {
	if err := mine(ctx, "Publish"); err != nil {
		return api.ApprovalPageOutput{}, err
	}
	if err := v.TS.Publish(ctx, v.S.Port); err != nil {
		return api.ApprovalPageOutput{}, err
	}
	return v.PageStatus(ctx, in)
}

// PageStatus reads the page's publication; a funnel-exposed page is not
// published.
func (v Service) PageStatus(ctx context.Context, _ api.ApprovalPublishInput) (api.ApprovalPageOutput, error) {
	out := api.ApprovalPageOutput{Port: v.S.Port}
	origin, err := v.TS.Origin(ctx)
	if err != nil {
		out.Note = err.Error()
		return out, nil
	}
	out.Origin = origin
	served, funnel, err := v.TS.Published(ctx, v.S.Port)
	if err != nil {
		out.Note = err.Error()
		return out, nil
	}
	out.Funnel = funnel
	out.Published = served && !funnel
	switch {
	case funnel:
		out.Note = "tailscale funnel exposes the page to the Internet: turn it off (tailscale funnel reset) - the page is for the person's tailnet only"
	case !served:
		out.Note = "not published: run katyoti profile passkey publish (tailscale serve)"
	}
	return out, nil
}

// RegisterOpen opens a one-time passkey registration: the person's own act.
func (v Service) RegisterOpen(ctx context.Context, in api.ApprovalRegisterInput) (api.ApprovalRegisterOutput, error) {
	if err := mine(ctx, "RegisterOpen"); err != nil {
		return api.ApprovalRegisterOutput{}, err
	}
	return v.S.RegisterOpen(ctx, in.Person)
}
