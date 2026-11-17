// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/hubdial"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// dialServing answers the fake hub of the universe at home, and fails the
// test when another universe's hub is dialed.
func dialServing(t *testing.T, home string, hub *servingHub) hubDialer {
	return func(_ context.Context, h string) (hubdial.Conn, error) {
		if filepath.Clean(h) != filepath.Clean(home) {
			t.Fatalf("dialed the hub of %s; want %s", h, home)
		}
		return hub, nil
	}
}

// ATTACHING OR DETACHING A PROJECT IN A RUNNING UNIVERSE SERVES THE CHANGE
// : the member serving the universe's objects binds its project
// when it starts, so an attach and a detach each restart it once through the
// universe's hub - after them the universe serves the project attached, then
// none. A switch, made of a detach and an attach, still restarts it exactly
// once; an attach of the project served restarts nothing; a universe whose hub
// does not run is not restarted, and nothing fails.
func TestAttachAndDetachRestartTheServingMemberOnce(t *testing.T) {
	root := projectTestRoot(t)
	seedUniverseFor(t, root, "acme")
	home := filepath.Join(root, "universes", "acme")
	group, child := filepath.Join(root, "ktw-acme-demo"), filepath.Join(root, "erp-acme")
	writeProjectObject(t, group, "ktw-acme-demo", testPID)
	writeProjectObject(t, child, "erp-acme", childPID)

	ctx := context.Background()
	hub := &servingHub{home: home}
	s := &Service{dialHub: dialServing(t, home, hub)}

	att, err := s.AttachProject(ctx, api.AttachProjectInput{Universe: "acme", ProjectDir: group})
	if err != nil {
		t.Fatal(err)
	}
	if len(hub.restarted) != 1 || hub.served != testPID || !att.Restarted || att.RestartError != "" {
		t.Fatalf("attach: %+v, restarts %v, serves %q; want one restart onto %s", att, hub.restarted, hub.served, testPID)
	}

	// The project served attached again: nothing changed, nothing restarts.
	hub.restarted = nil
	if again, err := s.AttachProject(ctx, api.AttachProjectInput{Universe: "acme", ProjectDir: group}); err != nil || again.Restarted || len(hub.restarted) != 0 {
		t.Errorf("attach of the project served: %+v %v, restarts %v; want none", again, err, hub.restarted)
	}

	// A switch restarts exactly once, not once per detach and attach.
	hub.restarted = nil
	if sw, err := s.SwitchProject(ctx, api.SwitchProjectInput{Universe: "acme", ProjectDir: child}); err != nil || !sw.Restarted {
		t.Fatalf("switch: %+v %v", sw, err)
	}
	if len(hub.restarted) != 1 || hub.served != childPID {
		t.Fatalf("switch: restarts %v, serves %q; want one restart onto %s", hub.restarted, hub.served, childPID)
	}

	// A detach restarts once; the universe serves no project after it.
	hub.restarted = nil
	det, err := s.DetachProject(ctx, api.DetachProjectInput{ProjectDir: child})
	if err != nil {
		t.Fatal(err)
	}
	if len(hub.restarted) != 1 || hub.served != "" || !det.Restarted || det.RestartError != "" {
		t.Fatalf("detach: %+v, restarts %v, serves %q; want one restart onto no project", det, hub.restarted, hub.served)
	}

	// A project attached to no universe: its detach restarts nothing.
	hub.restarted = nil
	if det, err := s.DetachProject(ctx, api.DetachProjectInput{ProjectDir: child}); err != nil || det.Restarted || len(hub.restarted) != 0 {
		t.Errorf("detach of a project attached nowhere: %+v %v, restarts %v; want none", det, err, hub.restarted)
	}

	// A restart that fails is said; the attach is kept.
	hub.fail = errors.New("member ktw-agents: stop timed out")
	att, err = s.AttachProject(ctx, api.AttachProjectInput{Universe: "acme", ProjectDir: group})
	if err != nil || att.Restarted || !strings.Contains(att.RestartError, "stop timed out") {
		t.Fatalf("attach with a failing restart: %+v %v; want not restarted, the reason", att, err)
	}
	if got := s.loadProjects("acme").Projects; len(got) != 1 || got[0] != group {
		t.Errorf("after a failed restart the universe has %v attached; want the group", got)
	}
	hub.fail = nil

	// A universe whose hub does not run: no restart, no error.
	s.dialHub = func(context.Context, string) (hubdial.Conn, error) { return nil, nil }
	if det, err := s.DetachProject(ctx, api.DetachProjectInput{ProjectDir: group}); err != nil || det.Restarted || det.RestartError != "" {
		t.Errorf("detach in a universe not running: %+v %v; want nothing restarted", det, err)
	}
	if att, err := s.AttachProject(ctx, api.AttachProjectInput{Universe: "acme", ProjectDir: group}); err != nil || att.Restarted || att.RestartError != "" {
		t.Errorf("attach in a universe not running: %+v %v; want nothing restarted", att, err)
	}
}

// CREATING A PROJECT IN A RUNNING UNIVERSE SERVES IT: the
// creation's attach restarts the serving member once and says so.
func TestCreateProjectRestartsTheServingMemberOnce(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	isolateDataDir(t)
	registry, _ := catalog.LoadRegistry(filepath.Join(home, "universes.yaml"))
	uhome := filepath.Join(home, "universes", "main")
	hub := &servingHub{home: uhome}
	m := &Service{registry: registry, dialHub: func(context.Context, string) (hubdial.Conn, error) { return nil, nil }}
	if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: "main", HomePath: uhome}); err != nil {
		t.Fatal(err)
	}
	m.dialHub = dialServing(t, uhome, hub)

	out, err := m.CreateProject(ctx, api.CreateProjectInput{Name: "archi", Universe: "main"})
	if err != nil || !out.Created || out.Universe != "main" {
		t.Fatalf("create archi: %+v %v", out, err)
	}
	if len(hub.restarted) != 1 || !out.Restarted || out.RestartError != "" {
		t.Errorf("create archi: %+v, restarts %v; want one restart, said", out, hub.restarted)
	}

	// Created only (no universe): nothing to serve, nothing restarted.
	hub.restarted = nil
	if out, err := m.CreateProject(ctx, api.CreateProjectInput{Name: "loose"}); err != nil || out.Restarted || len(hub.restarted) != 0 {
		t.Errorf("create loose: %+v %v, restarts %v; want none", out, err, hub.restarted)
	}
}
