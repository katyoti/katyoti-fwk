// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : a project's data carries its data version; a binary refuses data
// newer than it knows and `cwm migrate` brings older data to its own version
// through ordered steps, with a dry run listing every object it would change.

// twoSteps are a binary two versions past the baseline: the first renames a
// capability field, the second derives a field from the renamed one - so it
// only works if the first ran before it.
func twoSteps() []MigrationStep {
	return []MigrationStep{
		{Title: "want.reach becomes want.reachability", Object: func(o *api.Object) (bool, error) {
			g, ok := o.Capability("want")
			if !ok {
				return false, nil
			}
			v, had := g["reach"]
			if !had {
				return false, nil
			}
			next := map[string]any{"reachability": v}
			for k, x := range g {
				if k != "reach" {
					next[k] = x
				}
			}
			return true, o.SetCapability("want", next)
		}},
		{Title: "want.closable from want.reachability", Object: func(o *api.Object) (bool, error) {
			g, ok := o.Capability("want")
			if !ok {
				return false, nil
			}
			if _, done := g["closable"]; done {
				return false, nil
			}
			r, has := g["reachability"]
			if !has {
				return false, nil
			}
			next := map[string]any{"closable": r == "closable"}
			for k, x := range g {
				next[k] = x
			}
			return true, o.SetCapability("want", next)
		}},
	}
}

// migrationFixture is a project at the baseline with two features in the old
// shape and one object no step touches.
func migrationFixture(t *testing.T) (*Service, string) {
	t.Helper()
	svc, project := projectFixture(t, nil)
	// A binary with no step of its own: the project is minted at the
	// baseline whatever steps this build ships.
	svc.steps = nil
	ctx := stdctx.Background()
	if _, err := svc.ProjectInit(ctx, api.ProjectInitInput{Name: "acme"}); err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	if f, _ := projectfile.Read(project); f.DataVersion != projectfile.BaselineDataVersion {
		t.Fatalf("a new project is at data version %d, want the binary's %d", f.DataVersion, projectfile.BaselineDataVersion)
	}
	writeModuleObject(t, project, api.DefaultModulesDir, "m")
	dir := filepath.Join(project, api.DefaultModulesDir, "m", ObjectsDirName)
	for _, id := range []string{"m.F-001", "m.F-002"} {
		o := api.Object{ID: id, Type: "feature", Name: id, Parent: "m",
			Capabilities: map[string]map[string]any{"want": {"reach": "closable"}}}
		if err := WriteObjectFile(filepath.Join(dir, id+ObjectFileSuffix), o); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	return svc, project
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			out[p] = string(b)
		}
		return nil
	})
	return out
}

func TestMigrateDryRunThenApplyInOrderThenNothing(t *testing.T) {
	svc, project := migrationFixture(t)
	svc.steps = twoSteps()
	ctx := stdctx.Background()

	before := snapshot(t, project)
	dry, err := svc.Migrate(ctx, api.MigrateInput{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dry.From != 1 || dry.To != 3 || len(dry.Steps) != 2 {
		t.Fatalf("dry run = from %d to %d, %d steps; want 1 to 3, 2 steps", dry.From, dry.To, len(dry.Steps))
	}
	for i, st := range dry.Steps {
		if len(st.Changed) != 2 {
			t.Errorf("step %d (%s) would change %v; want both features", i, st.Title, st.Changed)
		}
	}
	if after := snapshot(t, project); !mapsEqual(before, after) {
		t.Fatal("the dry run wrote to the project")
	}

	out, err := svc.Migrate(ctx, api.MigrateInput{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if out.To != 3 || len(out.Steps) != 2 {
		t.Fatalf("migrate = %+v", out)
	}
	if f, _ := projectfile.Read(project); f.DataVersion != 3 {
		t.Fatalf("data version after = %d, want 3", f.DataVersion)
	}
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.F-001"})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := got.Object.Capability("want")
	if _, old := want["reach"]; old || want["reachability"] != "closable" || want["closable"] != true {
		t.Fatalf("m.F-001 want = %v; want reachability closable and closable true (the steps in order)", want)
	}
	module, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m"})
	if module.Object.ID != "m" {
		t.Fatal("the module object is gone after the migration")
	}

	again := snapshot(t, project)
	second, err := svc.Migrate(ctx, api.MigrateInput{})
	if err != nil || len(second.Steps) != 0 || second.From != 3 {
		t.Fatalf("second run = %+v, %v; want nothing to do at 3", second, err)
	}
	if after := snapshot(t, project); !mapsEqual(again, after) {
		t.Fatal("a second migration changed the project")
	}
}

func TestOlderBinaryRefusesNewerData(t *testing.T) {
	svc, project := migrationFixture(t)
	// The data is written one step past what the shipped binary knows,
	// whatever it ships.
	newer := twoSteps()
	for len(newer) <= len(migrationSteps) {
		newer = append(newer, MigrationStep{Title: "a later step", Object: func(*api.Object) (bool, error) { return false, nil }})
	}
	svc.steps = newer
	if _, err := svc.Migrate(stdctx.Background(), api.MigrateInput{}); err != nil {
		t.Fatal(err)
	}
	version := fmt.Sprintf("version %d", dataVersionOf(newer))
	// This binary ships no step: it knows the baseline only.
	err := refuseNewerData(project, migrationSteps[:0])
	if err == nil || !errors.Is(err, api.ErrProjectRefused) || !strings.Contains(err.Error(), version) {
		t.Fatalf("refuseNewerData = %v; want a refusal naming %s", err, version)
	}
	// The service starts and refuses the project at opening.
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: project}
	s, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New on data newer than the binary: %v; want the service started, the project refused", err)
	}
	_, err = s.ObjectTree(stdctx.Background(), api.ObjectTreeInput{})
	_ = s.Close()
	if !errors.Is(err, api.ErrProjectRefused) || !strings.Contains(err.Error(), version) {
		t.Fatalf("ObjectTree on data newer than the binary = %v; want the refusal naming %s", err, version)
	}
	// The binary that wrote it opens it.
	if err := refuseNewerData(project, newer); err != nil {
		t.Fatalf("the binary at the data's version refused it: %v", err)
	}
	// A binary older than its data cannot migrate it down either.
	svc.steps = nil
	if _, err := svc.Migrate(stdctx.Background(), api.MigrateInput{DryRun: true}); err == nil {
		t.Fatal("Migrate ran on data newer than the binary")
	}
}

func TestShippedStepsStartAtTheBaseline(t *testing.T) {
	if got := dataVersionOf(migrationSteps); got < projectfile.BaselineDataVersion {
		t.Fatalf("data version %d below the baseline", got)
	}
	for i, st := range migrationSteps {
		if strings.TrimSpace(st.Title) == "" || (st.Object == nil && st.Corpus == nil && st.Project == nil) {
			t.Errorf("step %d (to version %d) has no title or no Object, Corpus nor Project func", i, projectfile.BaselineDataVersion+i+1)
		}
	}
	if slices.ContainsFunc(migrationSteps, func(s MigrationStep) bool { return s.Object == nil && s.Corpus == nil && s.Project == nil }) {
		t.Fatal("a shipped step changes nothing")
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// A project that already holds objects when its project object is minted (a
// seed, a repository from before data versions) is at the baseline: it records
// no data version, so a newer binary migrates it instead of claiming it current.
func TestAProjectWithObjectsIsMintedAtTheBaseline(t *testing.T) {
	svc, project := projectFixture(t, nil)
	svc.steps = twoSteps()
	writeModuleObject(t, project, api.DefaultModulesDir, "seeded")
	if _, err := svc.ObjectScan(stdctx.Background(), api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ProjectInit(stdctx.Background(), api.ProjectInitInput{Name: "seeded"}); err != nil {
		t.Fatal(err)
	}
	if f, _ := projectfile.Read(project); f.DataVersion != projectfile.BaselineDataVersion {
		t.Fatalf("a seeded project was minted at data version %d, want the baseline", f.DataVersion)
	}
	out, err := svc.Migrate(stdctx.Background(), api.MigrateInput{DryRun: true})
	if err != nil || out.From != 1 || out.To != 3 {
		t.Fatalf("its migration = %+v, %v; want 1 to 3", out, err)
	}
}

// 8 → 9: a plan written by the old plan-step model
// loses next, planned_at and run, a running plan becomes ready, and the rest
// of the plan stays; applied twice, the step changes nothing more.
func TestDropPlanStepRemovesTheOldPlanStepModel(t *testing.T) {
	o := &api.Object{ID: "m.RQ-001", Type: "request", Capabilities: map[string]map[string]any{
		"plan": {"state": "running", "priority": "P1", "next": map[string]any{"task": "intake"},
			"planned_at": "2026-10-08T05:49:32Z", "run": "r1", "steps": []any{map[string]any{"task": "intake"}}},
		"request": {"kind": "build"},
	}}
	changed, err := dropPlanStep(o)
	if err != nil || !changed {
		t.Fatalf("dropPlanStep = %v, %v; want changed", changed, err)
	}
	plan := o.Capabilities["plan"]
	for _, k := range []string{"next", "planned_at", "run"} {
		if _, has := plan[k]; has {
			t.Errorf("plan still carries %s: %v", k, plan)
		}
	}
	if plan["state"] != "ready" || plan["priority"] != "P1" || plan["steps"] == nil || o.Capabilities["request"]["kind"] != "build" {
		t.Errorf("the rest of the object changed: %v", o.Capabilities)
	}
	if changed, _ := dropPlanStep(o); changed {
		t.Errorf("applied twice, the step changed the object again")
	}
	if changed, _ := dropPlanStep(&api.Object{ID: "m.F-001", Type: "feature"}); changed {
		t.Errorf("an object with no plan was changed")
	}
}
