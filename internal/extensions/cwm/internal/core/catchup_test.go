// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// catchUpRepo is a project in its own git repository, its universe's CWM data
// in a folder of its own, opened again as a restart does.
type catchUpRepo struct {
	t        *testing.T
	root, ws string
}

func newCatchUpRepo(t *testing.T) *catchUpRepo {
	t.Helper()
	r := &catchUpRepo{t: t, root: t.TempDir(), ws: freshWorkspace(t)}
	r.git("init", "-q")
	return r
}

func (r *catchUpRepo) git(args ...string) string {
	r.t.Helper()
	out, err := gitcli.Run(r.root, append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "core.autocrlf=false"}, args...)...)
	if err != nil {
		r.t.Fatal(err)
	}
	return out
}

func (r *catchUpRepo) commit(msg string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

// open starts a service on the project - a restart - and loads its index,
// counting the catch-ups that announced something.
func (r *catchUpRepo) open() (*Service, *[]catchUp) {
	r.t.Helper()
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: r.ws, ProjectRoot: r.root})
	if err != nil {
		r.t.Fatalf("New: %v", err)
	}
	r.t.Cleanup(func() { _ = svc.Close() })
	var seen []catchUp
	ix := svc.index()
	ix.catch.on = func(c catchUp) { seen = append(seen, c) }
	if err := ix.ensureLoaded(); err != nil {
		r.t.Fatal(err)
	}
	return svc, &seen
}

func (r *catchUpRepo) objectPath(svc *Service, id string) string {
	r.t.Helper()
	p, ok := svc.index().pathOf(id)
	if !ok {
		r.t.Fatalf("%s is not indexed", id)
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(r.root, p)
	}
	return p
}

// handEdit replaces old by new in a file, as an editor does.
func (r *catchUpRepo) handEdit(path, old, new string) {
	r.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		r.t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		r.t.Fatalf("%s holds no %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), old, new, 1)), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// seeded opens the project, files objects through the verbs, commits them
// and closes: the baseline a later open catches up from.
func (r *catchUpRepo) seeded(ids ...string) map[string]string {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.root, "README.md"), []byte("a project\n"), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.commit("start")
	svc, _ := r.open()
	paths := map[string]string{}
	for _, id := range ids {
		if _, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: "cwm", Object: obj(id, "feature", "")}); err != nil {
			r.t.Fatal(err)
		}
		paths[id] = r.objectPath(svc, id)
	}
	if _, err := svc.ObjectScan(stdctx.Background(), api.ObjectScanInput{}); err != nil {
		r.t.Fatal(err)
	}
	_ = svc.Close()
	r.commit("seed")
	return paths
}

// TestAHandEditIsCaughtUpAtOpen pins the seeing half of: an object
// edited, added and removed by hand while CWM was closed is read at the next
// open, and the catch-up names each one, in one announcement.
func TestAHandEditIsCaughtUpAtOpen(t *testing.T) {
	r := newCatchUpRepo(t)
	paths := r.seeded("cwm.a", "cwm.b", "cwm.c")

	r.handEdit(paths["cwm.a"], "name: cwm.a", "name: edited by hand")
	if err := os.Remove(paths["cwm.b"]); err != nil {
		t.Fatal(err)
	}
	added := filepath.Join(filepath.Dir(paths["cwm.c"]), strings.Replace(filepath.Base(paths["cwm.c"]), "c", "d", 1))
	raw, _ := os.ReadFile(paths["cwm.c"])
	if err := os.WriteFile(added, []byte(strings.ReplaceAll(string(raw), "cwm.c", "cwm.d")), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, seen := r.open()
	got, err := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{ID: "cwm.a"})
	if err != nil || got.Object.Name != "edited by hand" {
		t.Fatalf("a read after the open does not see the hand edit: %+v, %v", got.Object.Name, err)
	}
	if len(*seen) != 1 {
		t.Fatalf("the open announced %d catch-ups, want one: %+v", len(*seen), *seen)
	}
	c := (*seen)[0]
	if !slices.Equal(c.Modified, []string{"cwm.a"}) || !slices.Equal(c.Removed, []string{"cwm.b"}) || !slices.Equal(c.Added, []string{"cwm.d"}) {
		t.Fatalf("the catch-up names added %v, modified %v, removed %v; want [cwm.d] [cwm.a] [cwm.b]", c.Added, c.Modified, c.Removed)
	}
	if head := strings.TrimSpace(r.git("rev-parse", "HEAD")); c.From == "" || c.To != head {
		t.Fatalf("the catch-up's cause names HEAD %q -> %q, want the last commit seen -> %s", c.From, c.To, head)
	}
}

// TestAQuietRestartAnnouncesNothing pins that an unchanged tree, and a dirty
// file already announced, announce nothing after a restart - and that a
// dirty file reverted since is announced.
func TestAQuietRestartAnnouncesNothing(t *testing.T) {
	r := newCatchUpRepo(t)
	paths := r.seeded("cwm.a", "cwm.b")

	if _, seen := r.open(); len(*seen) != 0 {
		t.Fatalf("a restart on an unchanged tree announced %+v", *seen)
	}

	r.handEdit(paths["cwm.a"], "name: cwm.a", "name: dirty")
	if _, seen := r.open(); len(*seen) != 1 || !slices.Equal((*seen)[0].Modified, []string{"cwm.a"}) {
		t.Fatalf("the dirty edit was not announced once: %+v", *seen)
	}
	if _, seen := r.open(); len(*seen) != 0 {
		t.Fatalf("a restart on a dirty file already announced announced it again: %+v", *seen)
	}

	r.git("checkout", "--", ".")
	_, seen := r.open()
	if len(*seen) != 1 || !slices.Equal((*seen)[0].Modified, []string{"cwm.a"}) {
		t.Fatalf("a dirty file reverted to HEAD was not announced: %+v", *seen)
	}
}

// TestACommitMadeOutsideIsCaughtUpAtTheNextScan pins the pull: a commit made
// outside CWM while it runs (here, a hand edit committed) is announced by the
// next object-scan, with the cursor's HEAD and the new one.
func TestACommitMadeOutsideIsCaughtUpAtTheNextScan(t *testing.T) {
	r := newCatchUpRepo(t)
	paths := r.seeded("cwm.a", "cwm.b")
	from := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	svc, seen := r.open()

	r.handEdit(paths["cwm.b"], "name: cwm.b", "name: pulled")
	r.commit("pulled")
	to := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	if _, err := svc.ObjectScan(stdctx.Background(), api.ObjectScanInput{}); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 1 {
		t.Fatalf("the scan announced %d catch-ups, want one", len(*seen))
	}
	c := (*seen)[0]
	if !slices.Equal(c.Modified, []string{"cwm.b"}) || c.From != from || c.To != to {
		t.Fatalf("catch-up %+v, want cwm.b modified from %s to %s", c, from, to)
	}
	if _, err := svc.ObjectScan(stdctx.Background(), api.ObjectScanInput{}); err != nil || len(*seen) != 1 {
		t.Fatalf("a second scan announced the commit again: %+v, %v", *seen, err)
	}
}

// TestACatchUpLeavesTheTreeByteIdentical pins that a catch-up writes no file
// of the project: only its cursor, in the universe's CWM data.
func TestACatchUpLeavesTheTreeByteIdentical(t *testing.T) {
	r := newCatchUpRepo(t)
	paths := r.seeded("cwm.a", "cwm.b")
	r.handEdit(paths["cwm.a"], "name: cwm.a", "name: by hand")
	before := treeDigest(t, r.root)
	if _, seen := r.open(); len(*seen) != 1 {
		t.Fatalf("the hand edit was not caught up: %+v", *seen)
	}
	if after := treeDigest(t, r.root); after != before {
		t.Fatal("a catch-up changed the project's files")
	}
	if _, err := os.Stat(filepath.Join(r.ws, "cwm", catchUpCursorFile)); err != nil {
		t.Fatalf("the cursor is not kept in the universe's CWM data: %v", err)
	}
}

// TestManyChangedObjectsAreCaughtUpFast pins the budget: 1,187 objects
// changed outside CWM are re-indexed and caught up in at most 2 s at open.
func TestManyChangedObjectsAreCaughtUpFast(t *testing.T) {
	long.Test(t)
	const n = 1187
	r := newCatchUpRepo(t)
	dir := filepath.Join(r.root, api.DefaultModulesDir, "cwm", "objects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string) { // plain writes, as a bulk copy makes them
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("cwm.o%04d", i)
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
		}
	}
	write("before")
	if _, seen := r.open(); len(*seen) != 0 {
		t.Fatal("a first open announced its baseline")
	}
	write("after")

	start := time.Now()
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: r.ws, ProjectRoot: r.root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	var seen []catchUp
	ix := svc.index()
	ix.catch.on = func(c catchUp) { seen = append(seen, c) }
	if err := ix.ensureLoaded(); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	if len(seen) != 1 || len(seen[0].Modified) != n {
		t.Fatalf("the open did not catch up %d modified objects: %+v", n, len(seen))
	}
	if took > 2*time.Second {
		t.Fatalf("re-indexing and catching up %d changed objects took %s, the budget is 2 s", n, took)
	}
	t.Logf("%d changed objects caught up in %s", n, took)
}

// treeDigest hashes every file of a project but its git folder.
func treeDigest(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%x\n", filepath.ToSlash(p), sha256.Sum256(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}
