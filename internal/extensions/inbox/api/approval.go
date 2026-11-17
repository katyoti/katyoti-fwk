// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// ApprovalServiceDef declares the one-tap phone approval: the
// approval page the serving process runs on 127.0.0.1 and publishes to the
// person's tailnet with tailscale serve - an act's card, Approve with Face ID
// (the person's passkey, iam), Refuse - and what an act tool asks of it.
var ApprovalServiceDef = meta.ServiceDef{
	Name:    "Approval",
	BusName: "approval",
	Short:   "The one-tap approval of the person's acts from their phone, over Tailscale",
	Long:    `An act waiting on the person opens an approval: a link to its card on the approval page, reached from the phone over the person's tailnet; Approve signs it with the person's passkey (Face ID), Refuse runs nothing. Publishing the page and opening a passkey registration are the person's own acts: refused from an agent's session.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Open":         meta.Method("Open the approval of one act of the person: its handle and the link to its card"),
		"Status":       {Short: "Where an approval stands: pending, approved (with the proof its act carries) or refused", ReadOnly: true},
		"Publish":      meta.Method("Publish the approval page to the person's tailnet with tailscale serve - never funnel - the person's own act"),
		"PageStatus":   {Short: "The approval page: its origin, its port, whether it is published to the tailnet and never to the Internet", ReadOnly: true},
		"RegisterOpen": meta.Method("Open a passkey registration for the person: a one-time link for their phone, ten minutes - the person's own act"),
	},
}

// ApprovalService is the one-tap phone approval.
type ApprovalService interface {
	Open(ctx context.Context, in ApprovalOpenInput) (ApprovalOpenOutput, error)
	Status(ctx context.Context, in ApprovalStatusInput) (ApprovalStatusOutput, error)
	Publish(ctx context.Context, in ApprovalPublishInput) (ApprovalPageOutput, error)
	PageStatus(ctx context.Context, in ApprovalPublishInput) (ApprovalPageOutput, error)
	RegisterOpen(ctx context.Context, in ApprovalRegisterInput) (ApprovalRegisterOutput, error)
}

// ApprovalOpenInput is the act an approval is of, as the card shows it.
type ApprovalOpenInput struct {
	Person string `json:"person" desc:"The person (their profile handle)." required:"true"`
	Act    string `json:"act" desc:"The act's key <verb>:<object id>." required:"true"`
	Title  string `json:"title,omitempty" desc:"What the act is about in one line: the card's title."`
	What   string `json:"what,omitempty" desc:"What approving does: the card's text."`
}

// ApprovalOpenOutput is the approval's handle and its card's link.
type ApprovalOpenOutput struct {
	Handle string `json:"handle"`
	Link   string `json:"link" desc:"The card on the approval page, for the phone."`
}

// ApprovalStatusInput names an approval.
type ApprovalStatusInput struct {
	Handle string `json:"handle" cli:"arg" required:"true"`
}

// The states of an approval.
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalRefused  = "refused"
)

// ApprovalStatusOutput is where an approval stands; Proof is what the act
// carries once approved (its guard checks it with the passkey service).
type ApprovalStatusOutput struct {
	State string `json:"state" desc:"pending or approved or refused"`
	Act   string `json:"act"`
	Proof string `json:"proof,omitempty"`
}

// ApprovalPublishInput takes nothing.
type ApprovalPublishInput struct{}

// ApprovalPageOutput is the approval page as published.
type ApprovalPageOutput struct {
	Origin    string `json:"origin,omitempty" desc:"https://<pc>.<tailnet>.ts.net - the passkey's relying party."`
	Port      int    `json:"port"`
	Published bool   `json:"published" desc:"tailscale serve forwards the origin to the page."`
	Funnel    bool   `json:"funnel" desc:"The page is exposed to the Internet by tailscale funnel: then it is NOT published."`
	Note      string `json:"note,omitempty"`
}

// ApprovalRegisterInput names the person registering a passkey.
type ApprovalRegisterInput struct {
	Person string `json:"person" required:"true"`
}

// ApprovalRegisterOutput is the one-time registration link.
type ApprovalRegisterOutput struct {
	Link    string `json:"link"`
	Expires string `json:"expires"`
}
