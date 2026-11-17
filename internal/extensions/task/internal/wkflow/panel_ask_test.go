// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	coreschemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// The workflow asks through its question panel: an ask naming
// a data type, its answer validated against it.

const ownerType = "ktw-x:/data-type/owner"

// ownerSchemas is a SchemaService knowing one data type, ownerType: an
// object whose name is a non-empty string.
type ownerSchemas struct {
	schemaapi.SchemaService
	checked int
}

func (s *ownerSchemas) ValidateAgainst(_ context.Context, in schemaapi.ValidateAgainstInput) (coreschemaapi.ValidateOutput, error) {
	s.checked++
	if in.URI != ownerType {
		return coreschemaapi.ValidateOutput{}, schemaapi.ErrDataTypeNotFound
	}
	var v map[string]any
	_ = json.Unmarshal(in.Body, &v)
	if name, _ := v["name"].(string); name != "" {
		return coreschemaapi.ValidateOutput{Valid: true}, nil
	}
	return coreschemaapi.ValidateOutput{Diagnostics: []coreschemaapi.Diagnostic{{Severity: "error", Path: "name", Message: "required"}}}, nil
}

// withSchemas puts svc behind the ask's schema seam (nil: no SchemaService).
func withSchemas(t *testing.T, svc schemaapi.SchemaService) {
	t.Helper()
	prev := resolveSchemaService
	resolveSchemaService = func(context.Context) (schemaapi.SchemaService, error) {
		if svc == nil {
			return nil, errors.New("no service")
		}
		return svc, nil
	}
	t.Cleanup(func() { resolveSchemaService = prev })
}

// askSteps asks the owner on a panel not opened, then returns the results.
func askSteps(onError any) []any {
	ask := ownerAsk()
	if onError != nil && onError != "" {
		ask["on_error"] = onError
	}
	return []any{ask, map[string]any{"kind": "return", "value": "${results}"}}
}

// ownerAsk is the ask of the owner by its data type.
func ownerAsk() map[string]any {
	return map[string]any{"kind": "panel", "name": "owner", "ask": map[string]any{"question": "Who owns it?", "data_type": ownerType}}
}

func askMachine(t *testing.T, ctx context.Context, steps []any) engine.Machine {
	t.Helper()
	m, err := (&workflowFactory{}).CreateMachine(ctx, &taskapi.TaskExecRequest{Config: map[string]any{"steps": steps}})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

// waitEvents waits until n panel events went out.
func (r *panelRig) waitEvents(t *testing.T, n int) []panel.Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if evs := r.events(); len(evs) >= n {
			return evs
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%d panel events, want %d: %+v", len(r.events()), n, r.events())
	return nil
}

func send(bus *engine.MemoryEventBus, in panel.Input) {
	raw, _ := json.Marshal(in)
	bus.Publish(engine.ExternalEventTopic(engine.PanelInputURI(in.Panel)), json.RawMessage(raw))
}

func submit(bus *engine.MemoryEventBus, id string, value any) {
	send(bus, panel.Input{Panel: id, Kind: panel.InputSubmit, Value: value, By: "jdoe"})
}

// runAsk runs steps on a fresh executor and answers its result channel.
func runAsk(t *testing.T, ctx context.Context, channel *engine.Channel, steps []any, rig *panelRig) <-chan *engine.RunResult {
	t.Helper()
	exec := engine.NewExecutor(askMachine(t, ctx, steps), channel, nil)
	exec.SetPanelPublisher(rig.publisher)
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	t.Cleanup(cancel)
	done := make(chan *engine.RunResult, 1)
	go func() {
		res, err := exec.Run(runCtx, engine.Event{Type: "start"})
		if err != nil {
			t.Errorf("Run: %v", err)
		}
		done <- res
	}()
	return done
}

func ownerOf(t *testing.T, res *engine.RunResult) map[string]any {
	t.Helper()
	if res == nil {
		t.Fatal("no result")
	}
	out, _ := res.Output.(map[string]any)
	got, _ := out["owner"].(map[string]any)
	return got
}

// AC-024: an ask naming a data type on a panel not opened opens a question
// panel, asks by the data type's form, and once answered completes it; the
// answer lands in results.<name>.
func TestAWorkflowAsksThroughItsPanelAndResumesWithTheAnswer(t *testing.T) {
	withSchemas(t, &ownerSchemas{})
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	rig := &panelRig{}
	done := runAsk(t, withLocalSpawner(channel), channel, askSteps(""), rig)
	id := rig.waitAsk(t)
	submit(bus, id, map[string]any{"name": "Ada"})
	res := <-done
	evs := rig.events()
	if len(evs) != 3 {
		t.Fatalf("%d panel events, want open, ask, complete: %+v", len(evs), evs)
	}
	if evs[0].Kind != panel.KindOpen || evs[0].Open.Type != panel.TypeQuestion || evs[0].Seq != 1 {
		t.Fatalf("the first event is no question panel's open: %+v", evs[0])
	}
	form, _ := json.Marshal(evs[1].Ask.Form)
	if evs[1].Kind != panel.KindAsk || !strings.Contains(string(form), `"kind":"form"`) || !strings.Contains(string(form), `"schema":"`+ownerType+`"`) {
		t.Fatalf("the ask does not ask by the data type's form: %+v %s", evs[1], form)
	}
	if evs[2].Kind != panel.KindComplete || evs[2].Complete.Status != "ok" || evs[2].Panel != id {
		t.Fatalf("the question panel is not completed ok: %+v", evs[2])
	}
	got := ownerOf(t, res)
	value, _ := got["value"].(map[string]any)
	if got["kind"] != "submit" || value["name"] != "Ada" {
		t.Fatalf("results.owner = %#v", got)
	}
}

// noModel is a spawner that fails the test if the workflow starts anything
// - an agent, a model call.
type noModel struct {
	engine.Spawner
	t *testing.T
}

func (s noModel) Spawn(context.Context, engine.ChildSpec) (engine.ChildHandle, error) {
	s.t.Error("the ask started a child: a model was called")
	return engine.ChildHandle{}, errors.New("no model")
}

// AC-025: asking and answering calls no model.
func TestAWorkflowAskCallsNoModel(t *testing.T) {
	withSchemas(t, &ownerSchemas{})
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := engine.WithSpawner(engine.WithChannel(context.Background(), channel), noModel{t: t})
	rig := &panelRig{}
	done := runAsk(t, ctx, channel, askSteps(""), rig)
	submit(bus, rig.waitAsk(t), map[string]any{"name": "Ada"})
	if got := ownerOf(t, <-done); got["kind"] != "submit" {
		t.Fatalf("results.owner = %#v", got)
	}
}

// AC-026: an answer that does not fit its data type is refused with the
// reason in place of the question's context and asked again; one that fits
// then resumes the run. No SchemaService: the node fails, never accepting an
// answer unchecked.
func TestAnAnswerThatDoesNotFitLeavesTheAskOpen(t *testing.T) {
	t.Run("refused then taken", func(t *testing.T) {
		schemas := &ownerSchemas{}
		withSchemas(t, schemas)
		bus := engine.NewMemoryEventBus()
		channel := engine.NewChannel(bus)
		rig := &panelRig{}
		done := runAsk(t, withLocalSpawner(channel), channel, askSteps(""), rig)
		id := rig.waitAsk(t)
		submit(bus, id, map[string]any{"name": ""})
		evs := rig.waitEvents(t, 4)
		if evs[2].Kind != panel.KindOps || evs[2].Ops[0].Target != panel.QuestionNode {
			t.Fatalf("the refusal does not replace the question's context: %+v", evs[2])
		}
		reason, _ := json.Marshal(evs[2].Ops[0].Node)
		if !strings.Contains(string(reason), "name: required") {
			t.Fatalf("the refusal does not say why: %s", reason)
		}
		if evs[3].Kind != panel.KindAsk || evs[3].Seq != 4 {
			t.Fatalf("the ask is not put again: %+v", evs[3])
		}
		select {
		case res := <-done:
			t.Fatalf("the run went on with an answer that does not fit: %#v", res.Output)
		case <-time.After(100 * time.Millisecond):
		}
		submit(bus, id, map[string]any{"name": "Ada"})
		got := ownerOf(t, <-done)
		if value, _ := got["value"].(map[string]any); value["name"] != "Ada" {
			t.Fatalf("results.owner = %#v", got)
		}
		if schemas.checked != 2 {
			t.Fatalf("%d answers checked, want 2", schemas.checked)
		}
	})
	t.Run("no schema service", func(t *testing.T) {
		withSchemas(t, nil)
		bus := engine.NewMemoryEventBus()
		channel := engine.NewChannel(bus)
		rig := &panelRig{}
		done := runAsk(t, withLocalSpawner(channel), channel, askSteps(""), rig)
		submit(bus, rig.waitAsk(t), map[string]any{"name": "Ada"})
		res := <-done
		if res == nil || res.Failure == nil || !strings.Contains(res.Failure.Reason, "no SchemaService") {
			t.Fatalf("an unchecked answer was taken: %#v", res)
		}
	})
}

// AC-027: a cancel fails the node to its on_error - the run fails, saying
// cancelled, with no on_error; it goes on with on_error: continue; it asks
// again on a new question panel with on_error: retry - and the question
// panel it opened ends cancelled.
func TestACancelledAskFailsItsNode(t *testing.T) {
	t.Run("on_error=retry", cancelThenRetry)
	for _, onError := range []string{"", "continue"} {
		t.Run("on_error="+onError, func(t *testing.T) {
			withSchemas(t, &ownerSchemas{})
			bus := engine.NewMemoryEventBus()
			channel := engine.NewChannel(bus)
			rig := &panelRig{}
			done := runAsk(t, withLocalSpawner(channel), channel, askSteps(onError), rig)
			id := rig.waitAsk(t)
			send(bus, panel.Input{Panel: id, Kind: panel.InputCancel, By: "jdoe"})
			res := <-done
			if res == nil {
				t.Fatal("no result")
			}
			evs := rig.events()
			last := evs[len(evs)-1]
			if last.Kind != panel.KindComplete || last.Complete.Status != "cancelled" {
				t.Fatalf("the question panel is not ended cancelled: %+v", last)
			}
			if onError == "" {
				if res.Failure == nil || res.Failure.Reason != "panel owner: cancelled" {
					t.Fatalf("the run does not fail cancelled: %#v", res)
				}
				return
			}
			if res.Failure != nil {
				t.Fatalf("on_error: continue did not go on: %#v", res.Failure)
			}
			if out, _ := res.Output.(map[string]any); out["owner"] != nil {
				t.Fatalf("a cancel was kept as an answer: %#v", out)
			}
		})
	}
}

// cancelThenRetry: with on_error: retry, a cancel ends the question panel
// cancelled and the retry asks again on a new question panel - an answer
// there resumes the run, a second cancel fails it, saying cancelled.
func cancelThenRetry(t *testing.T) {
	for _, then := range []string{"submit", "cancel"} {
		t.Run("then "+then, func(t *testing.T) {
			withSchemas(t, &ownerSchemas{})
			bus := engine.NewMemoryEventBus()
			channel := engine.NewChannel(bus)
			rig := &panelRig{}
			done := runAsk(t, withLocalSpawner(channel), channel, askSteps(map[string]any{"strategy": "retry", "max_retries": 1}), rig)
			first := rig.waitAsk(t)
			send(bus, panel.Input{Panel: first, Kind: panel.InputCancel, By: "jdoe"})
			evs := rig.waitEvents(t, 5)
			if evs[2].Kind != panel.KindComplete || evs[2].Complete.Status != "cancelled" || evs[2].Panel != first {
				t.Fatalf("the first question panel is not ended cancelled: %+v", evs[2])
			}
			again := evs[3].Panel
			if evs[3].Kind != panel.KindOpen || evs[3].Open.Type != panel.TypeQuestion || again == first || evs[3].Seq != 1 {
				t.Fatalf("the retry opens no new question panel: %+v", evs[3])
			}
			if evs[4].Kind != panel.KindAsk || evs[4].Panel != again {
				t.Fatalf("the retry does not ask on its new panel: %+v", evs[4])
			}
			if then == "submit" {
				submit(bus, again, map[string]any{"name": "Ada"})
				res := <-done
				if res == nil || res.Failure != nil {
					t.Fatalf("the answer after the retry did not resume the run: %#v", res)
				}
				if value, _ := ownerOf(t, res)["value"].(map[string]any); value["name"] != "Ada" {
					t.Fatalf("results.owner = %#v", res.Output)
				}
				return
			}
			send(bus, panel.Input{Panel: again, Kind: panel.InputCancel, By: "jdoe"})
			res := <-done
			if res == nil || res.Failure == nil || !strings.HasPrefix(res.Failure.Reason, "panel owner: cancelled") {
				t.Fatalf("a second cancel does not fail the run cancelled: %#v", res)
			}
			evs = rig.events()
			if last := evs[len(evs)-1]; last.Kind != panel.KindComplete || last.Complete.Status != "cancelled" || last.Panel != again {
				t.Fatalf("the second question panel is not ended cancelled: %+v", last)
			}
		})
	}
}

// A loop asking again after an answer asks on a new question panel each
// time; the last answer is kept.
func TestAnAskInALoopOpensANewQuestionPanelEachTime(t *testing.T) {
	withSchemas(t, &ownerSchemas{})
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	rig := &panelRig{}
	steps := []any{
		map[string]any{"kind": "loop", "times": 2, "body": ownerAsk()},
		map[string]any{"kind": "return", "value": "${results}"},
	}
	done := runAsk(t, withLocalSpawner(channel), channel, steps, rig)
	first := rig.waitAsk(t)
	submit(bus, first, map[string]any{"name": "Ada"})
	evs := rig.waitEvents(t, 5)
	if evs[2].Kind != panel.KindComplete || evs[2].Complete.Status != "ok" {
		t.Fatalf("the first question panel is not completed ok: %+v", evs[2])
	}
	again := evs[3].Panel
	if evs[3].Kind != panel.KindOpen || again == first || evs[4].Kind != panel.KindAsk || evs[4].Panel != again {
		t.Fatalf("the second turn does not ask on a new question panel: %+v", evs[3:])
	}
	submit(bus, again, map[string]any{"name": "Bob"})
	res := <-done
	if res == nil || res.Failure != nil {
		t.Fatalf("the loop did not end: %#v", res)
	}
	if value, _ := ownerOf(t, res)["value"].(map[string]any); value["name"] != "Bob" {
		t.Fatalf("results.owner = %#v", res.Output)
	}
}

// A self-opened question panel is not left open when the node fails for
// another reason than a cancel: no SchemaService ends it failed.
func TestAFailedAskEndsItsQuestionPanel(t *testing.T) {
	withSchemas(t, nil)
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	rig := &panelRig{}
	done := runAsk(t, withLocalSpawner(channel), channel, askSteps(""), rig)
	id := rig.waitAsk(t)
	submit(bus, id, map[string]any{"name": "Ada"})
	if res := <-done; res == nil || res.Failure == nil {
		t.Fatalf("the run did not fail: %#v", res)
	}
	evs := rig.events()
	if last := evs[len(evs)-1]; last.Kind != panel.KindComplete || last.Complete.Status != "failed" || last.Panel != id {
		t.Fatalf("the question panel is left open: %+v", last)
	}
}

// A cancel on an ask put on a panel an earlier open step opened ends that
// panel cancelled and the run with it - no panel is left asking after the run
// ; with on_error: retry the panel stays open and the fresh ask
// goes on the same panel.
func TestACancelledAskOnAnOpenedPanelEndsThatPanel(t *testing.T) {
	steps := func(onError any) []any {
		ask := map[string]any{"kind": "panel", "name": "pub", "ask": map[string]any{"question": "Overwrite?", "actions": []any{map[string]any{"name": "yes"}, map[string]any{"name": "no"}}}}
		if onError != nil {
			ask["on_error"] = onError
		}
		return []any{
			map[string]any{"kind": "panel", "name": "pub", "open": map[string]any{"type": "ktw-x:/panel-type/publish", "title": "Publication"}},
			ask,
			map[string]any{"kind": "return", "value": "${results}"},
		}
	}
	t.Run("no on_error", func(t *testing.T) {
		bus := engine.NewMemoryEventBus()
		channel := engine.NewChannel(bus)
		rig := &panelRig{}
		done := runAsk(t, withLocalSpawner(channel), channel, steps(nil), rig)
		id := rig.waitAsk(t)
		send(bus, panel.Input{Panel: id, Kind: panel.InputCancel, By: "jdoe"})
		res := <-done
		if res == nil || res.Failure == nil || res.Failure.Reason != "panel pub: cancelled" {
			t.Fatalf("the run does not end cancelled: %#v", res)
		}
		evs := rig.events()
		if last := evs[len(evs)-1]; last.Kind != panel.KindComplete || last.Complete.Status != "cancelled" || last.Panel != id {
			t.Fatalf("the panel is left asking after the run: %+v", evs)
		}
	})
	t.Run("on_error=retry", func(t *testing.T) {
		bus := engine.NewMemoryEventBus()
		channel := engine.NewChannel(bus)
		rig := &panelRig{}
		done := runAsk(t, withLocalSpawner(channel), channel, steps(map[string]any{"strategy": "retry", "max_retries": 1}), rig)
		id := rig.waitAsk(t)
		send(bus, panel.Input{Panel: id, Kind: panel.InputCancel, By: "jdoe"})
		evs := rig.waitEvents(t, 3)
		if evs[2].Kind != panel.KindAsk || evs[2].Panel != id || evs[2].Seq != 3 {
			t.Fatalf("the retry does not ask again on the same panel: %+v", evs)
		}
		answer(bus, id)
		res := <-done
		if res == nil || res.Failure != nil {
			t.Fatalf("the answer after the retry did not resume the run: %#v", res)
		}
		for _, e := range rig.events() {
			if e.Kind == panel.KindComplete {
				t.Fatalf("a retry ended the panel it asks again on: %+v", e)
			}
		}
		out, _ := res.Output.(map[string]any)
		if got, _ := out["pub"].(map[string]any); got["name"] != "yes" {
			t.Fatalf("results.pub = %#v", out)
		}
	})
}

// AC-028: stopped while asking, the run resumed by a new executor from its
// checkpoint waits on the same question; the answer then validated ends the
// ask, its numbering going on.
func TestAWorkflowAskSurvivesARestart(t *testing.T) {
	withSchemas(t, &ownerSchemas{})
	store := engine.NewMemoryStateStore()
	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	ctx := withLocalSpawner(channel)
	rig := &panelRig{}
	first := engine.NewExecutor(askMachine(t, ctx, askSteps("")), channel, store)
	first.SetPanelPublisher(rig.publisher)
	first.SetRunID("run-ask")
	runCtx, stop := context.WithCancel(ctx)
	stopped := make(chan struct{})
	go func() { _, _ = first.Run(runCtx, engine.Event{Type: "start"}); close(stopped) }()
	id := rig.waitAsk(t)
	stop() // the process dies while the question is asked
	<-stopped

	bus2 := engine.NewMemoryEventBus()
	channel2 := engine.NewChannel(bus2)
	ctx2 := withLocalSpawner(channel2)
	second := engine.NewExecutor(askMachine(t, ctx2, askSteps("")), channel2, store)
	second.SetPanelPublisher(rig.publisher)
	resCtx, cancel := context.WithTimeout(ctx2, 10*time.Second)
	defer cancel()
	done := make(chan *engine.RunResult, 1)
	go func() {
		res, err := second.ResumeWaiting(resCtx, "run-ask")
		if err != nil {
			t.Errorf("ResumeWaiting: %v", err)
		}
		done <- res
	}()
	time.Sleep(50 * time.Millisecond)
	submit(bus2, id, map[string]any{"name": ""})
	evs := rig.waitEvents(t, 4)
	if evs[3].Kind != panel.KindAsk || evs[3].Seq != 4 || evs[3].Panel != id {
		t.Fatalf("after the restart the answer was not checked against its data type: %+v", evs[2:])
	}
	submit(bus2, id, map[string]any{"name": "Ada"})
	res := <-done
	evs = rig.events()
	last := evs[len(evs)-1]
	if last.Kind != panel.KindComplete || last.Seq != 5 || last.Panel != id {
		t.Fatalf("after the restart: %+v", last)
	}
	if value, _ := ownerOf(t, res)["value"].(map[string]any); value["name"] != "Ada" {
		t.Fatalf("results.owner = %#v", res.Output)
	}
}
