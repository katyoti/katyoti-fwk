// SPDX-License-Identifier: Apache-2.0

package waiting

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE CARD IN A CHAT - a question asked from a conversation
// shows there as the card « À faire » draws for it: one frame for one human
// task, « À faire »'s own (api.HumanTaskCard, set by
// planification), given the question as the waiting list reads it - its
// kind, what it is about, its question, its options, who asks - and the
// nodes answering it, the same that answer it in « À faire », reporting the
// same to-do. It arrives resolved - no answer form keeps its ui block -, as a
// panel's tree is drawn as it comes. Once answered, in the chat or in « À
// faire », the card keeps what it asked and says who answered and what - a
// relayed answer naming who relayed it and the proof the person gave -, with
// nothing left to answer.

// ChatCard is a human task's card for the chat that asks it, resolved; its
// answered form once its to-do carries the answer.
func ChatCard(ctx context.Context, t schedapi.Todo) render.Node {
	lang := LanguageOf(ctx, "")
	it, ok := humanTaskItem(t, time.Now())
	if !ok || api.HumanTaskCard == nil {
		return node{"kind": "card", "id": SourceInbox + ":" + t.ID, "children": []node{{"kind": "text", "content": t.Title}}}
	}
	if it.Module == "" && it.About != "" {
		it.Module, _ = cwmapi.ModuleOfObjectID(it.About)
	}
	answered := ""
	if a, done := api.AnswerOf(t); done {
		answered = answeredLine(a, lang)
	} else {
		it.Answer = controlsOf(ctx, it, lang)
	}
	return resolved(api.HumanTaskCard(ctx, it, answered, lang))
}

// answeredLine says who answered a question and what, and who relayed it
// with which proof - the line an answered card shows in place of its answer
// area, in the person's language.
func answeredLine(a api.HumanAnswer, lang string) string {
	what := a.Answer
	if what == "" {
		what = a.Verdict
	}
	if what == "" {
		what = a.Summary
	}
	text := fmt.Sprintf(L(lang, "answered"), a.By.ID, what)
	if a.Relayed != nil {
		text += " · " + fmt.Sprintf(L(lang, "relayed"), a.Relayed.By, a.Relayed.Confirmed)
	}
	return text
}

// resolved is a card with its answer forms resolved: their inline schemas
// laid out, no ui block left (render.ResolveComponents, nothing fetched - the
// cards answer through inline schemas).
func resolved(n render.Node) render.Node {
	raw, err := json.Marshal(n)
	if err != nil {
		return n
	}
	var tree any
	if json.Unmarshal(raw, &tree) != nil {
		return n
	}
	if !render.HasComponents(tree) {
		return render.Node(tree.(map[string]any))
	}
	out, err := render.ResolveComponents(tree, render.ComponentResolver{Fetch: func(render.SchemaRef, string, string) (map[string]any, error) {
		return nil, fmt.Errorf("a card answers through inline schemas")
	}})
	if err != nil {
		return n
	}
	if m, ok := out.(map[string]any); ok {
		return m
	}
	return n
}
