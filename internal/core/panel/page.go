// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// A PAGE IS A PANEL WHOSE PRODUCER IS A METHOD: a page source
// may answer render.Answer{Panel: id} instead of a tree. Its skeleton - the
// page tree whose slow parts are pushed sections (render.AsPushedSection: an
// id and a placeholder, no load) - is the panel's open, drawn at once; its
// producer then patches each section by its id with the one op set
// as it builds it, and completes. The Page is the panel
// handle emitting those ops: no other answer, no other op. The person's
// cancel on the panel (its `cancel` input) ends the producer's context.

// Page is a page panel's emitter: the handle of the panel its skeleton opened.
type Page struct {
	h *Handle
}

// OpenPage opens a page panel showing skeleton - an inline page tree (a page
// root) - and answers its emitter. title names the panel; owner is who
// produces it.
func OpenPage(ctx context.Context, scope Scope, publish Publish, title, owner string, skeleton render.Node) (*Page, error) {
	if skeleton == nil {
		return nil, fmt.Errorf("panel: page: no skeleton")
	}
	h, err := OpenPanel(ctx, scope, publish, Open{Title: title, Owner: owner, Answer: render.Answer{Tree: &render.Spec{Inline: skeleton}}})
	if err != nil {
		return nil, err
	}
	return &Page{h: h}, nil
}

// Answer is what the page source answers: the panel, by its id.
func (p *Page) Answer() render.Answer { return render.Answer{Panel: p.h.ID()} }

// Handle is the page's panel handle.
func (p *Page) Handle() *Handle { return p.h }

// Replace swaps the section id for node, which keeps that id.
func (p *Page) Replace(ctx context.Context, id string, node render.Node) error {
	return p.h.Patch(ctx, render.Op{Op: render.OpReplace, Target: id, Node: node.With("id", id)})
}

// Append adds items to the data of the node id (a list, a table, a tree or
// lanes).
func (p *Page) Append(ctx context.Context, id string, items ...any) error {
	return p.h.Patch(ctx, render.Op{Op: render.OpAppend, Target: id, Items: items})
}

// Merge lays value over the fields of the node id (RFC 7396).
func (p *Page) Merge(ctx context.Context, id string, value map[string]any) error {
	return p.h.Patch(ctx, render.Op{Op: render.OpMerge, Target: id, Value: value})
}

// Fail shows the section id failed with message: only that section, drawn
// as an error card - the rest of the page stays.
func (p *Page) Fail(ctx context.Context, id, message string) error {
	return p.h.Patch(ctx, render.Op{Op: render.OpFail, Target: id, Message: message})
}

// Complete ends the page's loading and the panel's work: a section still
// waiting shows that it was not loaded.
func (p *Page) Complete(ctx context.Context) error {
	if err := p.h.Patch(ctx, render.Op{Op: render.OpComplete}); err != nil {
		return err
	}
	return p.h.Complete(ctx, "ok", nil)
}

// Subscribe receives each payload published on subject until stop - the
// bus's Subscribe (MessageBus, rule) as a function.
type Subscribe func(ctx context.Context, subject string, deliver func(payload []byte)) (stop func(), err error)

// CancelContext is ctx ended when the person cancels the page panel (its
// `cancel` input): the producer builds on it. stop releases the watch.
func (p *Page) CancelContext(ctx context.Context, subscribe Subscribe) (context.Context, func(), error) {
	cctx, cancel := context.WithCancel(ctx)
	unsub, err := subscribe(cctx, p.h.InputSubject(), func(payload []byte) {
		var in Input
		if json.Unmarshal(payload, &in) == nil && in.Panel == p.h.ID() && in.Kind == InputCancel {
			cancel()
		}
	})
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("panel %s: watch its cancel: %w", p.h.ID(), err)
	}
	return cctx, func() { unsub(); cancel() }, nil
}
