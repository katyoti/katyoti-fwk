// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/hubdial"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// servingHub is a running universe's hub whose member serving CWM binds its
// project once, when it starts - as ktw-agents does: it reads the universe's
// projects.yaml at start and at each restart, never in between.
type servingHub struct {
	home      string
	served    string // the project id the member serves
	restarted []string
	fail      error
}

func (h *servingHub) start() {
	h.served = ""
	if dirs := cenv.AttachedProjects(application.EnvOf(h.home).HomeDir()); len(dirs) == 1 {
		if f, err := projectfile.Read(dirs[0]); err == nil {
			h.served = f.UUID
		}
	}
}

func (h *servingHub) Running(context.Context) ([]string, error) { return []string{servingMember}, nil }

func (h *servingHub) Restart(_ context.Context, member string) (bool, error) {
	if h.fail != nil {
		return false, h.fail
	}
	h.restarted = append(h.restarted, member)
	if member == servingMember {
		h.start()
	}
	return true, nil
}

func (h *servingHub) Close() {}

const childPID = "00000000-0000-7000-8000-000000000002"

// SWITCHING TO A GROUP'S CHILD SERVES THE CHILD: the member
// serving the universe's objects reads its project at start, so the switch
// restarts it through the universe's hub - after it, the project id served is
// the child's; a restart that fails is said in the output, the switch kept.
func TestSwitchProjectRestartsTheServingMemberOntoTheChild(t *testing.T) {
	root := projectTestRoot(t)
	seedUniverseFor(t, root, "acme")
	home := filepath.Join(root, "universes", "acme")
	group, child := filepath.Join(root, "ktw-acme-demo"), filepath.Join(root, "erp-acme")
	writeProjectObject(t, group, "ktw-acme-demo", testPID)
	writeProjectObject(t, child, "erp-acme", childPID)

	ctx := context.Background()
	hub := &servingHub{home: home}
	s := &Service{dialHub: func(_ context.Context, h string) (hubdial.Conn, error) {
		if filepath.Clean(h) != filepath.Clean(home) {
			t.Fatalf("dialed the hub of %s; want %s", h, home)
		}
		return hub, nil
	}}
	if _, err := s.AttachProject(ctx, api.AttachProjectInput{Universe: "acme", ProjectDir: group}); err != nil {
		t.Fatal(err)
	}
	hub.start()
	if hub.served != testPID {
		t.Fatalf("before the switch the universe serves %q; want the group %s", hub.served, testPID)
	}

	out, err := s.SwitchProject(ctx, api.SwitchProjectInput{Universe: "acme", ProjectDir: child})
	if err != nil || !out.Changed {
		t.Fatalf("switch to the child: %+v %v", out, err)
	}
	if hub.served != childPID {
		t.Fatalf("after the switch the universe serves %q; want the child %s (restarts asked: %v)", hub.served, childPID, hub.restarted)
	}
	if !out.Restarted || out.RestartError != "" {
		t.Errorf("output = %+v; want restarted, no error", out)
	}

	// Already served: nothing changes, nothing restarts.
	hub.restarted = nil
	if out, err := s.SwitchProject(ctx, api.SwitchProjectInput{Universe: "acme", ProjectDir: child}); err != nil || out.Changed || out.Restarted || len(hub.restarted) != 0 {
		t.Errorf("switch to the project served: %+v %v, restarts %v; want nothing", out, err, hub.restarted)
	}

	// A restart that fails is said; the switch is kept.
	hub.fail = errors.New("member ktw-agents: stop timed out")
	out, err = s.SwitchProject(ctx, api.SwitchProjectInput{Universe: "acme", ProjectDir: group})
	if err != nil || !out.Changed || out.Restarted || !strings.Contains(out.RestartError, "stop timed out") {
		t.Fatalf("switch back with a failing restart: %+v %v; want changed, not restarted, the reason", out, err)
	}
	if got := s.loadProjects("acme").Projects; len(got) != 1 || got[0] != group {
		t.Errorf("after a failed restart the universe has %v selected; want the group still switched to", got)
	}

	// A universe whose hub does not run starts on the project: no restart, no error.
	s.dialHub = func(context.Context, string) (hubdial.Conn, error) { return nil, nil }
	if out, err := s.SwitchProject(ctx, api.SwitchProjectInput{Universe: "acme", ProjectDir: child}); err != nil || !out.Changed || out.Restarted || out.RestartError != "" {
		t.Errorf("switch in a universe not running: %+v %v; want changed, nothing to restart", out, err)
	}
}
