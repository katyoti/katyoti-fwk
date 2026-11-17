// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
)

// quarantineOf puts legacy folders, each holding files, into root's
// quarantine through QuarantineLegacyMemory and answers their names.
func quarantineOf(t *testing.T, root string, legacy ...map[string]string) []string {
	t.Helper()
	var dirs []string
	for i, files := range legacy {
		src := filepath.Join(t.TempDir(), "legacy", string(rune('a'+i)))
		for name, body := range files {
			p := filepath.Join(src, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		dirs = append(dirs, src)
	}
	moved, err := QuarantineLegacyMemory(root, dirs, time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC))
	if err != nil || len(moved) != len(dirs) {
		t.Fatalf("quarantine: moved %v, err %v", moved, err)
	}
	names := make([]string, 0, len(moved))
	for _, m := range moved {
		names = append(names, filepath.Base(m))
	}
	return names
}

// Each quarantined folder raises exactly one card in « À faire », keyed by its
// name and naming where it came from; none when nothing waits; a folder gone
// takes its card with it.
func TestAQuarantinedFolderRaisesOneCard(t *testing.T) {
	root := t.TempDir()
	src := memoryWaitingSource(func() string { return root })
	if got, err := src.Items(context.Background()); err != nil || len(got) != 0 {
		t.Fatalf("empty quarantine: %d items, err %v", len(got), err)
	}
	names := quarantineOf(t, root, map[string]string{memoryEntrypoint: "one"}, map[string]string{memoryEntrypoint: "two", "topic.md": "x"})
	got, err := src.Items(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("two folders: %d items, err %v", len(got), err)
	}
	for _, it := range got {
		if !strings.HasPrefix(it.Key, MemoryWaitingSourceName+":") || it.Ref == "" || it.Kind != "choose" {
			t.Errorf("item not keyed by its folder as a choice: %+v", it)
		}
		if it.About != "agent" {
			t.Errorf("« Ouvrir » must open the agent module, an object every installation has: About %q", it.About)
		}
		if !strings.Contains(it.Asks, "legacy") {
			t.Errorf("item does not name where it came from: %q", it.Asks)
		}
		ctrl, _ := json.Marshal(src.Controls(context.Background(), it, "fr"))
		for _, want := range []string{memoryAssignButton + it.Ref, memoryDeleteButton + it.Ref, memoryAssignURI, memoryDeleteURI, "Garder comme ma mémoire ici"} {
			if !strings.Contains(string(ctrl), want) {
				t.Errorf("controls lack %q: %s", want, ctrl)
			}
		}
	}
	if err := DeleteQuarantinedMemory(root, names[0]); err != nil {
		t.Fatal(err)
	}
	got, _ = src.Items(context.Background())
	if len(got) != 1 || got[0].Ref != names[1] {
		t.Fatalf("after delete: %+v", got)
	}
}

// Keeping a quarantined folder makes it that key's memory only: joined into
// that key's prompt, never another person's nor another universe's; a
// MEMORY.md already there keeps its text with the kept one appended; the
// quarantine folder is gone.
func TestAssignKeepsQuarantinedMemoryForThatKeyOnly(t *testing.T) {
	root := t.TempDir()
	key := MemoryKey{Tenant: "t1", Universe: "u1", Person: "alice"}
	if err := NewMemoryManager(root, key).SaveMemory("OWN-TEXT"); err != nil {
		t.Fatal(err)
	}
	names := quarantineOf(t, root, map[string]string{memoryEntrypoint: "KEPT-TEXT", "notes/topic.md": "TOPIC"})
	if err := AssignQuarantinedMemory(root, names[0], key); err != nil {
		t.Fatal(err)
	}
	got := NewMemoryManager(root, key).LoadMemoryPrompt()
	if !strings.Contains(got, "OWN-TEXT") || !strings.Contains(got, "KEPT-TEXT") || !strings.Contains(got, "## Kept from") {
		t.Fatalf("kept memory not joined with the own one: %q", got)
	}
	if b, err := os.ReadFile(filepath.Join(NewMemoryManager(root, key).MemoryDir(), "notes", "topic.md")); err != nil || string(b) != "TOPIC" {
		t.Errorf("other file not moved: %q %v", b, err)
	}
	for _, other := range []MemoryKey{{Tenant: "t1", Universe: "u1", Person: "bob"}, {Tenant: "t1", Universe: "u2", Person: "alice"}} {
		if p := NewMemoryManager(root, other).LoadMemoryPrompt(); strings.Contains(p, "KEPT-TEXT") {
			t.Errorf("kept memory reached %+v: %q", other, p)
		}
	}
	if _, err := os.Stat(filepath.Join(root, quarantineDir, names[0])); !os.IsNotExist(err) {
		t.Errorf("quarantine folder still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(NewMemoryManager(root, key).MemoryDir(), quarantineMarkerFile)); !os.IsNotExist(err) {
		t.Errorf("the marker was kept as memory: %v", err)
	}
}

// A file of another name already in the key's memory refuses the keep:
// nothing moved, the quarantine folder stays.
func TestAssignRefusesAConflictingFile(t *testing.T) {
	root := t.TempDir()
	key := MemoryKey{Tenant: "t1", Universe: "u1", Person: "alice"}
	dir := filepath.Join(root, "t1", "u1", "alice")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "topic.md"), []byte("MINE"), 0o600); err != nil {
		t.Fatal(err)
	}
	names := quarantineOf(t, root, map[string]string{memoryEntrypoint: "KEPT", "topic.md": "THEIRS"})
	if err := AssignQuarantinedMemory(root, names[0], key); err == nil || !strings.Contains(err.Error(), "topic.md") {
		t.Fatalf("conflict not refused by name: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, memoryEntrypoint)); !os.IsNotExist(err) {
		t.Errorf("something moved despite the refusal")
	}
	if _, err := os.Stat(filepath.Join(root, quarantineDir, names[0])); err != nil {
		t.Errorf("quarantine folder gone after a refusal: %v", err)
	}
}

func TestDeleteRemovesQuarantinedMemory(t *testing.T) {
	root := t.TempDir()
	names := quarantineOf(t, root, map[string]string{memoryEntrypoint: "x"})
	if err := DeleteQuarantinedMemory(root, names[0]); err != nil {
		t.Fatal(err)
	}
	if got, _ := QuarantinedMemory(root); len(got) != 0 {
		t.Fatalf("still quarantined: %+v", got)
	}
}

// A name that climbs, holds a separator, is absolute or names nothing is
// refused by keep and delete alike, and nothing moves.
func TestQuarantineNamesCannotEscape(t *testing.T) {
	root := t.TempDir()
	quarantineOf(t, root, map[string]string{memoryEntrypoint: "x"})
	victim := filepath.Join(root, "t1", "u1", "alice")
	if err := os.MkdirAll(victim, 0o700); err != nil {
		t.Fatal(err)
	}
	key := MemoryKey{Tenant: "t1", Universe: "u1", Person: "alice"}
	for _, name := range []string{"", ".", "..", "../t1", "a/b", `a\b`, filepath.Join(root, "t1"), "C:x", "no-such-folder"} {
		if err := AssignQuarantinedMemory(root, name, key); err == nil {
			t.Errorf("assign %q accepted", name)
		}
		if err := DeleteQuarantinedMemory(root, name); err == nil {
			t.Errorf("delete %q accepted", name)
		}
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("a refused name removed a folder: %v", err)
	}
	if got, _ := QuarantinedMemory(root); len(got) != 1 {
		t.Errorf("the quarantine changed: %+v", got)
	}
}

// Keeping is for the person who calls - their cause, or the person an
// agent's session relays; no input names a person; an agent acting for no
// person is refused; a call with no cause is the universe's person's.
func TestAssignTakesThePersonFromTheCause(t *testing.T) {
	root := t.TempDir()
	svc := memoryService{
		root:    func() string { return root },
		place:   func() (string, string) { return "t1", "u1" },
		profile: func(context.Context) string { return "owner" },
	}
	names := quarantineOf(t, root,
		map[string]string{memoryEntrypoint: "BY-CAUSE"},
		map[string]string{memoryEntrypoint: "NO-CAUSE"},
		map[string]string{memoryEntrypoint: "FOR-NOBODY"})
	byName := map[string]string{}
	for _, n := range names {
		data, _ := os.ReadFile(filepath.Join(root, quarantineDir, n, memoryEntrypoint))
		byName[string(data)] = n
	}
	ownerKey := MemoryKey{Tenant: "t1", Universe: "u1", Person: "owner"}

	owner := cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindPerson, ID: "owner"})
	if r, err := svc.Assign(owner, QuarantinedMemoryNameInput{Name: byName["BY-CAUSE"]}); err != nil || !r.OK {
		t.Fatalf("the owner's keep: %+v %v", r, err)
	}
	if p := NewMemoryManager(root, ownerKey).LoadMemoryPrompt(); !strings.Contains(p, "BY-CAUSE") {
		t.Errorf("not kept under the caller's key: %q", p)
	}

	if r, err := svc.Assign(context.Background(), QuarantinedMemoryNameInput{Name: byName["NO-CAUSE"]}); err != nil || !r.OK {
		t.Fatalf("no-cause keep: %+v %v", r, err)
	}
	if p := NewMemoryManager(root, ownerKey).LoadMemoryPrompt(); !strings.Contains(p, "NO-CAUSE") {
		t.Errorf("a call with no cause not kept as the universe's person's: %q", p)
	}

	agent := cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindAgent, ID: "some-agent"})
	in := QuarantinedMemoryNameInput{Name: byName["FOR-NOBODY"]}
	if r, err := svc.Assign(agent, in); err != nil || r.OK {
		t.Errorf("an agent acting for no person kept memory: %+v %v", r, err)
	}
	if r, err := svc.Delete(agent, in); err != nil || r.OK {
		t.Errorf("an agent acting for no person deleted memory: %+v %v", r, err)
	}
	relayed := cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindAgent, ID: "some-agent", Relay: &cause.Relay{Person: "owner"}})
	if p, err := svc.caller(relayed); err != nil || p != "owner" {
		t.Errorf("a relayed act is not the relayed person's: %q %v", p, err)
	}
	if got, _ := QuarantinedMemory(root); len(got) != 1 || got[0].Name != byName["FOR-NOBODY"] {
		t.Fatalf("an agent acting for no person moved quarantined memory: %+v", got)
	}
}

// The quarantined memory came from the universe's person's own folders: only
// that person keeps or deletes it. Another person of a shared universe - by
// their cause or relayed by an agent's session - is refused for both acts and
// nothing moves; a universe with no profile refuses everyone.
func TestOnlyTheUniversesPersonKeepsQuarantinedMemory(t *testing.T) {
	root := t.TempDir()
	svc := memoryService{
		root:    func() string { return root },
		place:   func() (string, string) { return "t1", "u1" },
		profile: func(context.Context) string { return "owner" },
	}
	names := quarantineOf(t, root,
		map[string]string{memoryEntrypoint: "OWNER-KEEPS"},
		map[string]string{memoryEntrypoint: "OWNER-DELETES"})
	byName := map[string]string{}
	for _, n := range names {
		data, _ := os.ReadFile(filepath.Join(root, quarantineDir, n, memoryEntrypoint))
		byName[string(data)] = n
	}

	others := map[string]context.Context{
		"another person":          cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindPerson, ID: "bob"}),
		"a relay for another one": cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindAgent, ID: "some-agent", Relay: &cause.Relay{Person: "bob"}}),
	}
	for who, ctx := range others {
		for _, n := range names {
			in := QuarantinedMemoryNameInput{Name: n}
			if r, err := svc.Assign(ctx, in); err != nil || r.OK || !strings.Contains(r.Message, "only owner") {
				t.Errorf("%s kept the owner's memory: %+v %v", who, r, err)
			}
			if r, err := svc.Delete(ctx, in); err != nil || r.OK || !strings.Contains(r.Message, "only owner") {
				t.Errorf("%s deleted the owner's memory: %+v %v", who, r, err)
			}
		}
	}
	if p := NewMemoryManager(root, MemoryKey{Tenant: "t1", Universe: "u1", Person: "bob"}).LoadMemoryPrompt(); strings.Contains(p, "OWNER-") {
		t.Errorf("another person got the owner's memory: %q", p)
	}
	if got, _ := QuarantinedMemory(root); len(got) != 2 {
		t.Fatalf("a refused act moved quarantined memory: %+v", got)
	}

	noProfile := svc
	noProfile.profile = func(context.Context) string { return "" }
	if r, err := noProfile.Assign(context.Background(), QuarantinedMemoryNameInput{Name: byName["OWNER-KEEPS"]}); err != nil || r.OK {
		t.Errorf("a universe with no profile kept memory: %+v %v", r, err)
	}

	owner := cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindPerson, ID: "owner"})
	if r, err := svc.Assign(owner, QuarantinedMemoryNameInput{Name: byName["OWNER-KEEPS"]}); err != nil || !r.OK {
		t.Fatalf("the owner's keep: %+v %v", r, err)
	}
	if p := NewMemoryManager(root, MemoryKey{Tenant: "t1", Universe: "u1", Person: "owner"}).LoadMemoryPrompt(); !strings.Contains(p, "OWNER-KEEPS") {
		t.Errorf("not kept under the owner's key: %q", p)
	}
	if r, err := svc.Delete(owner, QuarantinedMemoryNameInput{Name: byName["OWNER-DELETES"]}); err != nil || !r.OK {
		t.Fatalf("the owner's delete: %+v %v", r, err)
	}
	if got, _ := QuarantinedMemory(root); len(got) != 0 {
		t.Fatalf("the owner's acts left quarantined memory: %+v", got)
	}
}
