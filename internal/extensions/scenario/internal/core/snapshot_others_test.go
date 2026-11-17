// SPDX-License-Identifier: Apache-2.0

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// A case declares its other universes beside its own: each named once, with
// members, its project a folder (never a repository); the rest is refused.
func TestACaseDeclaresItsOtherUniverses(t *testing.T) {
	dir := t.TempDir()
	load := func(body string) (*api.Case, error) {
		d := filepath.Join(dir, "c")
		_ = os.MkdirAll(d, 0o755)
		_ = os.WriteFile(filepath.Join(d, api.CaseFile), []byte(body), 0o644)
		return api.Load(d)
	}
	c, err := load(`name: duo
universe: {sidecars: [ktw-agents]}
other_universes:
  - name: review
    sidecars: [ktw-kb]
    copy: [{from: review/config, to: config/}]
    project: {from: review/project}
    display_name: Relecture
    icon: R
    gui: {hide: [runs]}
    tags: [review]
`)
	if err != nil {
		t.Fatal(err)
	}
	o := c.OtherUniverses
	if len(o) != 1 || o[0].Name != "review" || o[0].Sidecars[0] != "ktw-kb" || o[0].Project.From != "review/project" ||
		o[0].DisplayName != "Relecture" || o[0].GUI == nil || o[0].Tags[0] != "review" || len(o[0].Copy) != 1 {
		t.Fatalf("other universes %+v", o)
	}
	if c.OtherUniverseName("review") != "scn-duo-review" {
		t.Errorf("registered as %s, want scn-duo-review", c.OtherUniverseName("review"))
	}
	for want, body := range map[string]string{
		"a name":           "name: x\nuniverse: {sidecars: [a]}\nother_universes: [{sidecars: [a]}]\n",
		"named twice":      "name: x\nuniverse: {sidecars: [a]}\nother_universes: [{name: r, sidecars: [a]}, {name: R, sidecars: [a]}]\n",
		"at least one":     "name: x\nuniverse: {sidecars: [a]}\nother_universes: [{name: r}]\n",
		"not a repository": "name: x\nuniverse: {sidecars: [a]}\nother_universes: [{name: r, sidecars: [a], project: {from: p, git: true}}]\n",
		"stay inside":      "name: x\nuniverse: {sidecars: [a]}\nother_universes: [{name: r, sidecars: [a], copy: [{from: f, to: ../x}]}]\n",
	} {
		if _, err := load(body); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want an error naming %q, got %v", want, err)
		}
	}
}

// A case of two universes is published as a solution of two: its own the main
// one, the other beside it with its display, its config-sets and its project
// as its seed (with that universe's own tasks, never the simulated user's).
// A tagged universe is published without gui.hide (its tag decides what it
// shows); the rest of its gui is the case's.
func TestASnapshotPublishesTheCaseOtherUniverses(t *testing.T) {
	caseDir := t.TempDir()
	write(t, filepath.Join(caseDir, "review", "project", "README.md"), "# Relecture\n")
	gui := &cenv.UniverseGUI{Hide: []string{"runs"}, First: []string{"guide"}}
	published := &cenv.UniverseGUI{First: []string{"guide"}}
	c := &api.Case{Name: "duo", Dir: caseDir,
		Universe: api.CaseUniverse{Sidecars: []string{"ktw-a"}, DisplayName: "Studio"},
		OtherUniverses: []api.CaseOtherUniverse{{Name: "review", Sidecars: []string{"ktw-b"},
			Project: api.CaseProject{From: "review/project"}, DisplayName: "Relecture", Icon: "R", GUI: gui, Tags: []string{"review"}}}}
	e := NewEngine(c, t.TempDir(), "", io.Discard)

	// The case as install leaves it: each universe's member a copy in bin/.
	var items []mktapi.Item
	wire := func(home, name, member string) {
		bin := filepath.Join(e.RT.Bin(), member+".exe")
		write(t, bin, "bytes of "+member)
		sum := sha256.Sum256([]byte("bytes of " + member))
		items = append(items, mktapi.Item{Name: member, Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{Version: "1.0.0",
			Assets: []mktapi.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "x", Sha256: hex.EncodeToString(sum[:])}}}}})
		if err := catalog.SaveManifest(home, &catalog.Manifest{Name: name,
			Sidecars: []orchapi.SidecarSpec{{Name: member, Transport: "nats", Binary: bin}}}); err != nil {
			t.Fatal(err)
		}
	}
	wire(e.RT.Universe(), "scn-duo", "ktw-a")
	wire(e.RT.OtherUniverse("review"), "scn-duo-review", "ktw-b")
	tasks := filepath.Join(application.EnvOf(e.RT.OtherUniverse("review")).HomeDir(), "tasks")
	write(t, filepath.Join(tasks, SimUserTask+".md"), "---\nname: scenario-user\n---\n")
	write(t, filepath.Join(tasks, "relire.md"), "---\nname: relire\n---\n")

	market := t.TempDir()
	catalogPath := filepath.Join(market, "katyoti-marketplace.yaml")
	data, err := index.MarshalCatalog(mktapi.Catalog{APIVersion: 1, Marketplace: mktapi.CatalogMeta{Name: "m"}, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	write(t, catalogPath, string(data))

	if _, err := e.Snapshot(SnapshotOptions{Version: "v1", Marketplace: catalogPath,
		OtherConfigSets: map[string][]string{"review": {"review-base@v1"}}}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatal(err)
	}
	var cat mktapi.Catalog
	if err := config.DecodeBytes(raw, marshal.YAML, &cat); err != nil {
		t.Fatal(err)
	}
	var sol mktapi.ItemVersion
	for _, it := range cat.Items {
		if it.Name == "duo" {
			sol = it.Versions[0]
		}
	}
	us := sol.SolutionUniverses()
	if len(us) != 2 || !us[0].Main || us[0].Name != "scn-duo" || us[0].DisplayName != "Studio" || us[1].Main || us[1].Name != "review" {
		t.Fatalf("universes %+v; want scn-duo (main, Studio) and review", us)
	}
	r := us[1]
	if r.DisplayName != "Relecture" || r.Icon != "R" || !reflect.DeepEqual(r.GUI, published) || !reflect.DeepEqual(r.Tags, []string{"review"}) {
		t.Errorf("review's display %q %q %+v %v; want the case's", r.DisplayName, r.Icon, r.GUI, r.Tags)
	}
	if len(r.ConfigSets) != 1 || r.ConfigSets[0] != (mktapi.PinnedRef{Name: "review-base", Version: "v1"}) {
		t.Errorf("review's config-sets %+v; want review-base@v1", r.ConfigSets)
	}
	if r.Seed == nil || r.Seed.Name != "duo-review-seed" {
		t.Fatalf("review's seed %+v; want duo-review-seed", r.Seed)
	}
	files := seedFiles(t, filepath.Join(market, filepath.FromSlash(index.SeedAssetPath("v1", "duo-review-seed"))))
	if !files["universe/tasks/relire.md"] || !files["project/README.md"] {
		t.Errorf("review's seed %v; want its project and its own task", files)
	}
	for f := range files {
		if strings.Contains(f, SimUserTask) {
			t.Errorf("review's seed publishes the simulated user's task: %s", f)
		}
	}

	// A config-set for a universe the case does not declare is refused.
	if _, err := e.Snapshot(SnapshotOptions{Version: "v2", Marketplace: catalogPath,
		OtherConfigSets: map[string][]string{"nope": {"x@v2"}}}); err == nil || !strings.Contains(err.Error(), "does not declare") {
		t.Errorf("err = %v, want the undeclared universe refused", err)
	}
}

// A case's binaries are the hub and every member of every one of its
// universes, each once.
func TestTheCaseBinariesAreEveryUniverseMembersOnce(t *testing.T) {
	e := NewEngine(&api.Case{Name: "duo", Universe: api.CaseUniverse{Sidecars: []string{"ktw-agents", "ktw-kb"}},
		OtherUniverses: []api.CaseOtherUniverse{{Name: "review", Sidecars: []string{"ktw-kb", "ktw-docs"}}}}, t.TempDir(), "", io.Discard)
	if got := strings.Join(e.caseBinaries(), ","); got != hubBinary+",ktw-agents,ktw-kb,ktw-docs" {
		t.Errorf("binaries %s; want the hub, then each member once", got)
	}
}

// A checkpoint holds the case's other universes: a restore puts them back.
func TestACheckpointKeepsTheOtherUniverses(t *testing.T) {
	e := NewEngine(&api.Case{Name: "duo", Stages: []api.Stage{{ID: "s1"}}}, t.TempDir(), "", io.Discard)
	file := filepath.Join(e.RT.OtherUniverse("review"), "config", "Settings.yaml")
	write(t, file, "v1")
	if err := e.Checkpoint(t.Context(), CheckpointInstalled); err != nil {
		t.Fatal(err)
	}
	write(t, file, "v2")
	if err := e.Restore(t.Context(), CheckpointInstalled); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); string(b) != "v1" {
		t.Errorf("after restore the other universe holds %q, want v1", b)
	}
}
