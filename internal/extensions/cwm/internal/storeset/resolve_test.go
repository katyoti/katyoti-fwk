// SPDX-License-Identifier: Apache-2.0

package storeset

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestBareIDResolvesAgainstItsProject: two projects of one
// repository hold the same ids. A qualified id <key>:<module>.<local> resolves
// in the project of its key, a full name in the project of its uuid, and a
// bare id in the project whose text holds it - never in the other project
// holding the same id. An edge written before several projects existed is
// read unchanged and still names its own project's object; the writer keeps a
// reference bare inside a project and qualifies it across.
func TestBareIDResolvesAgainstItsProject(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	alphaDir, betaDir := filepath.Join(repo, "alpha"), filepath.Join(repo, "beta")
	set, err := New(projectStore(t, alphaDir, "alpha"), projectStore(t, betaDir, "beta"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, p := range []string{"alpha", "beta"} {
		if _, _, err := set.ObjectCreate(ctx, p, api.ObjectCreateInput{Module: "inbox", Object: note("inbox.n1")}); err != nil {
			t.Fatalf("create inbox.n1 in %s: %v", p, err)
		}
	}
	// An existing edge: alpha's object names inbox.n1 bare.
	src := note("inbox.src")
	src.Edges = []api.ObjectEdge{{Kind: "depends-on", To: "inbox.n1"}}
	if _, _, err := set.ObjectCreate(ctx, "alpha", api.ObjectCreateInput{Module: "inbox", Object: src}); err != nil {
		t.Fatalf("create inbox.src: %v", err)
	}
	if _, _, err := set.ObjectCreate(ctx, "beta", api.ObjectCreateInput{Module: "inbox", Object: note("inbox.only-beta")}); err != nil {
		t.Fatalf("create inbox.only-beta: %v", err)
	}

	inFolder := func(out api.ObjectResolveOutput, dir string) bool {
		rel, err := filepath.Rel(dir, out.Path)
		return err == nil && !strings.HasPrefix(rel, "..")
	}

	// A bare id in each project resolves against that project.
	for _, c := range []struct{ from, dir string }{{"alpha", alphaDir}, {"beta", betaDir}} {
		key, out, err := set.Resolve(ctx, c.from, "inbox.n1")
		if err != nil || key != c.from || out.Project != c.from || out.ID != "inbox.n1" || !inFolder(out, c.dir) {
			t.Fatalf("bare inbox.n1 written in %s: key %q, %+v, %v - want %s's object", c.from, key, out, err, c.from)
		}
	}
	// A qualified id resolves in its key's project, whoever wrote it.
	for _, c := range []struct{ from, ref, want, dir string }{
		{"alpha", "beta:inbox.n1", "beta", betaDir},
		{"beta", "alpha:inbox.n1", "alpha", alphaDir},
		{"", "beta:inbox.n1", "beta", betaDir},
		{"alpha", "alpha:inbox.n1", "alpha", alphaDir},
	} {
		key, out, err := set.Resolve(ctx, c.from, c.ref)
		if err != nil || key != c.want || out.ID != "inbox.n1" || !inFolder(out, c.dir) {
			t.Fatalf("%s written in %q: key %q, %+v, %v - want %s's object", c.ref, c.from, key, out, err, c.want)
		}
	}
	// A full name resolves in the project of its uuid.
	betaStore, _ := set.Store("beta")
	full := api.FullName(betaStore.ProjectUUID(), "inbox.n1")
	if key, out, err := set.Resolve(ctx, "alpha", full); err != nil || key != "beta" || out.FullName != full {
		t.Fatalf("full name %s: key %q, %+v, %v - want beta", full, key, out, err)
	}

	// A bare id never crosses to the other project: refused, naming the form.
	if _, _, err := set.Resolve(ctx, "alpha", "inbox.only-beta"); !errors.Is(err, api.ErrNotFound) || !strings.Contains(err.Error(), "<key>:inbox.only-beta") {
		t.Fatalf("bare id of beta written in alpha: %v - want not found, naming <key>:inbox.only-beta", err)
	}
	// No container named: the only holder, and several holders are refused.
	if key, _, err := set.Resolve(ctx, "", "inbox.only-beta"); err != nil || key != "beta" {
		t.Fatalf("bare id one project holds, no container: key %q, %v - want beta", key, err)
	}
	if _, _, err := set.Resolve(ctx, "", "inbox.n1"); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("bare id both hold, no container: %v - want refused", err)
	}
	// A key no store serves, and a malformed qualified id.
	if _, _, err := set.Resolve(ctx, "alpha", "gamma:inbox.n1"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("unknown key: %v - want not found", err)
	}
	if _, _, err := set.Resolve(ctx, "alpha", "a:b:inbox.n1"); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("malformed qualified id: %v - want invalid input", err)
	}
	// One store alone resolves its own key and refuses another's.
	alphaStore, _ := set.Store("alpha")
	if _, err := alphaStore.ObjectResolve(ctx, api.ObjectResolveInput{Ref: "beta:inbox.n1"}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("alpha's store resolving beta:inbox.n1: %v - want not found", err)
	}

	// The existing edge is read unchanged and still names alpha's object.
	_, got, err := set.ObjectGet(ctx, "alpha", api.ObjectGetInput{ID: "inbox.src"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Object.Edges) != 1 || got.Object.Edges[0].To != "inbox.n1" {
		t.Fatalf("edges of inbox.src: %+v - want the bare edge to inbox.n1 unchanged", got.Object.Edges)
	}
	if key, out, err := set.Resolve(ctx, "alpha", got.Object.Edges[0].To); err != nil || key != "alpha" || !inFolder(out, alphaDir) {
		t.Fatalf("existing edge's target: key %q, %+v, %v - want alpha's inbox.n1", key, out, err)
	}

	// The writer: bare inside a project, qualified across, unknown keys refused.
	for _, c := range []struct{ from, to, want string }{
		{"alpha", "alpha", "inbox.n1"},
		{"alpha", "beta", "beta:inbox.n1"},
		{"beta", "alpha", "alpha:inbox.n1"},
	} {
		if ref, err := set.Reference(c.from, c.to, "inbox.n1"); err != nil || ref != c.want {
			t.Fatalf("Reference(%s -> %s) = %q, %v - want %q", c.from, c.to, ref, err, c.want)
		}
		if key, _, err := set.Resolve(ctx, c.from, c.want); err != nil || key != c.to {
			t.Fatalf("the written %q read back from %s: key %q, %v - want %s", c.want, c.from, key, err, c.to)
		}
	}
	if _, err := set.Reference("alpha", "gamma", "inbox.n1"); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("Reference to an unserved project: %v - want not found", err)
	}
}
