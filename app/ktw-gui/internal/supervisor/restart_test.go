// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// « Redémarrer l'univers »: the GUI's core is closed, the
// universe's hub is ended - also one the GUI found running, which closing the
// core alone leaves running - and the universe is activated again on a new
// hub.
func TestRestart_EndsTheUniversesHubThenActivatesItAgain(t *testing.T) {
	m := NewMaster()
	var steps []string
	starts := 0
	m.start = func(ctx context.Context, name string) (*ActiveCore, error) {
		starts++
		steps = append(steps, "start")
		// The first hub was found running; the one the restart starts is the GUI's.
		return &ActiveCore{Name: name, HomePath: "/u/" + name, HubFound: starts == 1, exited: make(chan struct{})}, nil
	}
	m.stopHub = func(ctx context.Context, home string) error {
		if home != "/u/work" {
			t.Errorf("stopHub(%q), want the universe's home /u/work", home)
		}
		if m.IsActive("work") {
			t.Error("the hub is stopped while the GUI still holds the universe's core")
		}
		steps = append(steps, "stop")
		return nil
	}
	first, err := m.Activate(context.Background(), "work")
	if err != nil {
		t.Fatal(err)
	}
	if got := m.ListActive(); len(got) != 1 || !got[0].HubKeepsRunning {
		t.Fatalf("ListActive = %+v, want work with hubKeepsRunning (its hub was found running)", got)
	}

	second, err := m.Restart(context.Background(), "work")
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if got := strings.Join(steps, ","); got != "start,stop,start" {
		t.Fatalf("steps = %s, want start,stop,start", got)
	}
	if second == first {
		t.Fatal("Restart answered the old core")
	}
	select {
	case <-first.exited:
	default:
		t.Fatal("the old core was not closed")
	}
	if got := m.ListActive(); len(got) != 1 || got[0].HubKeepsRunning {
		t.Fatalf("ListActive after the restart = %+v, want work on the GUI's own hub", got)
	}
}

// A hub that does not stop fails the restart, saying so; the universe is not
// activated again on the old hub.
func TestRestart_AHubThatDoesNotStopFailsIt(t *testing.T) {
	m := NewMaster()
	starts := 0
	m.start = func(ctx context.Context, name string) (*ActiveCore, error) {
		starts++
		return &ActiveCore{Name: name, HomePath: "/u/" + name, exited: make(chan struct{})}, nil
	}
	m.stopHub = func(ctx context.Context, home string) error { return errors.New("did not stop within 15s") }
	if _, err := m.Activate(context.Background(), "work"); err != nil {
		t.Fatal(err)
	}
	_, err := m.Restart(context.Background(), "work")
	if err == nil || !strings.Contains(err.Error(), "did not stop") {
		t.Fatalf("Restart err = %v, want the hub's failure to stop", err)
	}
	if starts != 1 || m.IsActive("work") {
		t.Fatalf("starts = %d, active = %v: want no activation after a failed stop", starts, m.IsActive("work"))
	}
}

// A universe this GUI does not run is simply activated: nothing to stop.
func TestRestart_AUniverseNotRunningIsActivated(t *testing.T) {
	m := NewMaster()
	m.start = func(ctx context.Context, name string) (*ActiveCore, error) {
		return &ActiveCore{Name: name, exited: make(chan struct{})}, nil
	}
	m.stopHub = func(ctx context.Context, home string) error {
		t.Error("stopHub called for a universe the GUI does not run")
		return nil
	}
	if _, err := m.Restart(context.Background(), "idle"); err != nil || !m.IsActive("idle") {
		t.Fatalf("Restart = %v, active = %v; want it activated", err, m.IsActive("idle"))
	}
}
