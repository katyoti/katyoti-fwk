// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"path/filepath"
	"testing"
)

// : a member repinned to a new version restarts on it when it
// runs, through its universe's hub; a member that does not run, or a universe
// whose hub does not run, is left to start on the new pin.
func TestARepinnedMemberThatRunsRestartsOnItsNewVersion(t *testing.T) {
	saved := repinSettle
	repinSettle = 0
	t.Cleanup(func() { repinSettle = saved })
	base := t.TempDir()
	running, idle, stopped := filepath.Join(base, "running"), filepath.Join(base, "idle"), filepath.Join(base, "stopped")
	w := &spyWorld{universes: map[string]map[string]spyMember{
		running: {"ktw-methods": {running: true}, "ktw-sync": {running: true}},
		idle:    {"ktw-methods": {running: false}},
	}}
	s := &service{}
	w.install(s)
	s.restartRepinned(context.Background(), map[string][]string{
		running: {"ktw-methods"},
		idle:    {"ktw-methods"},
		stopped: {"ktw-methods"},
	})
	if len(w.restarted) != 1 || w.restarted[0] != "running/ktw-methods" {
		t.Errorf("restarted %v; want only running/ktw-methods (ktw-sync was not repinned)", w.restarted)
	}
	if len(w.started) != 0 {
		t.Errorf("started %v; a member that does not run is never started", w.started)
	}
}
