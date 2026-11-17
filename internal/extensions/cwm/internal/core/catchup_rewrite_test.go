// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestARewrittenHistoryIsCaughtUpByTheManifestNotACommitRange pins
// when HEAD moved but not forward: the HEAD CWM saw last is no
// ancestor of this one - a reset, a rebase, a force-pull. The catch-up still
// names exactly the object whose file changed (the manifest decides), and its
// cause is a rewritten history between the two HEADs, never a commit range
// with a sha.
func TestARewrittenHistoryIsCaughtUpByTheManifestNotACommitRange(t *testing.T) {
	r := newCatchUpRepo(t)
	paths := r.seeded("cwm.a", "cwm.b", "cwm.c")
	seed := strings.TrimSpace(r.git("rev-parse", "HEAD"))

	// A commit that moves no object: the cursor records it as its HEAD.
	if err := os.WriteFile(filepath.Join(r.root, "README.md"), []byte("a project, edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.commit("readme")
	seen := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	if _, c := r.openPublishing(); len(c.all()) != 0 {
		t.Fatalf("a commit that moved no object was announced: %+v", c.all())
	}

	// The history is rewritten: back to the seed, a new commit edits cwm.b.
	r.git("reset", "-q", "--hard", seed)
	r.handEdit(paths["cwm.b"], "name: cwm.b", "name: rewritten")
	r.commit("rewritten")
	now := strings.TrimSpace(r.git("rev-parse", "HEAD"))

	_, c := r.openPublishing()
	p := caughtUp(t, c)
	if !slices.Equal(p.Modified, []string{"cwm.b"}) || len(p.Added)+len(p.Removed) != 0 {
		t.Fatalf("the event names added %v, modified %v, removed %v; want exactly [cwm.b] modified", p.Added, p.Modified, p.Removed)
	}
	if p.Cause != (api.CaughtUpCause{Kind: api.CaughtUpRewritten, From: seen, To: now}) {
		t.Fatalf("cause %+v, want a rewritten history %s -> %s, not a commit range", p.Cause, seen, now)
	}
	if p.Sha != "" || p.By.ID == "" {
		t.Fatalf("actor %+v sha %q: a rewritten history names the local user with no sha", p.By, p.Sha)
	}

	// A HEAD git does not have (gc, shallow clone) is no ancestor either.
	top, _ := gitTopLevel(r.root)
	if isAncestor(top, strings.Repeat("0", 40), now) {
		t.Fatal("a HEAD git does not have reads as an ancestor")
	}
	if !isAncestor(top, seed, now) || !isAncestor(top, "", now) {
		t.Fatal("the seed (or no HEAD at all) does not read as an ancestor of the rewritten HEAD")
	}
}

// TestADeletedCursorIsANewBaseline pins the lost cursor: with
// catchup-cursor.json gone, the next open announces nothing - not the whole
// tree as added - and writes the cursor; an open after it is quiet.
func TestADeletedCursorIsANewBaseline(t *testing.T) {
	r := newCatchUpRepo(t)
	paths := r.seeded("cwm.a", "cwm.b")
	svc, _ := r.open()
	cursor := svc.index().cursorPath()
	_ = svc.Close()

	r.handEdit(paths["cwm.a"], "name: cwm.a", "name: edited while the cursor was lost")
	r.commit("edited")
	if err := os.Remove(cursor); err != nil {
		t.Fatalf("the cursor was not written beside the store: %v", err)
	}

	if _, c := r.openPublishing(); len(c.all()) != 0 {
		t.Fatalf("an open with no cursor announced: %+v", c.all())
	}
	if _, err := os.Stat(cursor); err != nil {
		t.Fatalf("an open with no cursor did not write it: %v", err)
	}
	if _, c := r.openPublishing(); len(c.all()) != 0 {
		t.Fatalf("an open after the new baseline announced: %+v", c.all())
	}
}
