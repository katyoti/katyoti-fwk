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
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/prereqs"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/registry"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/wire"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// : RepinSolution moves the universes a solution installed to
// another of its versions - the update path's repin, and its rollback.

// repinFixture is a scoped katyoti root whose marketplace offers the content
// items notes (1.0 and 2.0) and cards (1.0), all in the content store, and the
// solution duo: version 1 pins notes 1.0, version 2 notes 2.0, both cards 1.0,
// in two universes.
func repinFixture(t *testing.T) *service {
	t.Helper()
	base := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", filepath.Join(base, "root"))
	t.Setenv("KTW_WORKSPACE", filepath.Join(base, "ws"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "cfg"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	prev := hostBinaryPath
	hostBinaryPath = func() string { return filepath.Join(t.TempDir(), "no-ktw-host.exe") }
	t.Cleanup(func() { hostBinaryPath = prev })
	for _, nv := range [][2]string{{"notes", "1.0"}, {"notes", "2.0"}, {"cards", "1.0"}} {
		folder, err := store.ContentFolderPath(nv[0], nv[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "ktw-host.yaml"), []byte("name: "+nv[0]+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	version := func(v string) string {
		return "      - version: \"" + v + "\"\n        assets:\n          - url: x-" + v + ".tar.gz\n"
	}
	solution := func(v, notes string) string {
		return `      - version: "` + v + `"
        includes:
          - {name: notes, version: "` + notes + `"}
          - {name: cards, version: "1.0"}
        universes:
          - name: studio
            main: true
            displayName: Studio
            sidecars: [{name: notes}, {name: cards}]
          - name: review
            sidecars: [{name: cards}]
`
	}
	cat := "apiVersion: 1\nmarketplace:\n  name: m\nitems:\n" +
		"  - name: notes\n    kind: content\n    versions:\n" + version("2.0") + version("1.0") +
		"  - name: cards\n    kind: content\n    versions:\n" + version("1.0") +
		"  - name: duo\n    kind: solution\n    versions:\n" + solution("2", "2.0") + solution("1", "1.0")
	catPath := filepath.Join(base, "market", "katyoti-marketplace.yaml")
	if err := os.MkdirAll(filepath.Dir(catPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catPath, []byte(cat), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add("m", catPath, "", mktapi.TrustNone); err != nil {
		t.Fatal(err)
	}
	return &service{reg: reg, prereqs: &prereqs.Engine{Runner: noInstall{}, Platform: runtime.GOOS}}
}

func solutionOf(t *testing.T, home string) string {
	t.Helper()
	m, err := orchcatalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	return m.Solution
}

func TestASolutionIsRepinnedToItsNewVersionAndBack(t *testing.T) {
	s := repinFixture(t)
	ctx := context.Background()
	if _, err := s.InstallSolution(ctx, mktapi.InstallSolutionInput{Ref: mktapi.ItemRef{Name: "duo", Version: "1"}, UniverseHome: "team"}); err != nil {
		t.Fatal(err)
	}
	team, review := registered(t)["team"], registered(t)["team-review"]
	if !wire.PinsVersion(team, "notes", "1.0") || solutionOf(t, team) != "duo@1" {
		t.Fatalf("installed: team pins notes 1.0 = %v, solution %q", wire.PinsVersion(team, "notes", "1.0"), solutionOf(t, team))
	}
	// A display name the person gave is kept by a repin.
	m, _ := orchcatalog.LoadManifest(team)
	m.DisplayName = "Our team"
	if err := orchcatalog.SaveManifest(team, m); err != nil {
		t.Fatal(err)
	}

	dry, err := s.RepinSolution(ctx, mktapi.RepinSolutionInput{UniverseHome: "team", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if dry.From != "1" || dry.To != "2" || len(dry.Members) != 1 || dry.Members[0] != (mktapi.MemberMove{Name: "notes", From: "1.0", To: "2.0"}) {
		t.Fatalf("dry run = %+v; want duo 1 → 2, notes 1.0 → 2.0", dry)
	}
	if len(dry.Universes) != 2 || dry.Universes[0] != team || dry.Universes[1] != review {
		t.Fatalf("universes %v; want team then team-review", dry.Universes)
	}
	if !wire.PinsVersion(team, "notes", "1.0") || solutionOf(t, team) != "duo@1" {
		t.Fatal("the dry run moved the universe")
	}

	if _, err := s.RepinSolution(ctx, mktapi.RepinSolutionInput{UniverseHome: "team"}); err != nil {
		t.Fatal(err)
	}
	if !wire.PinsVersion(team, "notes", "2.0") || solutionOf(t, team) != "duo@2" || solutionOf(t, review) != "duo@2" {
		t.Fatalf("after the repin: team pins notes 2.0 = %v, solutions %q %q", wire.PinsVersion(team, "notes", "2.0"), solutionOf(t, team), solutionOf(t, review))
	}
	if got := strings.Join(members(t, review), ","); got != "cards" {
		t.Errorf("team-review wires %s, want cards alone", got)
	}
	if m, _ := orchcatalog.LoadManifest(team); m.DisplayName != "Our team" {
		t.Errorf("the repin replaced the display name: %q", m.DisplayName)
	}

	// The rollback is the same call with the previous version.
	if _, err := s.RepinSolution(ctx, mktapi.RepinSolutionInput{UniverseHome: "team", Version: "1"}); err != nil {
		t.Fatal(err)
	}
	if !wire.PinsVersion(team, "notes", "1.0") || solutionOf(t, team) != "duo@1" {
		t.Fatal("the rollback did not repin version 1")
	}
}

func TestAUniverseNotInstalledFromASolutionIsNotRepinned(t *testing.T) {
	s := repinFixture(t)
	home := filepath.Join(t.TempDir(), "plain")
	if _, err := wire.CreateUniverse(nil, wire.Display{}, home); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RepinSolution(context.Background(), mktapi.RepinSolutionInput{UniverseHome: home}); err == nil || !strings.Contains(err.Error(), "not installed from a solution") {
		t.Fatalf("err = %v", err)
	}
}
