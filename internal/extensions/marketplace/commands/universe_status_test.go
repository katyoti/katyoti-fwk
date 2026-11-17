// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/oslock"
)

// EVERY HUB SERVING A HOME IS LISTED: a hub the universe's
// lockfile does not name — an orphan whose lockfile was overwritten or
// removed — still serves the home through the member it started. Status
// finds it through that member's serving lock and its parent process, and
// never says "stopped" while it serves. Here the test process holds the
// ktw-agents serving lock of a temporary universe with no lockfile: the
// process that started it (the test runner) is the unregistered hub.
func TestStatusListsAHubNoLockfileNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("KTW_DATA", t.TempDir())
	dataDir := application.EnvOf(home).DataDir()
	lockPath := filepath.Join(dataDir, "serving", "ktw-agents.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	l, err := oslock.TryLock(lockPath)
	if err != nil {
		t.Fatalf("take the serving lock: %v", err)
	}
	defer l.Unlock()
	if err := l.Note(fmt.Sprintf("pid %d", os.Getpid())); err != nil {
		t.Fatal(err)
	}
	hub := os.Getppid()

	st := statusOf(context.Background(), home)
	if st.Hub != nil {
		t.Fatalf("no lockfile, yet status names a registered hub: %+v", st.Hub)
	}
	others := st.unregistered()
	if len(others) != 1 || others[0].PID != hub || others[0].Registered || strings.Join(others[0].Serves, ",") != "ktw-agents" {
		t.Fatalf("hubs = %+v, want the unregistered hub pid %d serving ktw-agents", st.Hubs, hub)
	}

	var out bytes.Buffer
	st.print(&out)
	if strings.HasPrefix(out.String(), "stopped") || !strings.Contains(out.String(), fmt.Sprintf("unregistered hub: pid %d", hub)) {
		t.Fatalf("status printed:\n%s\nwant the unregistered hub pid %d, never stopped", out.String(), hub)
	}

	// --out json carries it.
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		Hubs []struct {
			PID        int      `json:"pid"`
			Registered bool     `json:"registered"`
			Serves     []string `json:"serves"`
		} `json:"hubs"`
	}
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Hubs) != 1 || back.Hubs[0].PID != hub || back.Hubs[0].Registered {
		t.Fatalf("json = %s, want hubs [{pid %d registered false}]", data, hub)
	}

	// Released, the member serves no more: status says stopped.
	_ = l.Unlock()
	if st := statusOf(context.Background(), home); len(st.Hubs) != 0 {
		t.Fatalf("after the member ended, hubs = %+v, want none", st.Hubs)
	}
}

// A serving lock's note names its holder as "pid N".
func TestNotePID(t *testing.T) {
	for note, want := range map[string]int{"pid 42": 42, " pid 7 ": 7, "": 0, "another process": 0} {
		if got := notePID(note); got != want {
			t.Errorf("notePID(%q) = %d, want %d", note, got, want)
		}
	}
}
