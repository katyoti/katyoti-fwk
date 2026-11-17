// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// GATE CONFIRMATIONS ASKED IN THE INBOX - before
// IN-039, a person's requirement was asked as an inbox validation expecting
// a verdict, the object asked-in it. A wait still follows one asked then: its
// answer, through the inbox's event (GateAnswered), confirms the requirement
// by whoever answered; a refusal ends the wait. These readers go with the
// entries' archive.

// confirmationEntries are the inbox validations asked for the wait: the
// entries the object is asked-in that the planner filed for this gate and
// wait.
func confirmationEntries(ctx context.Context, o cwmapi.Object, w api.GateWaitFields) ([]resolution, error) {
	var out []resolution
	for _, e := range o.Edges {
		if e.Kind != api.EdgeAskedIn {
			continue
		}
		a, err := gateEntry(ctx, e.To)
		if err != nil {
			return nil, err
		}
		what, wait, _ := strings.Cut(strings.TrimPrefix(a.RequestedBy, askPrefix), "@")
		gate, req, ok := strings.Cut(what, "/")
		if !strings.HasPrefix(a.RequestedBy, askPrefix) || !ok || gate != w.Gate || wait != w.Since {
			continue
		}
		by := cwmapi.Actor{ID: a.ResolvedBy, Kind: "person"}
		if by.ID == "" {
			by.ID = "inbox"
		}
		out = append(out, resolution{Requirement: req, Ask: e.To, Entry: true,
			Answered: a.Status != "" && a.Status != inboxapi.StatusPending,
			Approved: a.Status == inboxapi.StatusApproved, By: by, Note: a.DecisionNote})
	}
	return out, nil
}

// gateEntry reads an inbox entry.
func gateEntry(ctx context.Context, id string) (inboxapi.Entry, error) {
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		return inboxapi.Entry{}, err
	}
	got, err := inbox.Get(ctx, inboxapi.GetInput{ID: id})
	if err != nil {
		return inboxapi.Entry{}, err
	}
	return got.Entry, nil
}

// dismissEntry takes back a gate's inbox validation no one answered.
func dismissEntry(ctx context.Context, id string) error {
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil {
		return err
	}
	_, err = inbox.Dismiss(ctx, inboxapi.DismissInput{ID: id})
	return err
}

// GateAnswered follows the objects an answered inbox validation is about; it
// reports whether the entry was one of the planner's gate validations.
func (s *Service) GateAnswered(ctx context.Context, entry string) (bool, error) {
	got, err := gateEntry(ctx, entry)
	if err != nil {
		return false, err
	}
	if !strings.HasPrefix(got.RequestedBy, askPrefix) {
		return false, nil
	}
	if got.Status == "" || got.Status == inboxapi.StatusPending {
		return true, nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return true, err
	}
	waiting, err := incoming(ctx, cwm, entry, api.EdgeAskedIn)
	if err != nil {
		return true, err
	}
	for _, id := range waiting {
		if err := s.GateProgress(ctx, id); err != nil {
			return true, err
		}
	}
	return true, nil
}
