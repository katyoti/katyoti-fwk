// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
)

// gitIn runs git in dir as the author name, failing the test on an error.
func gitIn(t *testing.T, dir, author string, args ...string) string {
	t.Helper()
	out, err := gitcli.Run(dir, append([]string{"-c", "user.name=" + author, "-c", "user.email=" + author + "@t", "-c", "core.autocrlf=false"}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out)
}

// storeObject writes an object into the universe's object store by hand, the
// way a bulk copy or a pull of the store's repository puts it there.
func storeObject(t *testing.T, ws, id, name string) string {
	t.Helper()
	dir := filepath.Join(storage.NewPaths(ws).CWMDataDir(), ObjectsDirName, "cwm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	o := obj(id, "feature", "")
	o.Name = name
	path := filepath.Join(dir, objectFileName("cwm", id))
	b, err := encodeObjectFile(path, o)
	if err == nil {
		err = os.WriteFile(path, b, 0o644)
	}
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// grouped are the grouped events an open published, by their sha.
func grouped(t *testing.T, c *captured) map[string]events.ObjectsCaughtUpPayload {
	t.Helper()
	out := map[string]events.ObjectsCaughtUpPayload{}
	for _, g := range c.all() {
		p, ok := g.(events.ObjectsCaughtUpPayload)
		if !ok {
			t.Fatalf("a grouped event's payload is a %T", g)
		}
		out[p.Sha] = p
	}
	return out
}

// TestEachRepositoryIsCaughtUpFromItsOwnCursor pins: the project
// root and the universe's object store sit in two git repositories, an edit is
// committed outside CWM in each, and the next open catches both up - each from
// its own HEAD, with its own commit's author and sha - and keeps one cursor per
// repository, keyed by its top level.
func TestEachRepositoryIsCaughtUpFromItsOwnCursor(t *testing.T) {
	r := newCatchUpRepo(t)
	paths := r.seeded("cwm.a")
	gitIn(t, r.ws, "s", "init", "-q")
	store := storeObject(t, r.ws, "cwm.s", "in the store")
	gitIn(t, r.ws, "s", "add", "--", store)
	gitIn(t, r.ws, "s", "commit", "-q", "-m", "store")
	if _, seen := r.open(); len(*seen) != 0 {
		t.Fatalf("a repository the cursor never saw was announced, not taken as a baseline: %+v", *seen)
	}
	projFrom := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	storeFrom := gitIn(t, r.ws, "s", "rev-parse", "HEAD")

	r.handEdit(paths["cwm.a"], "name: cwm.a", "name: edited in the project")
	r.commit("project edit")
	r.handEdit(store, "name: in the store", "name: edited in the store")
	gitIn(t, r.ws, "s", "commit", "-q", "-am", "store edit")
	projTo := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	storeTo := gitIn(t, r.ws, "s", "rev-parse", "HEAD")

	_, c := r.openPublishing()
	got := grouped(t, c)
	if len(got) != 2 {
		t.Fatalf("the open published %d grouped events, want one per repository: %+v", len(got), got)
	}
	for _, want := range []struct {
		id, from, to, author string
	}{{"cwm.a", projFrom, projTo, "t"}, {"cwm.s", storeFrom, storeTo, "s"}} {
		p, ok := got[want.to]
		if !ok {
			t.Fatalf("no grouped event carries %s's commit %s: %+v", want.id, want.to, got)
		}
		if !slices.Equal(p.Modified, []string{want.id}) || len(p.Added)+len(p.Removed) != 0 {
			t.Fatalf("the event of %s's repository names added %v, modified %v, removed %v", want.id, p.Added, p.Modified, p.Removed)
		}
		if p.Cause != (api.CaughtUpCause{Kind: api.CaughtUpCommits, From: want.from, To: want.to}) || p.By.ID != want.author {
			t.Fatalf("the event of %s's repository: cause %+v by %+v, want the commits %s..%s by %s", want.id, p.Cause, p.By, want.from, want.to, want.author)
		}
	}

	cur, found, err := readCursor(svcCursorPath(r))
	if err != nil || !found {
		t.Fatalf("no cursor: %v", err)
	}
	heads := map[string]string{}
	for top, rc := range cur.Repos {
		heads[rc.Head] = top
	}
	if top := heads[projTo]; !sameDir(top, r.root) {
		t.Fatalf("the project's HEAD is kept under %q, want its top level %s: %+v", top, r.root, cur.Repos)
	}
	if top := heads[storeTo]; !sameDir(top, r.ws) {
		t.Fatalf("the store's HEAD is kept under %q, want its top level %s: %+v", top, r.ws, cur.Repos)
	}
}

// TestARootOutsideGitIsCaughtUpByItsManifest pins the fallback: a project in
// no git repository is caught up by the manifest diff alone - the working
// tree its cause, an outside actor with no sha.
func TestARootOutsideGitIsCaughtUpByItsManifest(t *testing.T) {
	r := &catchUpRepo{t: t, root: t.TempDir(), ws: freshWorkspace(t)}
	svc, _ := r.open()
	if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: obj("cwm.a", "feature", "")}); err != nil {
		t.Fatal(err)
	}
	path := r.objectPath(svc, "cwm.a")
	_ = svc.Close()

	r.handEdit(path, "name: cwm.a", "name: edited outside git")
	_, c := r.openPublishing()
	p := caughtUp(t, c)
	if !slices.Equal(p.Modified, []string{"cwm.a"}) || p.Sha != "" || p.Cause.Kind != api.CaughtUpWorkingTree || p.Cause.To != "" {
		t.Fatalf("a root outside git caught up as modified %v, sha %q, cause %+v; want [cwm.a], no sha, the working tree", p.Modified, p.Sha, p.Cause)
	}
}

func svcCursorPath(r *catchUpRepo) string {
	return filepath.Join(storage.NewPaths(r.ws).CWMDataDir(), catchUpCursorFile)
}

// sameDir compares a slash-form top level with a folder, as the file system
// does on Windows (case-insensitive, either separator).
func sameDir(top, dir string) bool {
	a, errA := filepath.EvalSymlinks(filepath.FromSlash(top))
	b, errB := filepath.EvalSymlinks(dir)
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
