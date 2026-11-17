// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// EVERY WAITING ITEM IN « À FAIRE » (the person's
// choice 2026-11-02: « keep the previous view, put all in this view, add the
// missing pieces, remove nothing »): with IN-039's switch on, the person queue
// also lists the inbox's own questions - its human tasks: a validation, a test
// by hand, a decision or a choice, a form, data to provide, a permission, an
// unblock, an acceptance -, the entries no item of the queue carries (a
// gate's confirmation: inboxItems leaves every planification asker's entry to
// its step) and the conversations running in the person's sessions, read from
// the inbox's waiting list (inboxapi.WaitingService), each one more card of
// the same page. Planification's own steps asked as human tasks are its own
// to-dos. A question keeps the nodes the inbox answers it with - its forms,
// its « Pourquoi ? » modal - drawn as they are: nothing is rendered twice.

// kindActions is the action a kind of question asks, as the queue's sections
// say it.
var kindActions = map[string]string{
	inboxapi.KindApprove: api.ActionValidate, inboxapi.KindAccept: api.ActionValidate,
	inboxapi.KindTestByHand: api.ActionTest,
	inboxapi.KindDecide:     api.ActionDecide, inboxapi.KindChoose: api.ActionDecide,
	inboxapi.KindForm: api.ActionAnswer, inboxapi.KindProvide: api.ActionAnswer, inboxapi.KindPermission: api.ActionAnswer,
	inboxapi.KindUnblock: api.ActionUnblock,
}

// actionOfKind is a kind's action: answer for a kind a pack adds.
func actionOfKind(kind string) string {
	if a, ok := kindActions[kind]; ok {
		return a
	}
	return api.ActionAnswer
}

// inboxQueueItems are the inbox's human tasks, the entries the queue does not
// carry (carried: the entries its items already ask in) and the
// conversations, as the queue's items, in the person's language; none when
// the inbox is not here. An error says why the inbox's waiting list could not
// be read: the queue says so.
func inboxQueueItems(ctx context.Context, lang string, carried map[string]bool) ([]api.PersonItem, inboxapi.WaitingOutput, error) {
	waiting, err := application.UseService[inboxapi.WaitingService](ctx)
	if err != nil || waiting == nil {
		return nil, inboxapi.WaitingOutput{}, nil // no inbox in this binary: nothing of it waits
	}
	out, err := waiting.List(ctx, inboxapi.WaitingInput{Language: lang, Answers: true})
	if err != nil {
		return nil, inboxapi.WaitingOutput{}, err
	}
	cwm, _ := cwmOf(ctx)
	var items []api.PersonItem
	for _, g := range out.Groups {
		holder, label := holderOf(g, lang)
		for _, it := range g.Items {
			if !drawnHere(it, carried) {
				continue
			}
			p := humanTaskQueueItem(ctx, cwm, it)
			p.Holder, p.HolderLabel = holder, label
			items = append(items, p)
		}
	}
	// The inbox's notices no item carries, each with its « Écarter »: under
	// « Pour information », as the previous view's notices.
	for _, it := range out.Recent {
		if !drawnHere(it, carried) || (it.Source == inboxapi.WaitingSourceInbox && it.Entry == "") {
			continue
		}
		items = append(items, api.PersonItem{Key: it.Key, Action: api.ActionNotice, Object: it.About, Module: it.Module,
			Title: it.Title, Asks: it.Asks, Since: it.Since, Answer: it.Answer, Kind: "notice", AskedBy: it.AskedBy})
	}
	// The conversations, in the inbox's order - those waiting on the person
	// first -, each saying whether it waits on them and what it asks.
	for _, c := range out.Conversations {
		state := ql(lang, "conv_running")
		if c.Waiting {
			state = ql(lang, "conv_waiting")
		}
		items = append(items, api.PersonItem{
			Key: "chat:" + c.Panel, Action: api.ActionConversation, Title: c.Title, Asks: c.Last, Since: c.Updated,
			Chat: c.Panel, Kind: inboxapi.KindConversation, KindLabel: state,
		})
	}
	if cwm != nil {
		placeItems(ctx, cwm, items)
	}
	return items, out, nil
}

// drawnHere says a waiting item is one more card of the queue
// : the inbox's own questions - human tasks, and
// entries no item carries yet - and every item another source contributes
// (agent-memory's quarantined memory, ...), drawn with its source's controls
// (Answer); never planification's own source, whose items are the queue's own
// to-dos.
func drawnHere(it inboxapi.WaitingItem, carried map[string]bool) bool {
	switch {
	case it.Source == WaitingSourceName:
		return false
	case it.Entry != "" && carried[it.Entry]:
		return false
	case it.Source == inboxapi.WaitingSourceInbox:
		return it.Todo != "" || it.Entry != ""
	}
	return true
}

// holderOf is who holds a group of the waiting list: the person (mine) or a
// holding list, named with its members, in the person's language.
func holderOf(g inboxapi.WaitingGroup, lang string) (string, string) {
	if g.Mine {
		return holderMine, ql(lang, "mine")
	}
	names := make([]string, 0, len(g.Members))
	for _, m := range g.Members {
		if m.Kind == "agent" {
			names = append(names, fmt.Sprintf(ql(lang, "agent_member"), m.ID))
			continue
		}
		names = append(names, m.ID)
	}
	title := g.Title
	if title == "" {
		title = g.List
	}
	return "list:" + g.List, fmt.Sprintf(ql(lang, "holding_list"), title, strings.Join(names, ", "))
}

// holderMine is the person's own questions' holder.
const holderMine = "mine"

// kindOf is an item's kind of question: its own (an inbox question), else
// the kind planification's items are counted as (KindOfItem).
func kindOf(it api.PersonItem) string {
	if it.Kind != "" {
		return it.Kind
	}
	return KindOfItem(it)
}

// ofKind keeps the items of a kind of question - overdue: those overdue;
// conversation: the conversations.
func ofKind(items []api.PersonItem, kind string) []api.PersonItem {
	out := []api.PersonItem{}
	for _, it := range items {
		switch {
		case kind == overdueKind && it.Overdue,
			kind == api.ActionConversation && it.Action == api.ActionConversation,
			kind != overdueKind && kind != api.ActionConversation && kindOf(it) == kind:
			out = append(out, it)
		}
	}
	return out
}

// overdueKind is the kind filter keeping the overdue questions.
const overdueKind = "overdue"

// humanTaskQueueItem is one human task as a card of the queue: its kind and
// action, what it is about - its title too -, its question, its options, who
// asks it and the nodes answering it.
func humanTaskQueueItem(ctx context.Context, cwm cwmapi.CWMService, it inboxapi.WaitingItem) api.PersonItem {
	p := api.PersonItem{
		Key: it.Key, Action: actionOfKind(it.Kind), Object: it.About, Request: it.Request, Module: it.Module, Title: it.Title,
		Asks: it.Asks, Options: it.Options, Todo: it.Todo, Since: it.Since,
		Kind: it.Kind, KindLabel: it.Label, AskedBy: it.AskedBy, Answer: it.Answer,
		Due: it.Due, Overdue: it.Overdue, Proof: it.Proof, AnswerableBy: it.AnswerableBy, Satisfies: it.Satisfies,
	}
	if it.ClaimedBy != nil {
		p.TakenBy = it.ClaimedBy.ID
	}
	if it.About != "" && cwm != nil {
		if t := titleOfID(ctx, cwm, it.About); t != it.About {
			p.AboutTitle = t
		}
	}
	return p
}

// A human task's card in a chat is « À faire »'s:
// wherever planification is linked, the inbox draws it with HumanTaskCard.
func init() { inboxapi.HumanTaskCard = HumanTaskCard }

// HumanTaskCard is one human task's card - « À faire »'s own, the one frame of
// a human task wherever it shows: the chat that asks
// it draws it (inboxapi.HumanTaskCard, set by planification's extension).
// Answered, it says who answered and what in place of its answer area.
func HumanTaskCard(ctx context.Context, it inboxapi.WaitingItem, answered, lang string) render.Node {
	cwm, _ := cwmOf(ctx)
	p := humanTaskQueueItem(ctx, cwm, it)
	if answered != "" {
		p.Answer, p.Answered = nil, answered
	}
	return queueCard(p, lang, "")
}
