// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// : the export ref leaves the machine to the repository's
// private remote, with its own refspec - all on bare repositories of
// t.TempDir, never a real remote.

func bareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := gitcli.Run(dir, "init", "-q", "--bare"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func withRemote(t *testing.T, repoDir, remote string) {
	t.Helper()
	if _, err := gitcli.Run(repoDir, "remote", "add", "origin", remote); err != nil {
		t.Fatal(err)
	}
}

func remoteTip(t *testing.T, bare, ref string) string {
	t.Helper()
	tip, err := refTip(bare, ref)
	if err != nil {
		t.Fatal(err)
	}
	return tip
}

// The export pushes its ref to the private remote and adds the refspec; a
// fresh clone fetching it reads the to-dos, and todos import on that copy
// brings them into an empty store.
func TestTheExportRefIsPushedAndAFreshCloneImportsIt(t *testing.T) {
	ctx := context.Background()
	repoDir, bare := gitRepo(t), bareRemote(t)
	withRemote(t, repoDir, bare)
	rootsAre(t, map[string]string{"repo-1": repoDir})
	st := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, st)
	out, err := NewOver(st).Export(ctx, api.ExportInput{})
	if err != nil {
		t.Fatal(err)
	}
	r := out.Refs[0]
	if !r.Pushed || r.Remote != "origin" || r.PushError != "" {
		t.Fatalf("export = %+v, want pushed to origin", r)
	}
	if got := remoteTip(t, bare, ExportRef("repo-1")); got != r.Commit {
		t.Fatalf("remote holds %q, want %s", got, r.Commit)
	}
	specs, _ := gitcli.Run(repoDir, "config", "--get-all", "remote.origin.fetch")
	if !strings.Contains(specs, ExportRefSpec) {
		t.Fatalf("remote.origin.fetch = %q, want %s", specs, ExportRefSpec)
	}

	clone := t.TempDir()
	if _, err := gitcli.Run(clone, "clone", "-q", bare, "."); err != nil {
		t.Fatal(err)
	}
	if _, err := gitcli.Run(clone, "config", "--add", "remote.origin.fetch", ExportRefSpec); err != nil {
		t.Fatal(err)
	}
	if _, err := gitcli.Run(clone, "fetch", "-q"); err != nil {
		t.Fatal(err)
	}
	rootsAre(t, map[string]string{"repo-1": clone})
	fresh := openSQL(t, t.TempDir(), "repo-1")
	in, err := NewOver(fresh).Import(ctx, api.ImportInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Refs) != 1 || in.Refs[0].Commit != r.Commit || in.Refs[0].Todos == 0 || in.Refs[0].Todos != r.Todos {
		t.Fatalf("import on the clone = %+v, want the %d to-dos of %s", in.Refs, r.Todos, r.Commit)
	}
}

// A public remote is refused: nothing reaches it, the export itself succeeds
// and says why.
func TestAPublicRemoteIsRefused(t *testing.T) {
	ctx := context.Background()
	repoDir, bare := gitRepo(t), bareRemote(t)
	withRemote(t, repoDir, bare)
	was := remoteVisibility
	remoteVisibility = func(string, string) Visibility { return VisibilityPublic }
	t.Cleanup(func() { remoteVisibility = was })
	rootsAre(t, map[string]string{"repo-1": repoDir})
	st := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, st)
	out, err := NewOver(st).Export(ctx, api.ExportInput{})
	if err != nil {
		t.Fatal(err)
	}
	r := out.Refs[0]
	if r.Pushed || !strings.Contains(r.PushError, "public") || r.Commit == "" {
		t.Fatalf("export to a public remote = %+v, want written locally and refused", r)
	}
	if got := remoteTip(t, bare, ExportRef("repo-1")); got != "" {
		t.Fatalf("a public remote got the ref at %s", got)
	}
}

// Another clone pushed first: the push is rejected, the export is written
// again over the remote's tip and pushed, never forced.
func TestARejectedPushIsWrittenAgainOverTheRemoteTip(t *testing.T) {
	ctx := context.Background()
	bare := bareRemote(t)
	a, b := gitRepo(t), gitRepo(t)
	withRemote(t, a, bare)
	withRemote(t, b, bare)

	rootsAre(t, map[string]string{"repo-1": a})
	sa := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, sa)
	outA, err := NewOver(sa).Export(ctx, api.ExportInput{})
	if err != nil || !outA.Refs[0].Pushed {
		t.Fatalf("first clone's export = %+v, %v", outA, err)
	}

	rootsAre(t, map[string]string{"repo-1": b})
	sb := openSQL(t, t.TempDir(), "repo-1")
	if _, err := sb.Create(ctx, api.Todo{Module: "billing", Title: "other", State: api.StateReady, For: api.ForAgent}); err != nil {
		t.Fatal(err)
	}
	outB, err := NewOver(sb).Export(ctx, api.ExportInput{})
	if err != nil {
		t.Fatal(err)
	}
	r := outB.Refs[0]
	if !r.Pushed || r.PushError != "" {
		t.Fatalf("second clone's export = %+v, want pushed after the rewrite", r)
	}
	if got := remoteTip(t, bare, ExportRef("repo-1")); got != r.Commit {
		t.Fatalf("remote holds %s, want %s", got, r.Commit)
	}
	parent, err := gitcli.Run(b, "rev-parse", r.Commit+"^")
	if err != nil || strings.TrimSpace(parent) != outA.Refs[0].Commit {
		t.Fatalf("the rewritten export's parent = %q (%v), want the first clone's %s", parent, err, outA.Refs[0].Commit)
	}
}

// A repository with no remote keeps its ref local and says nothing.
func TestNoRemoteNoPush(t *testing.T) {
	ctx := context.Background()
	repoDir := gitRepo(t)
	rootsAre(t, map[string]string{"repo-1": repoDir})
	st := openSQL(t, t.TempDir(), "repo-1")
	fill(t, ctx, st)
	out, err := NewOver(st).Export(ctx, api.ExportInput{})
	if err != nil {
		t.Fatal(err)
	}
	if r := out.Refs[0]; r.Pushed || r.Remote != "" || r.PushError != "" {
		t.Fatalf("export with no remote = %+v", r)
	}
}

// The anonymous probe reads an ssh remote over https on the same host and
// path; a path or file:// is on this machine.
func TestProbeURL(t *testing.T) {
	for _, c := range []struct {
		in, want string
		local    bool
	}{
		{"git@example.org:example/repo.git", "https://example.org/example/repo.git", false},
		{"ssh://git@example.org:2222/team/repo.git", "https://example.org/team/repo.git", false},
		{"https://user:token@example.org/team/repo.git", "https://example.org/team/repo.git", false}, // secret-scan: fake
		{"git://example.org/repo.git", "git://example.org/repo.git", false},
		{"file:///srv/repo.git", "", true},
		{"/srv/repo.git", "", true},
		{"../repo.git", "", true},
	} {
		got, local := probeURL(c.in)
		if got != c.want || local != c.local {
			t.Errorf("probeURL(%q) = %q, %v; want %q, %v", c.in, got, local, c.want, c.local)
		}
	}
}
