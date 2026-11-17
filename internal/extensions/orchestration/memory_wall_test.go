// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
)

// The wall: a run joins only its own tenant's, universe's and
// person's memory. Each pair differs in one key part; the other's marker
// never reaches the prompt.
func TestMemoryNeverCrossesTenantUniverseOrPerson(t *testing.T) {
	root := t.TempDir()
	base := MemoryKey{Tenant: "t1", Universe: "u1", Person: "alice"}
	others := map[string]MemoryKey{
		"tenant":   {Tenant: "t2", Universe: "u1", Person: "alice"},
		"universe": {Tenant: "t1", Universe: "u2", Person: "alice"},
		"person":   {Tenant: "t1", Universe: "u1", Person: "bob"},
	}
	if err := NewMemoryManager(root, base).SaveMemory("MARKER-BASE"); err != nil {
		t.Fatal(err)
	}
	for part, k := range others {
		if err := NewMemoryManager(root, k).SaveMemory("MARKER-" + part); err != nil {
			t.Fatal(err)
		}
	}
	got := NewMemoryManager(root, base).LoadMemoryPrompt()
	if !strings.Contains(got, "MARKER-BASE") {
		t.Fatalf("own memory not joined: %q", got)
	}
	for part, k := range others {
		if strings.Contains(got, "MARKER-"+part) {
			t.Errorf("memory of another %s joined into the prompt: %q", part, got)
		}
		if o := NewMemoryManager(root, k).LoadMemoryPrompt(); strings.Contains(o, "MARKER-BASE") {
			t.Errorf("the other %s's run joined base's memory: %q", part, o)
		}
	}
}

// A run that names no person (or no tenant or universe) joins no memory, and a
// key part cannot climb out of its folder nor reach the quarantine.
func TestMemoryIncompleteOrUnsafeKeyReadsNothing(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "t1", "u1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "t1", "u1", memoryEntrypoint), []byte("LEAK"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, k := range []MemoryKey{
		{Tenant: "t1", Universe: "u1"},
		{Tenant: "t1", Person: "alice"},
		{Universe: "u1", Person: "alice"},
		{Tenant: "t1", Universe: "u1", Person: ".."},
		{Tenant: "t1", Universe: "u1", Person: "_quarantine"},
	} {
		m := NewMemoryManager(root, k)
		if d := m.MemoryDir(); d != "" {
			t.Errorf("key %+v has a memory dir %q", k, d)
		}
		if p := m.LoadMemoryPrompt(); p != "" {
			t.Errorf("key %+v joined %q", k, p)
		}
		if err := m.SaveMemory("x"); err == nil {
			t.Errorf("key %+v saved memory", k)
		}
	}
	// A separator inside a part stays inside its folder.
	d := NewMemoryManager(root, MemoryKey{Tenant: "t1", Universe: "u1", Person: "a/../../x"}).MemoryDir()
	if rel, err := filepath.Rel(filepath.Join(root, "t1", "u1"), d); err != nil || rel == ".." || strings.ContainsAny(rel, `/\`) {
		t.Errorf("unsafe person escapes its folder: %q", d)
	}
}

// No memory path sits under the working folder, and memory is written for its
// owner only.
func TestMemoryNeverInsideTheWorkingFolder(t *testing.T) {
	work := t.TempDir()
	data := t.TempDir()
	m := NewMemoryManager(filepath.Join(data, "agent-memory"), MemoryKey{Tenant: "t", Universe: "u", Person: "p"})
	if err := m.SaveMemory("x"); err != nil {
		t.Fatal(err)
	}
	if rel, err := filepath.Rel(work, m.MemoryDir()); err == nil && !strings.HasPrefix(rel, "..") {
		t.Fatalf("memory dir %q is under the working folder %q", m.MemoryDir(), work)
	}
	entries, _ := os.ReadDir(work)
	if len(entries) != 0 {
		t.Fatalf("the working folder got %d entries", len(entries))
	}
}

// The memory written before the wall - the home folder's and the working
// folder's - is moved into the quarantine and never joined into a prompt.
func TestLegacyMemoryIsQuarantinedAndNotJoined(t *testing.T) {
	home, work, data := t.TempDir(), t.TempDir(), t.TempDir()
	root := filepath.Join(data, "agent-memory")
	legacy := LegacyMemoryDirs(home, work)
	for i, d := range legacy {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, memoryEntrypoint), []byte("LEGACY-"+string(rune('A'+i))), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	moved, err := QuarantineLegacyMemory(root, legacy, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != len(legacy) {
		t.Fatalf("moved %d of %d legacy folders", len(moved), len(legacy))
	}
	for _, d := range legacy {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("legacy folder %q still in place", d)
		}
	}
	for _, d := range moved {
		if !strings.HasPrefix(d, filepath.Join(root, quarantineDir)) {
			t.Errorf("moved to %q, outside the quarantine", d)
		}
		if _, err := os.Stat(filepath.Join(d, memoryEntrypoint)); err != nil {
			t.Errorf("quarantined memory lost in %q: %v", d, err)
		}
	}
	// No key, whatever it names, reads the quarantine.
	for _, p := range []string{"alice", quarantineDir} {
		if got := NewMemoryManager(root, MemoryKey{Tenant: "local", Universe: "default", Person: p}).LoadMemoryPrompt(); strings.Contains(got, "LEGACY") {
			t.Errorf("quarantined memory joined for %q: %q", p, got)
		}
	}
}

// The run's person comes from its cause: a person's own act, or the person an
// agent's session relays; an agent run alone names none.
func TestRunPersonFromCause(t *testing.T) {
	ctx := context.Background()
	if p := runPerson(ctx); p != "" {
		t.Errorf("no cause: person %q", p)
	}
	if p := runPerson(cause.WithCause(ctx, cause.Cause{Kind: cause.KindPerson, ID: "alice"})); p != "alice" {
		t.Errorf("person cause: %q", p)
	}
	if p := runPerson(cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "run-1", Relay: &cause.Relay{Person: "bob"}})); p != "bob" {
		t.Errorf("relayed cause: %q", p)
	}
	if p := runPerson(cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "run-1"})); p != "" {
		t.Errorf("agent cause: %q", p)
	}
}
