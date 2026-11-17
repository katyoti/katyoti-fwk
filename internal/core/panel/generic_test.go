// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// The generic types are panel types like any pack's: each valid, its tree on
// the page grammar, the nodes a producer patches in it.
func TestTheGenericPanelTypesAreValidPanelTypes(t *testing.T) {
	want := map[string]string{TypeProgress: StepsNode, TypeLog: LogNode, TypeQuestion: QuestionNode, TypeChat: ChatArtifactsNode}
	if got := GenericTypeURIs(); len(got) != len(want) {
		t.Fatalf("generic types %v, want %d", got, len(want))
	}
	for uri, node := range want {
		decl, ok := GenericTypes()[uri]
		if !ok {
			t.Fatalf("%s is no generic type", uri)
		}
		if err := decl.Validate(); err != nil {
			t.Errorf("%s: %v", uri, err)
		}
		if err := render.ValidatePage(decl.Tree); err != nil {
			t.Errorf("%s: its tree is off the page grammar: %v", uri, err)
		}
		raw, _ := json.Marshal(decl.Tree)
		if !bytes.Contains(raw, []byte(`"id":"`+node+`"`)) {
			t.Errorf("%s: its tree has no node %q: %s", uri, node, raw)
		}
	}
	// A copy each time: a producer changing one never changes the type.
	a, _ := GenericTree(TypeProgress)
	a["id"] = "changed"
	if b, _ := GenericTree(TypeProgress); b["id"] != "progress" {
		t.Fatalf("GenericTree hands out the type's own tree")
	}
}

func openOf(t *testing.T, got []sent) *Open {
	t.Helper()
	if len(got) == 0 || got[0].event.Kind != KindOpen || got[0].event.Open == nil {
		t.Fatalf("no open went out: %+v", got)
	}
	return got[0].event.Open
}

// A panel opened with a generic type and no tree of its own carries the
// type's tree in its open event - every screen draws it with nothing to
// resolve -; an open with its own tree, and one of another type, go out as
// they were given. A machine's track does the same.
func TestAnOpenOfAGenericTypeCarriesItsTree(t *testing.T) {
	ctx := context.Background()
	pub, got := recorder(t)
	if _, err := OpenPanel(ctx, scope, pub, Open{Type: TypeProgress, Title: "Installer les programmes"}); err != nil {
		t.Fatal(err)
	}
	o := openOf(t, got())
	if o.Answer.Tree == nil {
		t.Fatalf("the open of a progress panel carries no tree")
	}
	raw, _ := json.Marshal(o.Answer)
	want, _ := GenericTree(TypeProgress)
	wantRaw, _ := json.Marshal(render.Answer{Tree: specPtr(want)})
	if !bytes.Equal(raw, wantRaw) {
		t.Fatalf("open answer %s, want the type's tree %s", raw, wantRaw)
	}

	own := render.InlineSpec(render.Node{"kind": "text", "id": "mine", "content": "x"})
	pub2, got2 := recorder(t)
	if _, err := OpenPanel(ctx, scope, pub2, Open{Type: TypeProgress, Answer: render.Answer{Tree: &own}}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(openOf(t, got2()).Answer); !bytes.Contains(raw, []byte(`"mine"`)) {
		t.Fatalf("an open with its own tree lost it: %s", raw)
	}
	pub3, got3 := recorder(t)
	if _, err := OpenPanel(ctx, scope, pub3, Open{Type: "ktw-example:/panel-type/confluence-publish"}); err != nil {
		t.Fatal(err)
	}
	if openOf(t, got3()).Answer.Tree != nil {
		t.Fatalf("an open of a pack's type was given a tree")
	}

	tr := NewTrack("")
	given := &Open{Type: TypeLog}
	e, err := tr.Next(Event{Kind: KindOpen, Open: given})
	if err != nil {
		t.Fatal(err)
	}
	if e.Open.Answer.Tree == nil || given.Answer.Tree != nil {
		t.Fatalf("a track's open of a log panel: tree %v, the caller's open changed %v", e.Open.Answer.Tree, given.Answer.Tree)
	}
}

func specPtr(n render.Node) *render.Spec { s := render.InlineSpec(n); return &s }

// A task shows its steps and its lines through the handle: a merge of the
// steps node's stages, an append to the log node - ops on the nodes the
// generic types name.
func TestAHandleShowsStepsAndLogLines(t *testing.T) {
	ctx := context.Background()
	pub, got := recorder(t)
	h, err := OpenPanel(ctx, scope, pub, Open{Type: TypeProgress, Title: "Publication"})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Steps(ctx, Step{ID: "p1", Label: "Vue d'ensemble", State: StepDone}, Step{ID: "p2", Label: "Contexte", State: StepRunning}); err != nil {
		t.Fatal(err)
	}
	if err := h.Log(ctx, "page 1 publiée", "page 2 en cours"); err != nil {
		t.Fatal(err)
	}
	if err := h.Log(ctx); err != nil {
		t.Fatal(err)
	}
	evs := got()
	if len(evs) != 3 {
		t.Fatalf("%d events, want open, steps and log", len(evs))
	}
	steps := evs[1].event.Ops[0]
	if steps.Op != render.OpMerge || steps.Target != StepsNode {
		t.Fatalf("steps op %+v", steps)
	}
	raw, _ := json.Marshal(steps.Value)
	if !bytes.Contains(raw, []byte(`"stages":[{"id":"p1","label":"Vue d'ensemble","state":"done"},{"id":"p2","label":"Contexte","state":"running"}]`)) {
		t.Fatalf("steps value %s", raw)
	}
	lines := evs[2].event.Ops[0]
	if lines.Op != render.OpAppend || lines.Target != LogNode || len(lines.Items) != 2 || lines.Items[1] != "page 2 en cours" {
		t.Fatalf("log op %+v", lines)
	}
}

// fakeInput is the bus's subscribe for one subject: deliver sends a payload
// to whoever subscribed it.
type fakeInput struct {
	subject string
	deliver func([]byte)
	ready   chan struct{}
}

func (f *fakeInput) subscribe(_ context.Context, subject string, deliver func([]byte)) (func(), error) {
	f.subject, f.deliver = subject, deliver
	close(f.ready)
	return func() {}, nil
}

// A question goes to the person and its answer back to the task: the input
// is subscribed before the ask goes out, the first answer of this panel
// taken - its state open again -, a cancel answers ErrCancelled, an input of
// another panel is not the answer.
func TestAQuestionsAnswerGoesBackToTheTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, tc := range []struct {
		name  string
		input Input
		err   error
	}{
		{"action", Input{Kind: InputAction, Name: "overwrite"}, nil},
		{"submit", Input{Kind: InputSubmit, Value: map[string]any{"overwrite": true}}, nil},
		{"cancel", Input{Kind: InputCancel}, ErrCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pub, got := recorder(t)
			h, err := OpenPanel(ctx, scope, pub, Open{Type: TypeQuestion, Title: "Publication"})
			if err != nil {
				t.Fatal(err)
			}
			f := &fakeInput{ready: make(chan struct{})}
			done := make(chan struct{})
			var answer Input
			var qerr error
			go func() {
				answer, qerr = h.Question(ctx, Ask{Question: "Écraser ?", Actions: []AskAction{{Name: "overwrite"}, {Name: "keep"}}}, f.subscribe)
				close(done)
			}()
			<-f.ready
			if f.subject != h.InputSubject() {
				t.Fatalf("subscribed %q, want %q", f.subject, h.InputSubject())
			}
			for h.State() != StateAsking {
				time.Sleep(time.Millisecond)
			}
			other, _ := json.Marshal(Input{Panel: NewID(), Kind: InputAction, Name: "keep"})
			f.deliver(other)
			in := tc.input
			in.Panel = h.ID()
			payload, _ := json.Marshal(in)
			f.deliver(payload)
			<-done
			if !errors.Is(qerr, tc.err) || answer.Kind != tc.input.Kind || answer.Panel != h.ID() {
				t.Fatalf("answer %+v err %v, want %s and %v", answer, qerr, tc.input.Kind, tc.err)
			}
			if h.State() != StateOpen {
				t.Fatalf("after the answer the panel is %s", h.State())
			}
			if evs := got(); len(evs) != 2 || evs[1].event.Kind != KindAsk {
				t.Fatalf("events %+v, want the open and the ask", evs)
			}
		})
	}
	if _, err := (&Handle{id: NewID()}).Question(ctx, Ask{Question: "x"}, nil); err == nil {
		t.Fatal("a question with no subscribe was put")
	}
}

// The display library draws the generic types from the trees the opens
// carry: its tests read them from a fixture this test holds equal to Go's
// (KTW_FIXTURE_UPDATE=1 writes it).
func TestTheDisplayLibrarysFixtureHoldsTheGenericTrees(t *testing.T) {
	trees := map[string]any{}
	for _, uri := range GenericTypeURIs() {
		tree, _ := GenericTree(uri)
		trees[uri] = tree
	}
	want, err := json.MarshalIndent(trees, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	path := filepath.Join("..", "..", "..", "app", "ktw-display-lib", "tests", "fixtures", "generic-panel-types.json")
	if os.Getenv("KTW_FIXTURE_UPDATE") == "1" {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the display library's fixture: %v (KTW_FIXTURE_UPDATE=1 writes it)", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
		t.Fatalf("%s is stale: KTW_FIXTURE_UPDATE=1 go test ./internal/core/panel -run TestTheDisplayLibrarysFixtureHoldsTheGenericTrees", path)
	}
}
