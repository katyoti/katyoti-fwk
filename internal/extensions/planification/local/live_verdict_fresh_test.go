// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A LIVE VERDICT COUNTS ONLY WHEN NEWER THAN ITS SCRIPT:
// a passed verdict recorded before the last commit touching the walk's script
// does not verify the deliverable - it waits on the batch; one recorded after
// it does. The walks would fail if they were run (they print nothing a check
// accepts): verified means counted from the verdict.
func TestALiveVerdictCountsOnlyWhenNewerThanItsScript(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is missing")
	}
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "The invoice is marked")
	const (
		freshWalk = "app/ktw-gui/frontend/tests/live/invoice-fresh-walk.sh"
		staleWalk = "app/ktw-gui/frontend/tests/live/invoice-stale-walk.sh"
	)
	cFresh := criterionFor(t, ctx, cwm, "billing", f, "bash "+freshWalk)
	cStale := criterionFor(t, ctx, cwm, "billing", f, "bash "+staleWalk)

	// The project is a git repository whose last commit on both walks was
	// made on 2026-01-01.
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: cFresh})
	if err != nil || res.ProjectRoot == "" {
		t.Fatalf("resolve %s: %v %+v", cFresh, err, res)
	}
	for _, w := range []string{freshWalk, staleWalk} {
		p := filepath.Join(res.ProjectRoot, filepath.FromSlash(w))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("exit 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", res.ProjectRoot, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "--quiet")
	git("add", "--", "app")
	git("commit", "--quiet", "-m", "the walks", "--", "app")

	verdictAt(t, ctx, cwm, cFresh, "pass", "2026-01-02T00:00:00Z")
	verdictAt(t, ctx, cwm, cStale, "pass", "2025-12-31T00:00:00Z")
	r := file(t, ctx, req, api.FileInput{Title: "Mark the invoice", Module: "billing", Kind: api.KindFix, By: person,
		Deliverables: []api.Deliverable{
			{Title: "it builds", Check: "go version"},
			{Title: "the fresh walk passed", Check: "bash " + freshWalk},
			{Title: "the stale walk passed", Check: "bash " + staleWalk},
		}})
	got, err := req.Check(ctx, api.CheckInput{ID: r.ID, By: session})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 3 {
		t.Fatalf("results %+v; want three", got.Results)
	}
	if d := got.Request.Deliverables[1]; !got.Results[1].Passed || !d.Verified || !strings.Contains(d.Evidence, cFresh) {
		t.Fatalf("the walk passed after its script's last commit: %+v (%+v); want it counted from %s", d, got.Results[1], cFresh)
	}
	if res := got.Results[2]; res.Passed || got.Request.Deliverables[2].Verified || !strings.Contains(res.Error, "counted by the final GUI batch") {
		t.Fatalf("the walk passed before its script's last commit: %+v; want it left to the batch", res)
	}
}
