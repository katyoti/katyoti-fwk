// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// PLANIFICATION'S QUESTIONS IN « À FAIRE » - until its person
// steps, decisions, hand tests, unblocks and acceptances ask through human
// tasks (F-081), planification contributes them to the
// inbox's one list: each of its items as the kind of question it is, the
// entries its steps file answered for by its items only, and each item's
// buttons as its « À faire » card has them. What the person put on hold
// stays out of the list, as it stays out of the count.

// WaitingSourceName is planification's source in « À faire ».
const WaitingSourceName = "planification"

// WaitingSource is planification's contribution to « À faire ».
func WaitingSource() inboxapi.WaitingSource {
	return inboxapi.WaitingSource{Name: WaitingSourceName, Items: waitingItems, Answers: answersFor, Controls: waitingControls}
}

// waitingItems are planification's items as questions.
func waitingItems(ctx context.Context) ([]inboxapi.WaitingItem, error) {
	ctx = withReadCache(ctx)
	items, err := personItems(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]inboxapi.WaitingItem, 0, len(items))
	for _, it := range items {
		if it.Held != nil {
			continue
		}
		w := inboxapi.WaitingItem{
			Key: WaitingSourceName + ":" + it.Key, Kind: KindOfItem(it), Title: it.Title, Asks: it.Asks,
			About: it.Object, Request: it.Request, Module: it.Module, Since: it.Since, Todo: it.Todo, Entry: it.Ask,
			Ref: it.Key, Own: it,
		}
		if it.Request != "" && it.Request != it.Object {
			w.About = it.Request
		}
		out = append(out, w)
	}
	return out, nil
}

// KindOfItem is the kind of question an item of the person's queue is: a
// decision decides, a hand test is a test by hand, a validation approves -
// a want to accept or confirm accepts -, a question is an answer to give, an
// unblock unblocks, a notice informs.
func KindOfItem(it api.PersonItem) string {
	switch it.Action {
	case api.ActionDecide:
		return inboxapi.KindDecide
	case api.ActionTest:
		return inboxapi.KindTestByHand
	case api.ActionValidate:
		if len(it.Wants) > 0 || it.Confirm != nil {
			return inboxapi.KindAccept
		}
		return inboxapi.KindApprove
	case api.ActionAnswer:
		return inboxapi.KindForm
	case api.ActionUnblock:
		return inboxapi.KindUnblock
	}
	return "notice"
}

// answersFor says an inbox entry is planification's to show: one its steps
// or its requests' questions filed - a gate's confirmation and a notice are
// the inbox's own items.
func answersFor(e inboxapi.Entry) bool {
	if len(e.RequestedBy) < len(api.Owner)+1 || e.RequestedBy[:len(api.Owner)+1] != api.Owner+":" {
		return false
	}
	if len(e.RequestedBy) >= len(askPrefix) && e.RequestedBy[:len(askPrefix)] == askPrefix {
		return false
	}
	return e.Expects != inboxapi.ExpectsNotice
}

// waitingControls are an item's buttons as its « À faire » card has them.
func waitingControls(_ context.Context, w inboxapi.WaitingItem, lang string) []render.Node {
	it, ok := w.Own.(api.PersonItem)
	if !ok {
		return nil
	}
	nodes := cardActions(it, lang)
	out := make([]render.Node, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, render.Node(n))
	}
	return out
}
