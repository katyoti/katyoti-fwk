// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// Checks answers the guide's inbox checks by reading the inbox of the
// project in context, the way Triage does — but it sees answered requests,
// which Triage leaves out.
type Checks struct{}

// NewChecks builds the inbox checks.
func NewChecks() *Checks { return &Checks{} }

var _ api.InboxChecks = (*Checks)(nil)

func (Checks) RequestAnswered(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return guideapi.Failed("The inbox cannot be read here: %v", err).WithMsg("checks.inboxUnreadable", map[string]any{"error": err.Error()}), nil
	}
	inbox, err := findInbox(ctx, cwm)
	if err != nil {
		return guideapi.Failed("The inbox cannot be found: %v", err).WithMsg("checks.inboxNotFound", map[string]any{"error": err.Error()}), nil
	}
	if inbox == "" {
		return guideapi.Failed("This project has no inbox yet.").WithMsg("checks.noInbox", nil), nil
	}
	kids, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: inbox})
	if err != nil {
		return guideapi.Failed("The inbox cannot be listed: %v", err).WithMsg("checks.inboxUnlisted", map[string]any{"error": err.Error()}), nil
	}
	want := in.With["title"]
	pending := 0
	for _, resident := range kids.Objects {
		obj := resident
		m, _ := cwmapi.ModuleOfObjectID(resident.ID)
		if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: m, ID: resident.ID}); err == nil {
			obj = got.Object
		}
		if obj.Type != api.RequestType || (want != "" && obj.DisplayTitle() != want) {
			continue
		}
		switch st := statusOf(obj); st {
		case "", api.StatusPending:
			pending++
		case api.StatusDismissed:
		default:
			return guideapi.Passed("%q is answered (%s).", obj.DisplayTitle(), st).WithMsg("checks.answered", map[string]any{"title": obj.DisplayTitle(), "status": st}), nil
		}
	}
	if pending > 0 {
		return guideapi.Failed("The request is still waiting for your answer.").WithMsg("checks.stillWaiting", nil), nil
	}
	if want != "" {
		return guideapi.Failed("No request titled %q is in the inbox.", want).WithMsg("checks.noRequestTitled", map[string]any{"title": want}), nil
	}
	return guideapi.Failed("No answered request is in the inbox yet.").WithMsg("checks.noneAnswered", nil), nil
}
