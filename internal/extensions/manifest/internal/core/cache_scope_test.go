// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// scopeBinary writes a stand-in member binary.
func scopeBinary(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "ktw-x.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return exe
}

// recordFolders lists the record folders of ktw-x under root.
func recordFolders(t *testing.T, root string) []string {
	t.Helper()
	got, _ := filepath.Glob(filepath.Join(root, "ktw-x@*"))
	for i := range got {
		got[i] = filepath.Base(got[i])
	}
	return got
}

// TestBuilder_RecordsDeclaredReadRoots — a binary's DeclareReadRoot
// contributions reach Manifest.ReadRoots, deduplicated and sorted, and stay
// out of the hash: where content comes from is not what it is.
func TestBuilder_RecordsDeclaredReadRoots(t *testing.T) {
	roots := []mbapi.ReadRoot{
		{Anchor: mbapi.ReadRootUniverse, Path: "tasks"},
		{Anchor: mbapi.ReadRootGlobal, Path: "skills"},
		{Anchor: mbapi.ReadRootUniverse, Path: "skills"},
		{Anchor: mbapi.ReadRootUniverse, Path: "tasks"},
	}
	app := application.New(application.Config(), func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", "read-roots", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[mbapi.ReadRoot]())
		})
		init.AddContribute("test", "read-roots", func(app *application.Application) error {
			for _, r := range roots {
				if err := application.Contribute(app, r); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	m, err := NewBuilder("ktw-x", "1").Build(app.Context())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want := []mbapi.ReadRoot{
		{Anchor: mbapi.ReadRootGlobal, Path: "skills"},
		{Anchor: mbapi.ReadRootUniverse, Path: "skills"},
		{Anchor: mbapi.ReadRootUniverse, Path: "tasks"},
	}
	if len(m.ReadRoots) != len(want) {
		t.Fatalf("ReadRoots = %v, want %v", m.ReadRoots, want)
	}
	for i := range want {
		if m.ReadRoots[i] != want[i] {
			t.Fatalf("ReadRoots = %v, want %v", m.ReadRoots, want)
		}
	}
	bare, err := NewBuilder("ktw-x", "1").Build(context.Background())
	if err != nil {
		t.Fatalf("Build without declarations: %v", err)
	}
	if bare.Hash != m.Hash {
		t.Errorf("declaring read roots changed the hash: %s vs %s", m.Hash, bare.Hash)
	}
}

// TestCache_OverridesKeepARecordPerUniverse — a record taken with a universe's
// override tree is that universe's: Save writes it to the universe's own
// folder, and once the overrides are gone Save writes the shared record and
// removes the universe's own, so Load does not keep preferring a stale one.
func TestCache_OverridesKeepARecordPerUniverse(t *testing.T) {
	root := t.TempDir()
	exe := scopeBinary(t)
	c := NewCache(root).ForUniverse(t.TempDir(), t.TempDir())
	m := &mbapi.Manifest{Provider: "ktw-x", Version: "1", Hash: "with-overrides"}
	if err := c.Save("ktw-x", m, exe, SourceDigests{Binary: exe, Overwrites: "o1"}); err != nil {
		t.Fatal(err)
	}
	got := recordFolders(t, root)
	if len(got) != 1 || !strings.Contains(got[0], scopeSep) {
		t.Fatalf("record folders %v; want one universe-scoped folder", got)
	}
	m2 := &mbapi.Manifest{Provider: "ktw-x", Version: "1", Hash: "plain"}
	if err := c.Save("ktw-x", m2, exe, SourceDigests{Binary: exe}); err != nil {
		t.Fatal(err)
	}
	got = recordFolders(t, root)
	if len(got) != 1 || strings.Contains(got[0], scopeSep) {
		t.Fatalf("record folders %v; want only the shared folder once the overrides are gone", got)
	}
	loaded, _, err := c.Load("ktw-x", exe)
	if err != nil || loaded == nil || loaded.Hash != "plain" {
		t.Fatalf("Load = %v, %v; want the shared plain record", loaded, err)
	}
}

// TestCache_ALegacySharedRecordIsNotServedToAUniverse — a shared record of the
// format before records were kept per universe may be another universe's; a
// universe's cache answers a miss for it, so a failed probe never leaves the
// catalog serving another universe's entries.
func TestCache_ALegacySharedRecordIsNotServedToAUniverse(t *testing.T) {
	root := t.TempDir()
	exe := scopeBinary(t)
	shared := NewCache(root)
	m := &mbapi.Manifest{Provider: "ktw-x", Version: "1", Hash: "legacy"}
	if err := shared.Save("ktw-x", m, exe, SourceDigests{Binary: exe}); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(shared.RecordDir("ktw-x", exe), "metadata.yaml")
	b, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Replace(string(b), "format: 5", "format: 4", 1)
	if old == string(b) {
		t.Fatalf("metadata does not say format: 5:\n%s", b)
	}
	if err := os.WriteFile(metaPath, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	scoped := NewCache(root).ForUniverse(t.TempDir(), t.TempDir())
	got, meta, err := scoped.Load("ktw-x", exe)
	if err != nil || got != nil || meta != nil {
		t.Errorf("a universe's cache loads the legacy shared record: manifest=%v meta=%v err=%v; want a miss", got, meta, err)
	}
	if got, _, _ := shared.Load("ktw-x", exe); got == nil {
		t.Error("the unscoped cache no longer lists the legacy record (operator views read it)")
	}
}

// TestScopeKey_OneUniverseOneScope — one universe folder reached by two
// spellings is one scope: on Windows `D:\x` and `d:\x`, and anywhere a
// relative path and its absolute form.
func TestScopeKey_OneUniverseOneScope(t *testing.T) {
	dir := t.TempDir()
	if scopeKey(dir) != scopeKey(dir+string(filepath.Separator)) {
		t.Error("a trailing separator changes the scope")
	}
	wd, _ := os.Getwd()
	rel, err := filepath.Rel(wd, dir)
	if err == nil && scopeKey(rel) != scopeKey(dir) {
		t.Error("a relative path and its absolute form are two scopes")
	}
	if runtime.GOOS == "windows" && scopeKey(strings.ToUpper(dir)) != scopeKey(strings.ToLower(dir)) {
		t.Error("on Windows, two casings of one path are two scopes")
	}
}

// TestCache_GCSweepsTheRecordsOfAGoneUniverse — a universe's own record whose
// universe folder no longer exists is swept with the orphans; a living
// universe's is kept.
func TestCache_GCSweepsTheRecordsOfAGoneUniverse(t *testing.T) {
	root := t.TempDir()
	exe := scopeBinary(t)
	gone, alive := t.TempDir(), t.TempDir()
	m := &mbapi.Manifest{Provider: "ktw-x", Version: "1", Hash: "h"}
	for _, u := range []string{gone, alive} {
		if err := NewCache(root).ForUniverse(u, "").Save("ktw-x", m, exe, SourceDigests{Binary: exe, Overwrites: "o"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	res, err := NewCache(root).GC([]string{"ktw-x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.OrphansRemoved) != 1 || !strings.HasSuffix(res.OrphansRemoved[0], scopeSep+scopeKey(gone)) {
		t.Fatalf("GC removed %v; want only the gone universe's record", res.OrphansRemoved)
	}
	if left := recordFolders(t, root); len(left) != 1 || !strings.HasSuffix(left[0], scopeSep+scopeKey(alive)) {
		t.Errorf("records left %v; want the living universe's", left)
	}
}
