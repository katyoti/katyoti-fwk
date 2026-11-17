// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os/exec"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// : in a project kept in git, a module-update report is
// not collected while the acceptance check or the change-log entry its module
// update records is untracked - the report names the file to commit; once the
// files are in git (staged is enough), the same report is collected.
func TestAModuleUpdateIsCollectedOnlyWithWhatItRecordsInGit(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	ctx, cwm, req := gatedWorld(t, "2020-01-01")
	uc := want(t, ctx, cwm, "use-case", "Send invoices by mail")
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices by mail", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Serves: uc, By: session}); err != nil {
		t.Fatal(err)
	}
	ac := criterion(t, ctx, cwm, uc, "A mail leaves for each invoice")
	rel, _ := changeLogRelease(t, ctx)
	entry, err := changesOf(t, ctx).Add(ctx, api.AddChangeInput{Release: rel, Title: "Les factures partent par mail", Domain: "Interface", Value: api.ValueMajor, Text: "x", Requests: []string{r.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := req.ModuleUpdate(ctx, api.ModuleUpdateInput{ID: r.ID, Changelog: entry.Change.ID, Checks: []string{ac}, By: session}); err != nil {
		t.Fatal(err)
	}

	// The project is a git repository; the criterion and the entry are not
	// in it.
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: ac})
	if err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", res.ProjectRoot}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "--quiet")

	mu := stepOf(t, ctx, r.ID, "module-update")
	s := schedOf(t, ctx)
	report := func() error {
		t.Helper()
		if got, err := s.Pull(ctx, schedapi.PullInput{ID: mu.ID, By: session}); err != nil || got.Todo == nil {
			t.Fatalf("pull the module update: %v %+v", err, got)
		}
		_, err := s.Report(ctx, schedapi.ReportInput{ID: mu.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "module updated", By: session}})
		return err
	}
	err = report()
	if err == nil {
		t.Fatal("a module update whose criterion and entry are untracked in git: collected")
	}
	for _, named := range []string{"untracked in git", ac, entry.Change.ID} {
		if !strings.Contains(err.Error(), named) {
			t.Errorf("the refusal %q does not name %q", err, named)
		}
	}
	if out, _ := req.CheckModuleUpdated(ctx, api.ModuleUpdatedInput{ID: r.ID}); out.Accept || !strings.Contains(out.Reason, "untracked in git") {
		t.Errorf("the module-updated gate with untracked files: %+v; want refused, saying so", out)
	}

	git("add", "--", ".")
	if err := report(); err != nil {
		t.Fatalf("the module update once its files are in git: %v", err)
	}
}
