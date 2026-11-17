// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A PROJECT'S CONVERSION FROM THE GUI: the projects
// a universe knows say whether their data waits for a conversion, the dry run
// says where git stands on the files it writes, and migrate-commit converts
// as one commit of those files alone.

// TestKnownProjectsSayTheirConversion: an older project waits, one at the
// binary's version waits for nothing, a newer one is newer.
func TestKnownProjectsSayTheirConversion(t *testing.T) {
	svc, served := projectFixture(t, nil)
	svc.steps = twoSteps() // the binary is at data version 3
	folder := t.TempDir()
	svc.Config.ProjectsFolder = folder
	write := func(dir, id string, version int) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "id: " + id + "\ntype: software-project\nname: " + id + "\n"
		if version > 0 {
			body += "capabilities:\n  project:\n    data_version: " + string(rune('0'+version)) + "\n"
		}
		if err := os.WriteFile(filepath.Join(dir, api.ProjectFileName), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(served, "alpha", 0) // no data_version: the baseline, 1
	write(filepath.Join(folder, "beta"), "beta", 3)
	write(filepath.Join(folder, "gamma"), "gamma", 5)

	out, err := svc.ProjectsKnown(stdctx.Background(), api.ProjectsKnownInput{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]api.KnownProject{}
	for _, p := range out.Projects {
		got[p.ID] = p
	}
	for id, want := range map[string]struct {
		version    int
		conversion string
	}{"alpha": {1, api.ConversionWaiting}, "beta": {3, ""}, "gamma": {5, api.ConversionNewer}} {
		if p := got[id]; p.DataVersion != want.version || p.Conversion != want.conversion {
			t.Errorf("%s: data version %d, conversion %q; want %d, %q", id, p.DataVersion, p.Conversion, want.version, want.conversion)
		}
	}
}

// gitRun runs git in dir and fails the test on an error.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitcli.Run(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

// gitRepoAt makes dir a repository with an identity and no hook of the
// machine's, then commits the project's object files (its index aside).
func gitRepoAt(t *testing.T, dir, project string) {
	t.Helper()
	gitRun(t, dir, "init", "-q")
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "test")
	gitRun(t, dir, "config", "core.hooksPath", filepath.Join(dir, ".no-hooks"))
	gitRun(t, dir, "config", "core.autocrlf", "false")
	gitRun(t, project, "add", "--", api.ProjectFileName, api.DefaultModulesDir)
	gitRun(t, dir, "commit", "-q", "-m", "the project")
}

const featureOne = "modules/m/" + ObjectsDirName + "/m.F-001" + ObjectFileSuffix

// TestMigrateDryRunSaysTheTree: clean when every file the conversion writes is
// committed; a touched file modified or untracked is named; a dirty file the
// conversion does not touch is not; outside a repository, repo false. The
// project sits in a sub-folder of its repository (IN-086): paths stay the
// project's.
func TestMigrateDryRunSaysTheTree(t *testing.T) {
	ctx := stdctx.Background()
	svc, project := migrationFixture(t)
	svc.steps = twoSteps()

	outside, err := svc.Migrate(ctx, api.MigrateInput{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if outside.Tree == nil || outside.Tree.Repo || outside.Tree.Clean {
		t.Fatalf("outside a repository: tree %+v; want repo false", outside.Tree)
	}

	// The repository is the project's parent: the project is a sub-folder.
	repo := filepath.Dir(project)
	gitRepoAt(t, repo, project)
	clean, err := svc.Migrate(ctx, api.MigrateInput{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if clean.Tree == nil || !clean.Tree.Repo || !clean.Tree.Clean || len(clean.Tree.TouchedDirty) != 0 {
		t.Fatalf("committed: tree %+v; want repo and clean", clean.Tree)
	}

	// A file the conversion does not touch (the module object) is dirty:
	// still clean.
	module := filepath.Join(project, "modules", "m", ObjectsDirName, "m"+ObjectFileSuffix)
	appendTo(t, module, "# a hand note\n")
	if out, _ := svc.Migrate(ctx, api.MigrateInput{DryRun: true}); !out.Tree.Clean {
		t.Fatalf("an untouched dirty file: tree %+v; want clean", out.Tree)
	}

	// A touched file modified, another untracked: both named.
	appendTo(t, filepath.Join(project, filepath.FromSlash(featureOne)), "# a hand note\n")
	gitRun(t, project, "rm", "-q", "--cached", "--", "modules/m/"+ObjectsDirName+"/m.F-002"+ObjectFileSuffix)
	gitRun(t, repo, "commit", "-q", "-m", "F-002 untracked")
	dirty, err := svc.Migrate(ctx, api.MigrateInput{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{featureOne, "modules/m/" + ObjectsDirName + "/m.F-002" + ObjectFileSuffix}
	if dirty.Tree.Clean || !slices.Equal(dirty.Tree.TouchedDirty, want) {
		t.Fatalf("touched files dirty: tree %+v; want not clean, %v", dirty.Tree, want)
	}
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

// removeFeatureTwo is a step merging away: its file is deleted.
var removeFeatureTwo = MigrationStep{Title: "m.F-002 merges away", Corpus: func(objs []*api.Object) (changed, removed []int, err error) {
	for i, o := range objs {
		if o.ID == "m.F-002" {
			removed = append(removed, i)
		}
	}
	return nil, removed, nil
}}

// TestMigrateCommitIsOneCommit: the conversion is exactly one commit of the
// changed and removed files and the project object, its data version stamped;
// another's staged change stays out of it, still staged; refused writing
// nothing when a touched file is dirty or nothing waits.
func TestMigrateCommitIsOneCommit(t *testing.T) {
	ctx := stdctx.Background()
	svc, project := migrationFixture(t)
	svc.steps = append(twoSteps(), removeFeatureTwo)
	gitRepoAt(t, project, project)
	// Another's change, staged, beside the conversion.
	other := filepath.Join(project, "notes.txt")
	if err := os.WriteFile(other, []byte("another's work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, project, "add", "--", "notes.txt")

	// A touched file dirty: refused, nothing written.
	feature := filepath.Join(project, filepath.FromSlash(featureOne))
	keep, _ := os.ReadFile(feature)
	appendTo(t, feature, "# a hand note\n")
	before := snapshot(t, project)
	if _, err := svc.MigrateCommit(ctx, api.MigrateCommitInput{}); !errors.Is(err, api.ErrConflict) || !strings.Contains(err.Error(), featureOne) {
		t.Fatalf("a touched file dirty: %v; want a conflict naming %s", err, featureOne)
	}
	if after := snapshot(t, project); !mapsEqual(before, after) {
		t.Fatal("a refused migrate-commit wrote to the project")
	}
	if err := os.WriteFile(feature, keep, 0o644); err != nil {
		t.Fatal(err)
	}

	head := strings.TrimSpace(gitRun(t, project, "rev-parse", "HEAD"))
	out, err := svc.MigrateCommit(ctx, api.MigrateCommitInput{})
	if err != nil {
		t.Fatalf("migrate-commit: %v", err)
	}
	if out.Commit.Sha == "" || out.Commit.Reason != "" {
		t.Fatalf("commit %+v; want a sha", out.Commit)
	}
	if parent := strings.TrimSpace(gitRun(t, project, "rev-parse", out.Commit.Sha+"^")); parent != head {
		t.Fatalf("the conversion's parent is %s; want the HEAD before it %s (one commit)", parent, head)
	}
	if msg := strings.TrimSpace(gitRun(t, project, "log", "-1", "--format=%s")); msg != "Convert project data from version 1 to 4" {
		t.Errorf("message %q", msg)
	}
	files := gitcli.Lines(gitRun(t, project, "show", "--name-status", "--format=", "HEAD"))
	wantFiles := []string{
		"D\tmodules/m/" + ObjectsDirName + "/m.F-002" + ObjectFileSuffix,
		"M\t" + featureOne,
		"M\t" + api.ProjectFileName,
	}
	slices.Sort(files)
	slices.Sort(wantFiles)
	if !slices.Equal(files, wantFiles) {
		t.Fatalf("the commit holds %v; want %v", files, wantFiles)
	}
	if f, _ := projectfile.Read(project); f.DataVersion != 4 {
		t.Errorf("data version %d; want 4", f.DataVersion)
	}
	if staged := strings.TrimSpace(gitRun(t, project, "diff", "--cached", "--name-only")); staged != "notes.txt" {
		t.Errorf("staged after the commit %q; want another's notes.txt, still staged", staged)
	}
	if st := strings.TrimSpace(gitRun(t, project, "status", "--porcelain", "--", "modules", api.ProjectFileName)); st != "" {
		t.Errorf("the conversion left changes out of its commit: %q", st)
	}

	// Nothing waits now: refused, nothing written.
	before = snapshot(t, project)
	if _, err := svc.MigrateCommit(ctx, api.MigrateCommitInput{}); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("nothing waits: %v; want refused", err)
	}
	if after := snapshot(t, project); !mapsEqual(before, after) {
		t.Fatal("a refused migrate-commit wrote to the project")
	}
}

// collect is a progress emitter keeping each panel line.
type collect struct{ lines []panel.Progress }

func (c *collect) Emit(v any) {
	if p, ok := v.(panel.Progress); ok {
		c.lines = append(c.lines, p)
	}
}

// TestMigrateCommitReportsEachStep: each step goes into the panel the run
// lends, then the index rebuilt, then the commit - outside a repository, the
// reason no commit was made.
func TestMigrateCommitReportsEachStep(t *testing.T) {
	svc, _ := migrationFixture(t)
	svc.steps = twoSteps()
	var c collect
	out, err := svc.MigrateCommit(progress.WithEmitter(stdctx.Background(), &c), api.MigrateCommitInput{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Commit.Sha != "" || out.Commit.Reason == "" {
		t.Fatalf("outside a repository: commit %+v; want a reason, no sha", out.Commit)
	}
	if len(c.lines) != 4 {
		t.Fatalf("panel lines %+v; want 4 (two steps, the index, the commit)", c.lines)
	}
	for i, want := range []string{"1 to 2: want.reach becomes want.reachability", "2 to 3: want.closable from want.reachability", "index rebuilt", "not committed: "} {
		if l := c.lines[i]; !strings.HasPrefix(l.Label, want) || l.Current != int64(i+1) || l.Total != 4 {
			t.Errorf("line %d = %+v; want %q, %d of 4", i, l, want, i+1)
		}
	}
}

// TestARefusedCommitSaysTheHooksWords: a pre-commit hook refusing the
// conversion's commit leaves the data converted, no commit made, and the
// answer's reason carries the hook's words without git's line-ending
// warnings.
func TestARefusedCommitSaysTheHooksWords(t *testing.T) {
	ctx := stdctx.Background()
	svc, project := migrationFixture(t)
	svc.steps = twoSteps()
	gitRepoAt(t, project, project)
	hooks := filepath.Join(project, ".no-hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	hook := "#!/bin/sh\necho 'secret-scan: refused by the test hook' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	// The project's files are written with LF: git warns it will turn them to CRLF.
	gitRun(t, project, "config", "core.autocrlf", "true")
	head := strings.TrimSpace(gitRun(t, project, "rev-parse", "HEAD"))

	out, err := svc.MigrateCommit(ctx, api.MigrateCommitInput{})
	if err != nil {
		t.Fatalf("migrate-commit: %v", err)
	}
	if out.Commit.Sha != "" || !strings.Contains(out.Commit.Reason, "refused by the test hook") {
		t.Fatalf("commit %+v; want no sha and the hook's words", out.Commit)
	}
	if strings.Contains(out.Commit.Reason, "will be replaced by") {
		t.Errorf("the reason keeps git's line-ending warnings: %q", out.Commit.Reason)
	}
	if now := strings.TrimSpace(gitRun(t, project, "rev-parse", "HEAD")); now != head {
		t.Errorf("HEAD moved to %s; want no commit", now)
	}
	if f, _ := projectfile.Read(project); f.DataVersion != 3 {
		t.Errorf("data version %d; want 3 (converted, not committed)", f.DataVersion)
	}
}

// TestCommitRefusalDropsLineEndingWarnings: git's line-ending notes, in their
// old and new spellings, are cut from the reason; the rest stays.
func TestCommitRefusalDropsLineEndingWarnings(t *testing.T) {
	err := errors.New("git commit --only: exit status 1: warning: in the working copy of 'a.yaml', LF will be replaced by CRLF the next time Git touches it\n" +
		"warning: LF will be replaced by CRLF in b.yaml.\nThe file will have its original line endings in your working directory\n" +
		"secret-scan: refused")
	got := commitRefusal(err)
	if want := "git commit --only: exit status 1:\nsecret-scan: refused"; got != want {
		t.Errorf("commitRefusal = %q; want %q", got, want)
	}
}
