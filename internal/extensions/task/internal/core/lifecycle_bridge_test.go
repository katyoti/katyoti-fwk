// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/redact"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// sentBus records what is published.
type sentBus struct {
	mbapi.MessageBus
	mu   sync.Mutex
	sent map[string][][]byte
}

func (b *sentBus) Publish(_ context.Context, subject string, payload []byte, _ ...mbapi.PubOpt) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent[subject] = append(b.sent[subject], append([]byte(nil), payload...))
	return nil
}

func (b *sentBus) on(subject string) []engine.RunLifecycleEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []engine.RunLifecycleEvent
	for _, p := range b.sent[subject] {
		var e engine.RunLifecycleEvent
		if json.Unmarshal(p, &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

// fakeAWSKey is AWS's documentation example key, in two pieces so that no
// credential shape sits in the source; the joined value is the shape the
// redactor must catch.
const fakeAWSKey = "AKIA" + "IOSFODNN7EXAMPLE"

// secretFailer fails with a secret in its reason.
type secretFailer struct{}

func (secretFailer) Name() string                           { return "secret-failer" }
func (secretFailer) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (secretFailer) Step(context.Context, *engine.State, engine.Event) (engine.ActionFunc, error) {
	return func(context.Context) (*engine.StepResult, error) {
		return nil, errors.New("denied with key " + fakeAWSKey)
	}, nil
}

// A run's transitions are published on its universe's run-lifecycle subject —
// a GUI in another process refreshes on them — with secrets redacted.
func TestARunsTransitionsLeaveTheProcessRedacted(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	bus := &sentBus{sent: map[string][][]byte{}}
	subject := subjects.RunLifecycleSubject("local", "u1")
	svc.bridge.once.Do(func() {}) // resolved by hand: no application here
	svc.bridge.bus, svc.bridge.subject, svc.bridge.redactor = bus, subject, redact.New(redact.RedactionSettings{})
	useMachine(svc, secretFailer{})

	r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "secret-failer"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "its failure published", func() bool {
		for _, e := range bus.on(subject) {
			if e.RunID == r.RunID && e.Transition == engine.RunFailed {
				return true
			}
		}
		return false
	})
	for _, e := range bus.on(subject) {
		if strings.Contains(e.Reason, fakeAWSKey) {
			t.Fatalf("published %q; want the key redacted", e.Reason)
		}
	}
	var got []string
	for _, e := range bus.on(subject) {
		got = append(got, string(e.Transition))
	}
	if strings.Join(got, " ") != "started failed" {
		t.Errorf("published %v; want every transition of the run", got)
	}
}
