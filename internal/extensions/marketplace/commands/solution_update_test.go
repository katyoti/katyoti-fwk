// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// : an installed solution updates in one action - running work
// refused, the data backed up, repinned, started, migrated (dry run first),
// smoke-tested - and rolls back to the previous version on a failure.

// fakeUpdater records each step; failAt makes one fail.
type fakeUpdater struct {
	steps   []string
	running string
	failAt  string // the step that fails, once
	failed  bool
}

func (f *fakeUpdater) step(s string) error {
	f.steps = append(f.steps, s)
	if s == f.failAt && !f.failed {
		f.failed = true
		return errors.New("forced")
	}
	return nil
}

func (f *fakeUpdater) Repin(_ context.Context, v string, dry bool) (mktapi.RepinSolutionOutput, error) {
	out := mktapi.RepinSolutionOutput{Solution: "acme", From: "1.004", To: "1.005",
		Members: []mktapi.MemberMove{{Name: "ktw-agents", From: "1.004", To: "1.005"}}}
	if v == "1.004" {
		out.To = "1.004"
	}
	if dry {
		return out, f.step("plan " + out.To)
	}
	return out, f.step("repin " + v)
}
func (f *fakeUpdater) RunningWork(context.Context) (string, error) {
	return f.running, f.step("running?")
}
func (f *fakeUpdater) Stop(context.Context) error  { return f.step("stop") }
func (f *fakeUpdater) Backup() (string, error)     { return "/backup", f.step("backup") }
func (f *fakeUpdater) Restore(d string) error      { return f.step("restore " + d) }
func (f *fakeUpdater) Start(context.Context) error { return f.step("start") }
func (f *fakeUpdater) Migrate(_ context.Context, dry bool) (string, error) {
	if dry {
		return "data version 1 → 2", f.step("migrate dry")
	}
	return "data version 1 → 2", f.step("migrate")
}
func (f *fakeUpdater) Smoke(context.Context, mktapi.RepinSolutionOutput) error {
	return f.step("smoke")
}

func TestAnUpdateRunsItsStepsInOrder(t *testing.T) {
	f := &fakeUpdater{}
	if _, err := updateSolution(context.Background(), f, "", false, io.Discard); err != nil {
		t.Fatal(err)
	}
	want := []string{"plan 1.005", "running?", "stop", "backup", "repin 1.005", "start", "migrate dry", "migrate", "smoke"}
	if !slices.Equal(f.steps, want) {
		t.Fatalf("steps = %v\nwant    %v", f.steps, want)
	}
}

func TestAnUpdateWithWorkRunningIsRefusedBeforeAnything(t *testing.T) {
	f := &fakeUpdater{running: "2 task run(s)"}
	_, err := updateSolution(context.Background(), f, "", false, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "work is running (2 task run(s))") {
		t.Fatalf("err = %v", err)
	}
	if slices.Contains(f.steps, "stop") {
		t.Fatalf("a universe with work running was stopped: %v", f.steps)
	}
}

func TestADryRunChangesNothing(t *testing.T) {
	f := &fakeUpdater{}
	if _, err := updateSolution(context.Background(), f, "", true, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.steps, []string{"plan 1.005", "running?"}) {
		t.Fatalf("a dry run did %v", f.steps)
	}
}

func TestAFailureAfterTheBackupRollsBack(t *testing.T) {
	for _, at := range []string{"repin 1.005", "start", "migrate dry", "migrate", "smoke"} {
		f := &fakeUpdater{failAt: at}
		_, err := updateSolution(context.Background(), f, "", false, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "rolled back to 1.004, the data restored") {
			t.Errorf("failing at %s: err = %v", at, err)
			continue
		}
		i := slices.Index(f.steps, at)
		rollback := f.steps[i+1:]
		if want := []string{"stop", "restore /backup", "repin 1.004", "start"}; !slices.Equal(rollback, want) {
			t.Errorf("failing at %s: rollback = %v, want %v", at, rollback, want)
		}
		if slices.Contains(rollback, "migrate") {
			t.Errorf("failing at %s: migrated after the failure", at)
		}
	}
}

func TestAFailedBackupChangesNothing(t *testing.T) {
	f := &fakeUpdater{failAt: "backup"}
	if _, err := updateSolution(context.Background(), f, "", false, io.Discard); err == nil || !strings.Contains(err.Error(), "nothing changed") {
		t.Fatalf("err = %v", err)
	}
	if slices.ContainsFunc(f.steps, func(s string) bool { return strings.HasPrefix(s, "repin") }) {
		t.Fatalf("repinned without a backup: %v", f.steps)
	}
}

func TestTheSameVersionIsNotUpdated(t *testing.T) {
	f := &fakeUpdater{}
	if _, err := updateSolution(context.Background(), f, "1.004", false, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.steps, []string{"plan 1.004"}) {
		t.Fatalf("an update to the version in place did %v", f.steps)
	}
}

// The backup and the restore, on a real project: what the migration changed
// and added is gone, what it removed is back.
func TestTheProjectIsRestoredFromItsBackup(t *testing.T) {
	project := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(project, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("project.cwm.yaml", "id: p\ntype: project\ncapabilities:\n  project:\n    data_version: 1\n")
	write("modules/m/objects/m.cwm.yaml", "id: m\ntype: module\n")
	write("modules/m/features/F-001.cwm.yaml", "id: m.F-001\nwant: {reach: closable}\n")
	before := treeOf(t, project)

	backup := filepath.Join(t.TempDir(), "backup")
	if err := backupProject(project, backup); err != nil {
		t.Fatal(err)
	}
	write("project.cwm.yaml", "id: p\ntype: project\ncapabilities:\n  project:\n    data_version: 2\n")
	write("modules/m/features/F-001.cwm.yaml", "id: m.F-001\nwant: {reachability: closable}\n")
	write("modules/m/features/F-002.cwm.yaml", "id: m.F-002\n")
	_ = os.Remove(filepath.Join(project, "modules/m/objects/m.cwm.yaml"))

	if err := restoreProject(project, backup); err != nil {
		t.Fatal(err)
	}
	if after := treeOf(t, project); !mapsEq(before, after) {
		t.Fatalf("restored project differs:\nbefore %v\nafter  %v", before, after)
	}
}

func treeOf(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(root, p)
			out[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	return out
}

func mapsEq(a, b map[string]string) bool {
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
