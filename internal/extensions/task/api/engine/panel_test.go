// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// panelMachine opens a panel in its first step and completes it in its
// second, keeping its panel.Track in its state.
type panelMachine struct{}

func (panelMachine) Name() string             { return "panel-test" }
func (panelMachine) InitialState(Event) State { return State{Name: "open"} }
func (panelMachine) Step(_ context.Context, st *State, _ Event) (ActionFunc, error) {
	return func(context.Context) (*StepResult, error) {
		tr, _ := st.Data.(panel.Track)
		if st.Name == "open" {
			tr = panel.NewTrack("")
			e, err := tr.Next(panel.Event{Kind: panel.KindOpen, Open: &panel.Open{Type: "t"}})
			if err != nil {
				return nil, err
			}
			return &StepResult{NextState: "done", Data: tr, Panels: []panel.Event{e}}, nil
		}
		e, err := tr.Next(panel.Event{Kind: panel.KindComplete, Complete: &panel.Complete{Status: "ok"}})
		if err != nil {
			return nil, err
		}
		return &StepResult{NextState: "end", Data: tr, Output: "ok", Panels: []panel.Event{e}}, nil
	}, nil
}

type orderStore struct {
	MemoryStateStore
	log *[]string
}

func (s *orderStore) Save(runID string, cp Checkpoint) error {
	*s.log = append(*s.log, "checkpoint")
	return nil
}
func (s *orderStore) Load(string) (Checkpoint, error) { return Checkpoint{}, nil }
func (s *orderStore) Delete(string) error             { return nil }

// A step's panel events go out after its checkpoint, in the order the steps
// return them, numbered by the machine's track.
func TestAStepsPanelEventsGoOutAfterItsCheckpoint(t *testing.T) {
	var log []string
	var sent []panel.Event
	ex := NewExecutor(panelMachine{}, nil, &orderStore{log: &log})
	ex.SetPanelPublisher(func(_ context.Context, evs []panel.Event) {
		for _, e := range evs {
			log = append(log, "panel "+string(e.Kind))
			sent = append(sent, e)
		}
	})
	if _, err := ex.Run(context.Background(), Event{Type: "start"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"checkpoint", "panel open", "checkpoint", "panel complete"}
	if len(log) != len(want) {
		t.Fatalf("order %v, want %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("order %v, want %v", log, want)
		}
	}
	if sent[0].Seq != 1 || sent[1].Seq != 2 || sent[0].Panel != sent[1].Panel {
		t.Fatalf("events %+v", sent)
	}
}

// The wait on a panel's input names the event URI whose subject is the
// panel's in subject - what the event bridge subscribes when the run waits.
func TestThePanelInputWaitNamesThePanelsInSubject(t *testing.T) {
	id := panel.NewID()
	c := PanelInputWait(id, "input")
	if c.Label != "input" || c.Topic != ExternalEventTopic(c.EventURI) {
		t.Fatalf("condition %+v", c)
	}
	u, err := mbapi.ParseEventURI(c.EventURI)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := u.Subject("t", "u", ""), subjects.PanelInSubject("t", "u", id); got != want {
		t.Fatalf("subject %q, want %q", got, want)
	}
}
