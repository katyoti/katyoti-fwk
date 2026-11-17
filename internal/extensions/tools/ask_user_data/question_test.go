// SPDX-License-Identifier: Apache-2.0

package ask_user_data

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// typingGate is a form-capable gate whose free-text answer is what the test
// types (typed), or the detach of the screens (detach).
type typingGate struct {
	typed  chan string
	detach chan struct{}
}

func newTypingGate() *typingGate {
	return &typingGate{typed: make(chan string, 1), detach: make(chan struct{})}
}

func (g *typingGate) Prompt(ctx context.Context, _ hgapi.PromptRequest) (hgapi.PromptResponse, error) {
	select {
	case t := <-g.typed:
		return hgapi.PromptResponse{Answer: t}, nil
	case <-g.detach:
		return hgapi.PromptResponse{}, hgapi.ErrUserDetached
	case <-ctx.Done():
		return hgapi.PromptResponse{}, ctx.Err()
	}
}

func (g *typingGate) PromptPermission(context.Context, hgapi.PromptPermissionRequest) (hgapi.PromptPermissionResponse, error) {
	return hgapi.PromptPermissionResponse{}, nil
}

func (g *typingGate) Capabilities() []string { return []string{"text", "form"} }

var testScope = panel.Scope{Tenant: "t", Universe: "u"}

// asked starts the tool's GUI path and answers the id of the question panel
// it opened, once that panel asks, and the tool's result.
func asked(t *testing.T, bus *teststub.Stub, gate hgapi.HumanGate) (string, <-chan AskUserDataOutput, <-chan error) {
	t.Helper()
	asks := make(chan panel.Event, 4)
	sub, err := bus.Subscribe(context.Background(), "ktw.t.universe.u.panel.*.out", func(_ context.Context, m mbapi.Message) {
		if e, err := panel.DecodeEvent(m.Data()); err == nil && e.Kind == panel.KindAsk {
			asks <- e
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	out := make(chan AskUserDataOutput, 1)
	errs := make(chan error, 1)
	schema := map[string]any{"type": "object"}
	go func() {
		o, err := executeGUI(context.Background(), gate, wireOf(bus, testScope), AskUserDataInput{SchemaInline: schema, Title: "Dossier", Prompt: "Renseigne-le"}, schema)
		out <- o
		errs <- err
	}()
	select {
	case e := <-asks:
		if e.Ask == nil || e.Ask.Question != "Renseigne-le" || e.Ask.Form == nil {
			t.Fatalf("the panel asked %+v, want the prompt and its form", e.Ask)
		}
		return e.Panel, out, errs
	case <-time.After(2 * time.Second):
		t.Fatal("the tool opened no question panel that asks")
	}
	return "", nil, nil
}

// answerPanel sends the person's input to the panel's input subject, as a
// screen following the panel (or the room, for the chat card) does.
func answerPanel(t *testing.T, bus *teststub.Stub, in panel.Input) {
	t.Helper()
	payload, _ := json.Marshal(in)
	if err := bus.Publish(context.Background(), subjects.PanelInSubject(testScope.Tenant, testScope.Universe, in.Panel), payload); err != nil {
		t.Fatal(err)
	}
}

func result(t *testing.T, out <-chan AskUserDataOutput, errs <-chan error) AskUserDataOutput {
	t.Helper()
	select {
	case o := <-out:
		if err := <-errs; err != nil {
			t.Fatalf("ask-user-data: %v", err)
		}
		return o
	case <-time.After(2 * time.Second):
		t.Fatal("ask-user-data did not end")
	}
	return AskUserDataOutput{}
}

// A submit on the question panel's input returns the form's data, the
// artifact being the panel.
func TestAskUserDataSubmitOnThePanelInputReturnsTheData(t *testing.T) {
	bus := teststub.New()
	id, out, errs := asked(t, bus, newTypingGate())
	answerPanel(t, bus, panel.Input{Panel: id, Kind: panel.InputSubmit, Value: map[string]any{"name": "  Atlas  "}})
	o := result(t, out, errs)
	if o.Cancelled || o.ArtifactID != id || o.Data["name"] != "Atlas" {
		t.Fatalf("got %+v, want the submitted data (cleaned) on artifact %s", o, id)
	}
}

func TestAskUserDataCancelOnThePanelInputCancels(t *testing.T) {
	bus := teststub.New()
	id, out, errs := asked(t, bus, newTypingGate())
	answerPanel(t, bus, panel.Input{Panel: id, Kind: panel.InputCancel})
	if o := result(t, out, errs); !o.Cancelled || o.Data != nil {
		t.Fatalf("got %+v, want cancelled", o)
	}
}

// A text typed in the composer while the form asks cancels it with the text.
func TestAskUserDataTypedTextCancelsWithTheText(t *testing.T) {
	bus := teststub.New()
	gate := newTypingGate()
	_, out, errs := asked(t, bus, gate)
	gate.typed <- "laisse tomber"
	if o := result(t, out, errs); !o.Cancelled || o.TypedText != "laisse tomber" {
		t.Fatalf("got %+v, want cancelled with the typed text", o)
	}
}

// The screens detaching does not end the question: the panel keeps asking
// (the panel server holds it) and an answer coming later on its input is
// taken - no in-memory waiter of the gate holds the wait.
func TestAskUserDataWaitOutlivesTheScreensDetaching(t *testing.T) {
	bus := teststub.New()
	gate := newTypingGate()
	id, out, errs := asked(t, bus, gate)
	close(gate.detach)
	select {
	case o := <-out:
		t.Fatalf("the question ended when the screens detached: %+v %v", o, <-errs)
	case <-time.After(50 * time.Millisecond):
	}
	answerPanel(t, bus, panel.Input{Panel: id, Kind: panel.InputSubmit, Value: map[string]any{"name": "Atlas"}})
	if o := result(t, out, errs); o.Data["name"] != "Atlas" {
		t.Fatalf("got %+v, want the data answered after the detach", o)
	}
}
