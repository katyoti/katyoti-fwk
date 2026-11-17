// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// A page source may answer with a panel: its answer names
// the panel, the panel's open carries the skeleton with its pushed sections
// at once, and the producer's ops patch each section by its id, in the
// panel's sequence, before the page completes; the person's cancel ends the
// producer's context.
func TestAPageSourceAnswersAPanelWhoseSectionsArriveByID(t *testing.T) {
	ctx := context.Background()
	publish, got := recorder(t)

	// The page source: the skeleton now, each section once it is built.
	source := func(ctx context.Context) (render.Answer, <-chan struct{}, error) {
		skeleton := render.Node{"kind": "vbox", "children": []any{
			render.Node{"kind": "heading", "text": "Delivery"},
			render.Node{"kind": "vbox"}.AsPushedSection("lane:core", render.Placeholder{Kind: render.PlaceholderCards, Count: 3}),
			render.Node{"kind": "list", "data": []any{}, "item": render.Node{"kind": "text", "content": "{{title}}"}}.AsPushedSection("backlog", render.Placeholder{Kind: render.PlaceholderRows}),
		}}
		page, err := OpenPage(ctx, scope, publish, "Delivery", "ktw-test:views/delivery", skeleton)
		if err != nil {
			return render.Answer{}, nil, err
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = page.Replace(ctx, "lane:core", render.Node{"kind": "text", "content": "Core: 3 increments"})
			_ = page.Append(ctx, "backlog", map[string]any{"title": "RQ-1"}, map[string]any{"title": "RQ-2"})
			_ = page.Complete(ctx)
		}()
		return page.Answer(), done, nil
	}

	answer, done, err := source(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := answer.Validate(); err != nil || answer.Panel == "" || answer.Tree != nil {
		t.Fatalf("the source answered %+v (%v), want a panel by its id", answer, err)
	}
	<-done
	events := got()
	if len(events) != 5 {
		t.Fatalf("sent %d events, want open, two ops, the page's complete op and the panel's complete: %+v", len(events), events)
	}
	open := events[0].event
	if open.Kind != KindOpen || open.Panel != answer.Panel || open.Open.Answer.Tree == nil {
		t.Fatalf("the first event is %+v, want the open carrying the skeleton", open)
	}
	raw, _ := json.Marshal(open.Open.Answer.Tree.Inline)
	var tree struct {
		Children []map[string]any `json:"children"`
	}
	_ = json.Unmarshal(raw, &tree)
	if len(tree.Children) != 3 || tree.Children[1]["id"] != "lane:core" || tree.Children[1]["placeholder"] == nil || tree.Children[1]["load"] != nil {
		t.Fatalf("the skeleton is %s, want its pushed sections by id with their placeholders and no load", raw)
	}
	r := events[1].event
	if r.Kind != KindOps || r.Ops[0].Op != render.OpReplace || r.Ops[0].Target != "lane:core" || r.Ops[0].Node["id"] != "lane:core" || r.Seq != 2 {
		t.Fatalf("the first section arrived as %+v, want a replace of lane:core keeping its id at seq 2", r)
	}
	a := events[2].event
	if a.Ops[0].Op != render.OpAppend || a.Ops[0].Target != "backlog" || len(a.Ops[0].Items) != 2 {
		t.Fatalf("the backlog arrived as %+v, want an append of two items", a)
	}
	if c := events[3].event; c.Kind != KindOps || c.Ops[0].Op != render.OpComplete {
		t.Fatalf("then %+v, want the page's complete op", c)
	}
	if c := events[4].event; c.Kind != KindComplete || c.Complete.Status != "ok" {
		t.Fatalf("then %+v, want the panel complete", c)
	}
	for i, e := range events {
		if e.event.Seq != int64(i+1) {
			t.Fatalf("event %d has seq %d: the ops are not in the panel's sequence", i, e.event.Seq)
		}
	}

	// A skeleton off the grammar is refused before anything goes out.
	if _, err := OpenPage(ctx, scope, publish, "x", "y", render.Node{"kind": "vbox", "children": []any{
		render.Node{"kind": "vbox", "placeholder": map[string]any{"kind": "spinner"}, "id": "a"},
	}}); err == nil {
		t.Fatal("a pushed section with an unknown placeholder is taken")
	}
	if _, err := OpenPage(ctx, scope, publish, "x", "y", render.Node{"kind": "vbox", "children": []any{
		render.Node{"kind": "vbox", "placeholder": map[string]any{"kind": "rows"}},
	}}); err == nil {
		t.Fatal("a pushed section with no id is taken")
	}

	// The person's cancel ends the producer's context.
	page, err := OpenPage(ctx, scope, publish, "Slow", "p", render.Node{"kind": "vbox", "children": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var deliver func([]byte)
	subscribe := func(_ context.Context, subject string, fn func([]byte)) (func(), error) {
		if subject != page.Handle().InputSubject() {
			t.Errorf("watched %s, want the panel's input subject", subject)
		}
		mu.Lock()
		deliver = fn
		mu.Unlock()
		return func() {}, nil
	}
	pctx, stop, err := page.CancelContext(ctx, subscribe)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	mu.Lock()
	msg, _ := json.Marshal(Input{Panel: page.Handle().ID(), Kind: InputMessage, Text: "hi"})
	deliver(msg)
	mu.Unlock()
	if pctx.Err() != nil {
		t.Fatal("a message input cancelled the producer")
	}
	mu.Lock()
	msg, _ = json.Marshal(Input{Panel: page.Handle().ID(), Kind: InputCancel})
	deliver(msg)
	mu.Unlock()
	select {
	case <-pctx.Done():
	case <-time.After(time.Second):
		t.Fatal("the person's cancel did not end the producer's context")
	}
}
