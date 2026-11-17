// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// TestOnlyTheUniverseOverrideTreeMovesAMembersStalenessKey: a member's record
// is keyed by the universe's override tree (behind allow_overwrites) and by
// nothing of an attached project — a project's overrides reach the catalog as
// override entries through its member's push, never as a tree the hub hashes.
// An edit in the project's overwrites/<member>/ leaves the record fresh; an
// edit in the universe's overwrites/<member>/ makes it stale.
func TestOnlyTheUniverseOverrideTreeMovesAMembersStalenessKey(t *testing.T) {
	universe, project := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(universe, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(universe, "config", "Univers.yaml"),
		[]byte("name: example\nallow_overwrites: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prevUniverse, prevProjects := overwritesUniverseDir, attachedProjectRoots
	overwritesUniverseDir = func() string { return universe }
	attachedProjectRoots = func() []string { return []string{project} }
	t.Cleanup(func() { overwritesUniverseDir, attachedProjectRoots = prevUniverse, prevProjects })

	write := func(dir, file string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte("---\nname: "+file+"\n---\nbody\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	projectTree := filepath.Join(project, "modules", "sys-contributions", "overwrites", "ktw-example", "embed", "skills")
	universeTree := filepath.Join(universe, "overwrites", "ktw-example", "embed", "skills")
	write(projectTree, "greet.md")
	write(universeTree, "greet.md")

	exe := filepath.Join(t.TempDir(), "ktw-example.exe")
	if err := os.WriteFile(exe, []byte("build-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	members := []appcore.SidecarSpec{{Name: "ktw-example", Binary: exe}}
	cache := manifestlocal.NewCache(t.TempDir())
	m := &mbapi.Manifest{Provider: "ktw-example", Version: "1", Hash: "h-1"}
	if err := cache.Save("ktw-example", m, exe, sourceDigestsIn("ktw-example", members)); err != nil {
		t.Fatal(err)
	}
	fresh := func() bool {
		t.Helper()
		ok, err := recordIsFresh(cache, "ktw-example", exe, sourceDigestsIn("ktw-example", members))
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !fresh() {
		t.Fatal("a record just saved reads stale")
	}

	write(projectTree, "extra.md")
	if !fresh() {
		t.Fatal("an edit in the attached project's override tree makes the member's record stale; the project tree is no part of its key")
	}

	write(universeTree, "extra.md")
	if fresh() {
		t.Fatal("an edit in the universe's override tree leaves the member's record fresh")
	}
}
