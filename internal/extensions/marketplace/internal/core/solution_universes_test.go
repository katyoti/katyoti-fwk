// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/prereqs"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/registry"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/store"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// solutionFixture is a scoped katyoti root (never the development universe)
// whose one marketplace offers the content items notes and cards, already in
// the content store, and the solution versions given as catalog YAML.
func solutionFixture(t *testing.T, solutionVersions string) *service {
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

	for _, name := range []string{"notes", "cards"} {
		folder, err := store.ContentFolderPath(name, "1.0")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "ktw-host.yaml"), []byte("name: "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	content := func(name string) string {
		return "  - name: " + name + "\n    kind: content\n    versions:\n      - version: \"1.0\"\n        assets:\n          - url: " + name + ".tar.gz\n"
	}
	cat := "apiVersion: 1\nmarketplace:\n  name: m\nitems:\n" + content("notes") + content("cards") +
		"  - name: duo\n    kind: solution\n    versions:\n" + solutionVersions
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

// registered is every universe of the registry, by name → home.
func registered(t *testing.T) map[string]string {
	t.Helper()
	regPath, err := orchcatalog.DefaultRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := orchcatalog.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range reg.List() {
		out[e.Name] = e.HomePath
	}
	return out
}

// members are the sidecars a universe's Univers.yaml wires, sorted.
func members(t *testing.T, home string) []string {
	t.Helper()
	m, err := orchcatalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, sc := range m.Sidecars {
		out = append(out, sc.Name)
	}
	sort.Strings(out)
	return out
}

// A solution of two universes installs both: the main one under the universe
// named, the other beside it as <name>-<its name>, each registered with its
// own members, and the install names the main one — the universe that opens
// (katyoti.yml default_universe, through setup).
func TestASolutionOfTwoUniversesInstallsBothAndNamesTheMain(t *testing.T) {
	s := solutionFixture(t, `      - version: "1"
        includes:
          - {name: notes, version: "1.0"}
          - {name: cards, version: "1.0"}
        universes:
          - name: review
            displayName: Review
            sidecars: [{name: cards}]
          - name: studio
            main: true
            displayName: Studio
            sidecars: [{name: notes}]
`)
	out, err := s.InstallSolution(context.Background(), mktapi.InstallSolutionInput{Ref: mktapi.ItemRef{Name: "duo"}, UniverseHome: "team"})
	if err != nil {
		t.Fatal(err)
	}
	reg := registered(t)
	team, review := reg["team"], reg["team-review"]
	if team == "" || review == "" {
		t.Fatalf("registry %v: want team and team-review", reg)
	}
	if out.Main != "team" || out.Home != team {
		t.Errorf("main %q at %q; want team at %q", out.Main, out.Home, team)
	}
	if len(out.Universes) != 2 || !out.Universes[0].Main || out.Universes[1].Name != "team-review" {
		t.Errorf("universes %+v; want team (main) then team-review", out.Universes)
	}
	if got := members(t, team); strings.Join(got, ",") != "notes" {
		t.Errorf("team wires %v, want notes", got)
	}
	if got := members(t, review); strings.Join(got, ",") != "cards" {
		t.Errorf("team-review wires %v, want cards", got)
	}
	if filepath.Dir(review) != filepath.Dir(team) {
		t.Errorf("team-review at %s, want beside team (%s)", review, team)
	}
}

// A solution of one universe (an older catalog's single `universe:`)
// installs exactly as before: one universe, under the name given, with every
// member, and it is the main one.
func TestASolutionOfOneUniverseInstallsAsBefore(t *testing.T) {
	s := solutionFixture(t, `      - version: "1"
        includes:
          - {name: notes, version: "1.0"}
          - {name: cards, version: "1.0"}
        universe:
          displayName: Duo
          sidecars: [{name: notes}, {name: cards}]
`)
	out, err := s.InstallSolution(context.Background(), mktapi.InstallSolutionInput{Ref: mktapi.ItemRef{Name: "duo"}, UniverseHome: "solo"})
	if err != nil {
		t.Fatal(err)
	}
	reg := registered(t)
	if len(reg) != 1 || reg["solo"] == "" {
		t.Fatalf("registry %v: want solo alone", reg)
	}
	if want := filepath.Join(application.Env().GlobalHomeDir(), "universes", "solo"); reg["solo"] != want || out.Home != want {
		t.Errorf("solo at %q (out %q), want %q", reg["solo"], out.Home, want)
	}
	if got := members(t, reg["solo"]); strings.Join(got, ",") != "cards,notes" {
		t.Errorf("solo wires %v, want every member", got)
	}
	if out.Main != "solo" || len(out.Universes) != 1 || !out.Universes[0].Main {
		t.Errorf("main %q, universes %+v; want solo, the one main", out.Main, out.Universes)
	}
}

// A solution whose universes are not exactly one main is refused before
// anything installs.
func TestASolutionWithoutExactlyOneMainIsRefused(t *testing.T) {
	s := solutionFixture(t, `      - version: "1"
        includes:
          - {name: notes, version: "1.0"}
          - {name: cards, version: "1.0"}
        universes:
          - {name: a, main: true, sidecars: [{name: notes}]}
          - {name: b, main: true, sidecars: [{name: cards}]}
`)
	_, err := s.InstallSolution(context.Background(), mktapi.InstallSolutionInput{Ref: mktapi.ItemRef{Name: "duo"}, UniverseHome: "two"})
	if err == nil || !strings.Contains(err.Error(), "exactly one main universe") {
		t.Fatalf("err = %v, want the one-main refusal", err)
	}
	if reg := registered(t); len(reg) != 0 {
		t.Errorf("a refused solution registered %v", reg)
	}
}
