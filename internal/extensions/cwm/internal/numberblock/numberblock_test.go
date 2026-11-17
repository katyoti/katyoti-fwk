// SPDX-License-Identifier: Apache-2.0

package numberblock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// gitT runs git for a test, failing it on an error.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git(dir, nil, args...)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return out
}

// clonesOfOneRemote makes a bare remote and two clones of it, each with one
// commit, and answers the clones' top levels.
func clonesOfOneRemote(t *testing.T) (remote, a, b string) {
	t.Helper()
	base := t.TempDir()
	remote = filepath.Join(base, "remote.git")
	gitT(t, base, "init", "--bare", "--quiet", remote)
	a, b = filepath.Join(base, "a"), filepath.Join(base, "b")
	for _, c := range []string{a, b} {
		gitT(t, base, "clone", "--quiet", remote, c)
	}
	return remote, a, b
}

func open(t *testing.T, dir string) *Repo {
	t.Helper()
	r, ok := Open(dir)
	if !ok {
		t.Fatalf("%s does not open as a repository", dir)
	}
	return r
}

// Two clones reserve at once: the second's push is rejected - the first
// reserved between its fetch and its push - and it retries on the remote's
// ledger, getting the next range. Both blocks are in the remote's ledger and
// they do not overlap. (Falsified: a forced push loses the first block.)
func TestTwoClonesReservingAtOnceGetDisjointBlocks(t *testing.T) {
	long.Test(t)
	remote, a, b := clonesOfOneRemote(t)
	ra, rb := open(t, a), open(t, b)
	if ra.Remote != "origin" {
		t.Fatalf("the clone's remote is %q, want origin", ra.Remote)
	}
	ma, mb := Minter{ID: "minter-a"}, Minter{ID: "minter-b"}
	var first Block
	raced := false
	rb.beforePush = func() {
		if raced {
			return
		}
		raced = true
		var err error
		if first, err = ra.Reserve(ma, 50, 0); err != nil {
			t.Fatalf("a's reservation: %v", err)
		}
	}
	second, err := rb.Reserve(mb, 50, 0)
	if err != nil {
		t.Fatalf("b's reservation: %v", err)
	}
	if !raced {
		t.Fatal("the race was not run")
	}
	if first.Start != 1 || first.End != 50 {
		t.Fatalf("a's block %d-%d, want 1-50", first.Start, first.End)
	}
	if second.Start != 51 || second.End != 100 || second.Provisional {
		t.Fatalf("b's block %+v, want a confirmed 51-100 after its rejected push", second)
	}
	// The remote's ledger, as a third reader sees it.
	bare := &Repo{Dir: remote}
	tip, err := bare.tip()
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := bare.read(tip)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger.Blocks) != 2 || ledger.Blocks[0].Minter != "minter-a" || ledger.Blocks[1].Minter != "minter-b" {
		t.Fatalf("the remote's ledger %+v, want a's block then b's", ledger.Blocks)
	}
	// The refspec is added once, so a plain fetch carries the ledger.
	if got := gitT(t, b, "config", "--get-all", "remote.origin.fetch"); !containsLine(got, "+"+Ref+":"+Ref) {
		t.Fatalf("the ledger's refspec is not in the remote's fetch list: %q", got)
	}
	if _, err := rb.Reserve(mb, 50, 0); err != nil {
		t.Fatal(err)
	}
	if got := gitT(t, b, "config", "--get-all", "remote.origin.fetch"); countLine(got, "+"+Ref+":"+Ref) != 1 {
		t.Fatalf("the ledger's refspec is added more than once: %q", got)
	}
}

// A repository with no remote reserves by the local compare-and-swap alone,
// above the floor given; the working tree and the index are never touched.
func TestAReservationWithNoRemoteIsLocal(t *testing.T) {
	dir := t.TempDir()
	gitT(t, dir, "init", "--quiet")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := open(t, dir)
	b, err := r.Reserve(Minter{ID: "m"}, 10, 966)
	if err != nil {
		t.Fatal(err)
	}
	if b.Start != 967 || b.End != 976 || b.Provisional {
		t.Fatalf("block %+v, want 967-976 above the floor", b)
	}
	if st := gitT(t, dir, "status", "--porcelain"); st != "?? f.txt\n" {
		t.Fatalf("the working tree or the index changed: %q", st)
	}
}

// A remote that cannot be reached gives a provisional block; Confirm pushes
// it once the remote is back, and a remote that took another ledger in the
// meantime refuses it (ErrBlockLost).
func TestAProvisionalBlockIsConfirmedOrLost(t *testing.T) {
	long.Test(t)
	remote, a, b := clonesOfOneRemote(t)
	ra, rb := open(t, a), open(t, b)
	gone := remote + ".away"
	if err := os.Rename(remote, gone); err != nil {
		t.Fatal(err)
	}
	pa, err := ra.Reserve(Minter{ID: "a"}, 10, 0)
	if err != nil || !pa.Provisional {
		t.Fatalf("offline: %+v, %v - want a provisional block", pa, err)
	}
	pb, err := rb.Reserve(Minter{ID: "b"}, 10, 0)
	if err != nil || !pb.Provisional || pb.Start != pa.Start {
		t.Fatalf("offline: %+v, %v - want the same provisional range", pb, err)
	}
	if ok, err := ra.Confirm(pa); ok || err != nil {
		t.Fatalf("still offline: confirmed %v, %v", ok, err)
	}
	if err := os.Rename(gone, remote); err != nil {
		t.Fatal(err)
	}
	if ok, err := ra.Confirm(pa); !ok || err != nil {
		t.Fatalf("back online: confirmed %v, %v", ok, err)
	}
	if ok, err := rb.Confirm(pb); ok || err != ErrBlockLost {
		t.Fatalf("a range the remote gave another: confirmed %v, %v - want ErrBlockLost", ok, err)
	}
	ledger, _, err := rb.ReadLedger()
	if err != nil {
		t.Fatal(err)
	}
	if owner, _ := ledger.OwnerOf(pb.Start); owner.Minter != "a" {
		t.Fatalf("after the refusal the local ledger gives %d to %q, want a", pb.Start, owner.Minter)
	}
}

func containsLine(s, line string) bool { return countLine(s, line) > 0 }

func countLine(s, line string) int {
	n := 0
	for _, l := range splitLines(s) {
		if l == line {
			n++
		}
	}
	return n
}

func splitLines(s string) []string {
	return strings.Fields(strings.ReplaceAll(s, "", ""))
}
