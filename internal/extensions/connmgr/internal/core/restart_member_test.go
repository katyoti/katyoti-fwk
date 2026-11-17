// SPDX-License-Identifier: Apache-2.0

package core

// Restart: a member that runs is stopped and started again —
// it reads its keys anew; one that does not run is left alone.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

// countedSpawns replaces the spawn with one that counts its calls and
// succeeds at once — no process runs.
func countedSpawns(t *testing.T) *atomic.Int32 {
	t.Helper()
	calls := &atomic.Int32{}
	prev := spawnAttempt
	spawnAttempt = func(context.Context, cmapi.SpawnSpec, *BusScope) (ktsp.Sender, *ktransport.Session, *exec.Cmd, error) {
		calls.Add(1)
		return ktsp.NewSender(nil), nil, nil, nil
	}
	t.Cleanup(func() { spawnAttempt = prev })
	return calls
}

func TestRestartMember_ARunningMemberIsStartedAgain(t *testing.T) {
	calls := countedSpawns(t)
	m, _ := NewManager([]cmapi.SpawnSpec{{Name: "a", Transport: "stdio", Binary: "a"}, {Name: "b", Transport: "stdio", Binary: "b"}})
	if _, err := m.Acquire(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Acquire(context.Background(), "a"); err != nil { // two holders
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("spawns before restart = %d, want 1", calls.Load())
	}
	restarted, err := m.Restart(context.Background(), "a")
	if err != nil || !restarted {
		t.Fatalf("Restart(a) = %v, %v; want true, nil", restarted, err)
	}
	if calls.Load() != 2 {
		t.Errorf("spawns after restart = %d, want 2 (stopped and started again)", calls.Load())
	}
	st, _ := m.Status("a")
	if st.State != cmapi.ConnReady || st.RefCount != 2 {
		t.Errorf("after restart: %s with %d holders, want READY with 2", st.State, st.RefCount)
	}
	if st, _ := m.Status("b"); st.State != cmapi.ConnDeclared {
		t.Errorf("b was never asked for and is %s", st.State)
	}
}

func TestRestartMember_AMemberNotRunningIsLeftAlone(t *testing.T) {
	calls := countedSpawns(t)
	m, _ := NewManager([]cmapi.SpawnSpec{{Name: "a", Transport: "stdio", Binary: "a"}})
	restarted, err := m.Restart(context.Background(), "a")
	if err != nil || restarted {
		t.Fatalf("Restart of a member not running = %v, %v; want false, nil", restarted, err)
	}
	if calls.Load() != 0 {
		t.Errorf("a member not running was started (%d spawns)", calls.Load())
	}
	if st, _ := m.Status("a"); st.State != cmapi.ConnDeclared {
		t.Errorf("state = %s, want DECLARED", st.State)
	}
	if _, err := m.Restart(context.Background(), "nobody"); !errors.Is(err, cmapi.ErrSidecarNotInUniverse) {
		t.Errorf("Restart of an unknown member: %v, want ErrSidecarNotInUniverse", err)
	}
}

// End to end: a member the hub started reads its .env; the key is saved
// with another value; Restart starts it again and it answers the new value
// — even though the hub's own environment holds the value it read when IT
// started (the hub's copy is not handed down: cenv.ChildEnviron).
func TestRestartMember_NATS_TheMemberAnswersTheNewKey(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	hub, err := natsembedded.StartEmbedded(natsembedded.EmbeddedOptions(filepath.Join(t.TempDir(), "nats"), true))
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	defer hub.Stop()
	bus := natsxport.New(hub.Client)
	binary := buildFakeSidecar(t)
	t.Setenv("KTW_NATS_URL", hub.Server.ClientURL())

	dotenv := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(dotenv, []byte("F025_KEY=first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KTW_FAKE_DOTENV", dotenv)
	// The hub read the universe's .env when it started: its environment
	// holds the key, listed as taken from a .env.
	t.Setenv("F025_KEY", "the-hub-copy")
	t.Setenv(cenv.DotenvKeysVar, "F025_KEY")

	m, err := NewManagerWithBus(
		[]cmapi.SpawnSpec{{Name: "fake", Transport: "nats", Binary: binary}},
		BusScope{Bus: bus, TenantID: "local", UniverseID: "test"},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	getKey := func() string {
		t.Helper()
		sender, err := m.Acquire(ctx, "fake")
		if err != nil {
			t.Fatalf("Acquire: %v", err)
		}
		defer m.Release("fake")
		out, err := sender.Call(ctx, "fake:/Demo/getenv", map[string]string{"name": "F025_KEY"})
		if err != nil {
			t.Fatalf("getenv: %v", err)
		}
		var v struct {
			Value string `json:"value"`
		}
		_ = json.Unmarshal(out, &v)
		return v.Value
	}
	if got := getKey(); got != "first" {
		t.Fatalf("the member answers %q, want first (its .env)", got)
	}
	firstPID, _ := pidFor(m, "fake")

	if err := os.WriteFile(dotenv, []byte("F025_KEY=second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	restarted, err := m.Restart(ctx, "fake")
	if err != nil || !restarted {
		t.Fatalf("Restart = %v, %v", restarted, err)
	}
	if pid, _ := pidFor(m, "fake"); pid == firstPID {
		t.Errorf("the member was not started again (same pid %d)", pid)
	}
	if got := getKey(); got != "second" {
		t.Errorf("after the restart the member answers %q, want second (the key saved)", got)
	}
}

// A running member whose spawn changes in the membership (its pinned version
// moved: another binary) is started again with it; one whose spawn is the
// same, or that does not run, is left alone.
func TestSetMembers_ARunningMemberWhoseBinaryChangedIsStartedAgain(t *testing.T) {
	calls := countedSpawns(t)
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "a", Transport: "stdio", Binary: "store/a/1.0/a"},
		{Name: "b", Transport: "stdio", Binary: "store/b/1.0/b"},
		{Name: "c", Transport: "stdio", Binary: "store/c/1.0/c"},
	})
	for _, n := range []string{"a", "b"} {
		if _, err := m.Acquire(context.Background(), n); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("spawns before = %d, want 2", calls.Load())
	}
	err := m.SetMembers([]cmapi.SpawnSpec{
		{Name: "a", Transport: "stdio", Binary: "store/a/1.1/a"}, // pin moved, running
		{Name: "b", Transport: "stdio", Binary: "store/b/1.0/b"}, // unchanged, running
		{Name: "c", Transport: "stdio", Binary: "store/c/1.1/c"}, // pin moved, not running
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("spawns after = %d, want 3 (a started again, b and c left alone)", calls.Load())
	}
	if st, _ := m.Status("a"); st.State != cmapi.ConnReady || st.RefCount != 1 {
		t.Errorf("a after its pin moved: %s with %d holders, want READY with 1", st.State, st.RefCount)
	}
	if st, _ := m.Status("c"); st.State != cmapi.ConnDeclared {
		t.Errorf("c, not running, is %s", st.State)
	}
}
