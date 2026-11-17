// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"fmt"
)

// A RUN LENDS ITS PANEL TO THE METHOD IT CALLS: the run that
// owns a panel (a long method's run) puts its Handle on the
// call's context (WithHandle) and installs its Reporter (ReportTo). In process
// the method's progress.Emit reaches that handle. Over the bus the KTSP sender
// carries the handle as a Ref - its id, its last sequence number and its scope
// - the serving side resumes a lent Handle at that sequence (Resume) under
// the method's context, so the method's own progress.Emit feeds the panel, and
// the answer gives the lent handle's last sequence back, which the run's
// handle takes (Advance) before it completes the panel. The method never
// completes or closes a panel it was lent: the run does. While its call runs
// the caller sends nothing on the panel.

// Ref is a panel handle as it travels with a call: what a lent Handle resumes
// from.
type Ref struct {
	ID       string `json:"id"`
	Seq      int64  `json:"seq"`
	Tenant   string `json:"tenant"`
	Universe string `json:"universe"`
}

// Validate refuses a ref that names no panel.
func (r Ref) Validate() error {
	if !ValidID(r.ID) {
		return fmt.Errorf("panel ref: %q is no panel id", r.ID)
	}
	if r.Seq < 0 {
		return fmt.Errorf("panel ref %s: negative sequence %d", r.ID, r.Seq)
	}
	return nil
}

// Ref is the handle as it travels with a call: its id, its last sequence
// number and its scope.
func (h *Handle) Ref() Ref {
	h.mu.Lock()
	defer h.mu.Unlock()
	return Ref{ID: h.id, Seq: h.seq, Tenant: h.scope.Tenant, Universe: h.scope.Universe}
}

// Resume is the handle a run lent to the call serving it, resumed at the
// ref's sequence: its events are numbered after the run's last one. It sends
// no open; it refuses to complete or close the panel - the run that owns it
// does.
func Resume(ref Ref, publish Publish) (*Handle, error) {
	if publish == nil {
		return nil, fmt.Errorf("panel: resume: no publish")
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	return &Handle{
		id: ref.ID, scope: Scope{Tenant: ref.Tenant, Universe: ref.Universe},
		publish: publish, seq: ref.Seq, state: StateOpen, lent: true,
	}, nil
}

// Lent says whether the handle was resumed from a ref: it reports, never
// completes.
func (h *Handle) Lent() bool { return h.lent }

// Advance takes back the last sequence number a lent handle sent: the next
// event is numbered after it. A sequence at or before the handle's own is
// ignored.
func (h *Handle) Advance(seq int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if seq > h.seq {
		h.seq = seq
	}
}

type handleKey struct{}

// WithHandle puts h on ctx: the panel a call made with ctx reports into
// (a KTSP sender carries it as a Ref).
func WithHandle(ctx context.Context, h *Handle) context.Context {
	if h == nil {
		return ctx
	}
	return context.WithValue(ctx, handleKey{}, h)
}

// HandleFrom is the panel handle ctx carries, and whether it carries one.
func HandleFrom(ctx context.Context) (*Handle, bool) {
	h, ok := ctx.Value(handleKey{}).(*Handle)
	return h, ok && h != nil
}

// Lend puts h on ctx and installs its Reporter on it (ReportTo with no
// target node): the work run with the context it answers reports its
// progress into the panel, in process or through a KTSP call.
func Lend(ctx context.Context, h *Handle) context.Context {
	if h == nil {
		return ctx
	}
	return ReportTo(WithHandle(ctx, h), h, "")
}
