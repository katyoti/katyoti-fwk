// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"testing"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	teststub "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
)

func sampleSpecs() []cmapi.SpawnSpec {
	return []cmapi.SpawnSpec{
		{Name: "ktw-docs", Transport: "stdio", Binary: "ktw-docs"},
		{Name: "ktw-example", Transport: "stdio", Binary: "ktw-example"},
	}
}

func TestNewManager_PopulatesDeclared(t *testing.T) {
	m, err := NewManager(sampleSpecs())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	all := m.All()
	if len(all) != 2 {
		t.Fatalf("All() = %d, want 2", len(all))
	}
	for _, s := range all {
		if s.State != cmapi.ConnDeclared {
			t.Errorf("%s: State = %s, want DECLARED", s.Name, s.State)
		}
		if s.RefCount != 0 {
			t.Errorf("%s: RefCount = %d, want 0", s.Name, s.RefCount)
		}
	}
	// Membership order preserved.
	if all[0].Name != "ktw-docs" || all[1].Name != "ktw-example" {
		t.Errorf("All() order = [%s %s], want [ktw-docs ktw-example]", all[0].Name, all[1].Name)
	}
}

func TestNewManager_RejectsEmpty(t *testing.T) {
	_, err := NewManager([]cmapi.SpawnSpec{{Name: ""}})
	if err == nil {
		t.Error("expected error for empty Name, got nil")
	}
}

func TestNewManager_RejectsDuplicates(t *testing.T) {
	_, err := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Binary: "x"},
		{Name: "x", Binary: "x"},
	})
	if err == nil {
		t.Error("expected error for duplicate Name, got nil")
	}
}

func TestStatus_Unknown(t *testing.T) {
	m, _ := NewManager(sampleSpecs())
	_, ok := m.Status("unknown")
	if ok {
		t.Error("Status(unknown) returned ok=true, want false")
	}
}

func TestAcquire_UnknownReturnsErrSidecarNotInUniverse(t *testing.T) {
	m, _ := NewManager(sampleSpecs())
	_, err := m.Acquire(context.Background(), "ghost")
	if !errors.Is(err, cmapi.ErrSidecarNotInUniverse) {
		t.Errorf("expected ErrSidecarNotInUniverse, got %v", err)
	}
}

func TestAcquire_RejectsUnsupportedTransport(t *testing.T) {
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "ws", Binary: "x"},
	})
	_, err := m.Acquire(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error for unsupported transport, got nil")
	}
	st, _ := m.Status("x")
	if st.State != cmapi.ConnFailed {
		t.Errorf("State = %s, want FAILED after spawn rejection", st.State)
	}
	if st.Error == "" {
		t.Error("Error should be populated on FAILED state")
	}
}

func TestAcquire_FailsOnMissingBinary(t *testing.T) {
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "stdio", Binary: "/path/that/does/not/exist/ktw-ghost"},
	})
	_, err := m.Acquire(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error for missing binary, got nil")
	}
	st, _ := m.Status("x")
	if st.State != cmapi.ConnFailed {
		t.Errorf("State = %s, want FAILED", st.State)
	}
}

func TestRelease_UnknownIsNoOp(t *testing.T) {
	m, _ := NewManager(sampleSpecs())
	m.Release("ghost") // must not panic
}

func TestRelease_DecrementsRefCount(t *testing.T) {
	m, _ := NewManager(sampleSpecs())
	// Force a synthetic READY state via reflection-free path: bump
	// refCount manually through the spawn-failure path won't help, so
	// we go directly to internals.
	c := m.conns["ktw-docs"]
	c.refCount = 3
	c.state = cmapi.ConnReady

	m.Release("ktw-docs")
	st, _ := m.Status("ktw-docs")
	if st.RefCount != 2 {
		t.Errorf("RefCount after Release = %d, want 2", st.RefCount)
	}
	m.Release("ktw-docs")
	m.Release("ktw-docs")
	m.Release("ktw-docs") // extra release: should not go negative
	st, _ = m.Status("ktw-docs")
	if st.RefCount != 0 {
		t.Errorf("RefCount after over-release = %d, want 0", st.RefCount)
	}
}

func TestClose_ResetsToDeclared(t *testing.T) {
	m, _ := NewManager(sampleSpecs())
	c := m.conns["ktw-docs"]
	c.state = cmapi.ConnReady
	c.refCount = 5

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	st, _ := m.Status("ktw-docs")
	if st.State != cmapi.ConnDeclared {
		t.Errorf("State after Close = %s, want DECLARED", st.State)
	}
	if st.RefCount != 0 {
		t.Errorf("RefCount after Close = %d, want 0", st.RefCount)
	}
}

func TestSetMembers_PreservesLiveEntries(t *testing.T) {
	m, _ := NewManager(sampleSpecs())
	// Mark ktw-docs as if it were live.
	c := m.conns["ktw-docs"]
	c.state = cmapi.ConnReady
	c.refCount = 1

	// New membership keeps ktw-docs (under its same name) but drops ktw-example.
	err := m.SetMembers([]cmapi.SpawnSpec{
		{Name: "ktw-docs", Transport: "stdio", Binary: "ktw-docs"},
		{Name: "ktw-notify", Transport: "stdio", Binary: "ktw-notify"},
	})
	if err != nil {
		t.Fatalf("SetMembers: %v", err)
	}

	// ktw-docs should remain in READY with refcount=1 (preserved live entry).
	st, ok := m.Status("ktw-docs")
	if !ok {
		t.Fatal("ktw-docs missing after SetMembers")
	}
	if st.State != cmapi.ConnReady {
		t.Errorf("ktw-docs State = %s, want READY (preserved)", st.State)
	}
	if st.RefCount != 1 {
		t.Errorf("ktw-docs RefCount = %d, want 1 (preserved)", st.RefCount)
	}

	// ktw-notify is new — DECLARED state.
	st2, _ := m.Status("ktw-notify")
	if st2.State != cmapi.ConnDeclared {
		t.Errorf("ktw-notify State = %s, want DECLARED", st2.State)
	}

	// ktw-example is gone.
	_, ok = m.Status("ktw-example")
	if ok {
		t.Error("ktw-example should have been removed")
	}
}

func TestInterfaceContract(t *testing.T) {
	var _ cmapi.ConnectionManager = (*ManagerImpl)(nil)
}

// ----------------------------------------------------------------------------
// NATS-mode constructor tests. End-to-end nats-mode spawn is covered by
// the integration test in nats_acquire_test.go (uses embedded NATS).
// ----------------------------------------------------------------------------

func TestNewManagerWithBus_RejectsNilBus(t *testing.T) {
	_, err := NewManagerWithBus(sampleSpecs(), BusScope{
		Bus:        nil,
		TenantID:   "local",
		UniverseID: "test",
	})
	if err == nil {
		t.Error("expected error for nil Bus, got nil")
	}
}

func TestNewManagerWithBus_RejectsEmptyTenant(t *testing.T) {
	stub := teststub.New()
	_, err := NewManagerWithBus(sampleSpecs(), BusScope{
		Bus:        stub,
		TenantID:   "",
		UniverseID: "test",
	})
	if err == nil {
		t.Error("expected error for empty TenantID, got nil")
	}
}

func TestNewManagerWithBus_RejectsEmptyUniverse(t *testing.T) {
	stub := teststub.New()
	_, err := NewManagerWithBus(sampleSpecs(), BusScope{
		Bus:        stub,
		TenantID:   "local",
		UniverseID: "",
	})
	if err == nil {
		t.Error("expected error for empty UniverseID, got nil")
	}
}

func TestAcquire_NATSWithoutBus_ErrorsExplicitly(t *testing.T) {
	// Stdio-only manager (no bus) but spec declares transport=nats —
	// must produce a clear error explaining the misconfiguration.
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "nats", Binary: "x"},
	})
	_, err := m.Acquire(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, _ := m.Status("x")
	if st.State != cmapi.ConnFailed {
		t.Errorf("State = %s, want FAILED", st.State)
	}
}

func TestAcquire_UnknownTransport_ErrorsExplicitly(t *testing.T) {
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "carrier-pigeon", Binary: "x"},
	})
	_, err := m.Acquire(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, _ := m.Status("x")
	if st.State != cmapi.ConnFailed {
		t.Errorf("State = %s, want FAILED", st.State)
	}
}

// ----------------------------------------------------------------------------
// Idle-timer tests (#12). Use very short timeouts (10ms) and synthesize
// the "ready" state by mutating stdioConnection directly — spawning real
// child processes here would couple tests to a binary on disk.
// ----------------------------------------------------------------------------

// fakeReady puts the named connection into READY with a dummy non-nil
// transport pointer so closeConn's transport.Close() path is exercised
// safely (it's nil-checked).
func fakeReady(t *testing.T, m *ManagerImpl, name string, refCount int) *stdioConnection {
	t.Helper()
	c, ok := m.conns[name]
	if !ok {
		t.Fatalf("fakeReady: unknown member %q", name)
	}
	c.mu.Lock()
	c.state = cmapi.ConnReady
	c.refCount = refCount
	c.spawnedAt = time.Now()
	c.lastCall = c.spawnedAt
	c.mu.Unlock()
	return c
}

// settleWait bounds a wait for something that does happen (an idle
// timer firing, an event arriving): far above any stall of a loaded or
// slow machine; a passing run polls and returns in milliseconds.
const settleWait = 10 * time.Second

// neverFires is an idle timeout no test outlives: for tests that need
// the timer armed or cancelled, never fired.
const neverFires = time.Hour

// waitForState polls Status until the named connection reaches the
// expected state or the deadline expires.
func waitForState(t *testing.T, m *ManagerImpl, name string, want cmapi.ConnState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, ok := m.Status(name)
		if ok && st.State == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	st, _ := m.Status(name)
	t.Fatalf("waited %v for %s state == %s; final state = %s", timeout, name, want, st.State)
}

func TestIdleTimer_Disconnects(t *testing.T) {
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "stdio", Binary: "x", IdleTimeout: 10 * time.Millisecond},
	})
	fakeReady(t, m, "x", 1)

	m.Release("x") // refCount → 0 → arms timer
	waitForState(t, m, "x", cmapi.ConnDeclared, settleWait)
}

func TestIdleTimer_DoesNotFireWhilePinned(t *testing.T) {
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "stdio", Binary: "x", IdleTimeout: 10 * time.Millisecond, Pin: true},
	})
	fakeReady(t, m, "x", 1)

	m.Release("x")
	time.Sleep(50 * time.Millisecond) // > IdleTimeout

	st, _ := m.Status("x")
	if st.State != cmapi.ConnReady {
		t.Errorf("Pin=true: state = %s, want READY (idle timer must not fire)", st.State)
	}
}

func TestIdleTimer_DoesNotFireWhileStateful(t *testing.T) {
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "stdio", Binary: "x", IdleTimeout: 10 * time.Millisecond, Lifecycle: cmapi.LifecycleStateful},
	})
	fakeReady(t, m, "x", 1)

	m.Release("x")
	time.Sleep(50 * time.Millisecond)

	st, _ := m.Status("x")
	if st.State != cmapi.ConnReady {
		t.Errorf("Lifecycle=stateful: state = %s, want READY", st.State)
	}
}

func TestIdleTimer_DisabledWhenTimeoutZero(t *testing.T) {
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "stdio", Binary: "x"}, // IdleTimeout zero (default)
	})
	fakeReady(t, m, "x", 1)

	m.Release("x")
	time.Sleep(30 * time.Millisecond)

	st, _ := m.Status("x")
	if st.State != cmapi.ConnReady {
		t.Errorf("IdleTimeout=0: state = %s, want READY", st.State)
	}
}

func TestIdleTimer_AcquireCancelsPendingTimer(t *testing.T) {
	// An idle timeout far above any scheduler stall: the test needs the
	// timer armed and cancelled, never fired, so a loaded machine cannot
	// let it fire in the gap after Release.
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "stdio", Binary: "x", IdleTimeout: neverFires},
	})
	c := fakeReady(t, m, "x", 1)
	m.Release("x") // arm idle timer

	// Inspect: timer is armed.
	c.mu.Lock()
	if c.idleTimer == nil {
		c.mu.Unlock()
		t.Fatal("expected idleTimer armed after Release")
	}
	armedEpoch := c.epoch
	c.mu.Unlock()

	// "Re-acquire" — manually replicate the Acquire mutation since real
	// Acquire would try to spawn the binary. This still exercises
	// cancelIdleTimerLocked.
	c.mu.Lock()
	c.cancelIdleTimerLocked()
	c.refCount = 1
	cancelled := c.idleTimer == nil
	epochBumped := c.epoch != armedEpoch
	c.mu.Unlock()

	if !cancelled {
		t.Error("After Acquire: idleTimer still armed, want it cancelled")
	}
	if !epochBumped {
		t.Error("After Acquire: epoch unchanged, want it bumped so a queued callback bails")
	}
	st, _ := m.Status("x")
	if st.State != cmapi.ConnReady {
		t.Errorf("After Acquire: state = %s, want READY (timer should have been cancelled)", st.State)
	}
}

func TestIdleTimer_RearmAfterReReady(t *testing.T) {
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "stdio", Binary: "x", IdleTimeout: 10 * time.Millisecond},
	})
	c := fakeReady(t, m, "x", 1)

	// First idle cycle: Release → fire → DECLARED.
	m.Release("x")
	waitForState(t, m, "x", cmapi.ConnDeclared, settleWait)

	// Re-set to Ready (simulating a fresh Acquire spawn) and Release again.
	c.mu.Lock()
	c.state = cmapi.ConnReady
	c.refCount = 1
	c.spawnedAt = time.Now()
	c.mu.Unlock()

	m.Release("x")
	waitForState(t, m, "x", cmapi.ConnDeclared, settleWait)
}

func TestIdleTimer_NoDisconnectIfRefcountGrowsBack(t *testing.T) {
	// The real timer never fires (neverFires); the test plays the
	// callback itself once the refcount has grown back, so no sleep
	// races the timer on a loaded machine.
	m, _ := NewManager([]cmapi.SpawnSpec{
		{Name: "x", Transport: "stdio", Binary: "x", IdleTimeout: neverFires},
	})
	c := fakeReady(t, m, "x", 1)
	m.Release("x") // refCount → 0; arm timer

	// Bump refcount back via a synthesised "second Acquire" before
	// the timer fires. The timer's idleDisconnect should observe
	// refCount > 0 and bail.
	c.mu.Lock()
	c.cancelIdleTimerLocked()
	c.refCount = 1
	c.mu.Unlock()

	m.idleDisconnect(c) // a callback that fired before the cancel
	st, _ := m.Status("x")
	if st.State != cmapi.ConnReady {
		t.Errorf("state = %s, want READY (refcount grew back before timer)", st.State)
	}
}
