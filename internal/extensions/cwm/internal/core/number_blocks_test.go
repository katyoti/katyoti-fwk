// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/numberblock"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// : object numbers come from number blocks a minter reserves by a
// commit in the repository that holds the objects (decision).

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

// blockService opens a service on the project root, its minter in its own
// folder, blocks of size numbers.
func blockService(t *testing.T, root, minterDir string, size int) *Service {
	t.Helper()
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: root, MinterDir: minterDir, NumberBlockSize: size}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "module"},
		{Name: "request", IDPrefix: "RQ"},
		{Name: "feature", IDPrefix: "F"},
	}
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	return svc
}

func createIn(t *testing.T, svc *Service, module string, o api.Object) api.Object {
	t.Helper()
	out, err := svc.ObjectCreate(stdctx.Background(), api.ObjectCreateInput{Module: module, Object: o})
	if err != nil {
		t.Fatalf("create %+v: %v", o, err)
	}
	return out.Object
}

func mintIn(t *testing.T, svc *Service, module, typ string) api.Object {
	t.Helper()
	return createIn(t, svc, module, api.Object{Type: typ, Name: "a " + typ})
}

// Two clones of one repository, through a bare remote, create objects turn by
// turn: no id is minted twice, and each clone's numbers lie in its own blocks.
func TestTwoClonesNeverMintTheSameID(t *testing.T) {
	long.Test(t)
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	runGit(t, base, "init", "--bare", "--quiet", remote)
	a, b := filepath.Join(base, "a"), filepath.Join(base, "b")
	runGit(t, base, "clone", "--quiet", remote, a)
	runGit(t, base, "clone", "--quiet", remote, b)
	sa := blockService(t, a, filepath.Join(base, "minter-a"), 3)
	sb := blockService(t, b, filepath.Join(base, "minter-b"), 3)
	seen := map[string]string{}
	for i := 0; i < 4; i++ {
		for name, svc := range map[string]*Service{"a": sa, "b": sb} {
			o := mintIn(t, svc, "m", "request")
			if prior, dup := seen[o.ID]; dup {
				t.Fatalf("%s minted %s, already minted by %s", name, o.ID, prior)
			}
			seen[o.ID] = name
		}
	}
	if len(seen) != 8 {
		t.Fatalf("%d ids, want 8", len(seen))
	}
}

// A restore: the minter's cache rolled back to an older block does not bring
// it back - the ledger gives the open block and the index the numbers used.
// (Falsified: a next number that ignores the index re-mints one.)
func TestARolledBackCacheMintsNoUsedNumber(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")
	minter := filepath.Join(t.TempDir(), "cwm")
	svc := blockService(t, root, minter, 2)
	used := map[string]bool{}
	used[mintIn(t, svc, "m", "request").ID] = true
	kept, err := os.ReadFile(filepath.Join(minter, numberblock.CacheFile))
	if err != nil {
		t.Fatalf("no cache after a mint: %v", err)
	}
	for i := 0; i < 2; i++ { // the first block spent, a second reserved and used
		used[mintIn(t, svc, "m", "request").ID] = true
	}
	if err := os.WriteFile(filepath.Join(minter, numberblock.CacheFile), kept, 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		id := mintIn(t, svc, "m", "request").ID
		if used[id] {
			t.Fatalf("after the cache was rolled back, %s was minted again (used: %v)", id, used)
		}
		used[id] = true
	}
}

// A created object records its minter, once: an update does not change it.
// An object written without one stays valid.
func TestACreatedObjectRecordsItsMinter(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")
	minterDir := filepath.Join(t.TempDir(), "cwm")
	svc := blockService(t, root, minterDir, 50)
	o := mintIn(t, svc, "m", "feature")
	m, err := numberblock.LoadMinter(minterDir, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{Module: "m", ID: o.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Object.Minter == "" || got.Object.Minter != m.ID {
		t.Fatalf("the created object's minter is %q, want %q", got.Object.Minter, m.ID)
	}
	edited := got.Object
	edited.Minter = "someone-else"
	edited.Description = "edited"
	if _, err := svc.ObjectUpdate(stdctx.Background(), api.ObjectUpdateInput{Module: "m", Object: edited}); err != nil {
		t.Fatal(err)
	}
	again, _ := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{Module: "m", ID: o.ID})
	if again.Object.Minter != m.ID {
		t.Fatalf("an update changed the minter to %q", again.Object.Minter)
	}
	legacy := createIn(t, svc, "m", api.Object{ID: "m.F-900", Type: "feature", Name: "before blocks"})
	if legacy.Minter != "" {
		t.Fatalf("an object created with its id records a minter %q", legacy.Minter)
	}
	if errs := svc.problemsOf(legacy.ID); len(errs) > 0 {
		t.Fatalf("an object with no minter is not valid: %v", errs)
	}
}

// Two objects on one number (two minters on a provisional block): the scan's
// duplicate names the one whose minter does not own the block.
// (Falsified: naming the owner's file.)
func TestADuplicateNamesTheObjectToRenumber(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")
	minterDir := filepath.Join(t.TempDir(), "cwm")
	svc := blockService(t, root, minterDir, 50)
	owned := mintIn(t, svc, "m", "request") // reserves the block, minter X
	other := owned
	other.Minter = "minter-y"
	other.Name = "the other minter's"
	dir := filepath.Join(root, "modules", "m", "other")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(dir, strings.TrimPrefix(owned.ID, "m.")+"-y"+ObjectFileSuffix)
	if err := WriteObjectFile(stray, other); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectScan(stdctx.Background(), api.ObjectScanInput{}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ObjectErrorList(stdctx.Background(), api.ObjectErrorListInput{})
	if err != nil {
		t.Fatal(err)
	}
	var problem string
	for _, u := range list.Unreadable {
		problem += strings.Join(u.Problems, " ")
	}
	if !strings.Contains(problem, api.DiagnosticDuplicateID) {
		t.Fatalf("no duplicate listed: %+v", list.Unreadable)
	}
	if !strings.Contains(problem, "renumber modules/m/other/") {
		t.Fatalf("the duplicate does not name the other minter's file to renumber: %s", problem)
	}
}

// A project in no git repository mints as before - the next number after the
// highest - and reserves no block.
func TestAProjectOutOfGitMintsAsBefore(t *testing.T) {
	root := t.TempDir()
	minterDir := filepath.Join(t.TempDir(), "cwm")
	svc := blockService(t, root, minterDir, 50)
	createIn(t, svc, "m", api.Object{ID: "m.RQ-007", Type: "request", Name: "seven"})
	if o := mintIn(t, svc, "m", "request"); o.ID != "m.RQ-008" || o.Minter != "" {
		t.Fatalf("minted %s (minter %q), want m.RQ-008 with none", o.ID, o.Minter)
	}
	if _, err := os.Stat(filepath.Join(minterDir, numberblock.CacheFile)); !os.IsNotExist(err) {
		t.Fatalf("a block was cached for a project out of git: %v", err)
	}
}

// The first block starts above the highest number of any prefix and module;
// the ids already there are never renumbered.
func TestTheFirstBlockStartsAboveEveryNumber(t *testing.T) {
	root := t.TempDir()
	minterDir := filepath.Join(t.TempDir(), "cwm")
	svc := blockService(t, root, minterDir, 50)
	createIn(t, svc, "m", api.Object{ID: "m.RQ-012", Type: "request", Name: "twelve"})
	createIn(t, svc, "n", api.Object{ID: "n.F-307", Type: "feature", Name: "three hundred and seven"})
	// A dated id is no readable number.
	createIn(t, svc, "n", api.Object{ID: "n.RQ-20261104", Type: "request", Name: "dated"})
	_ = svc.Close()
	runGit(t, root, "init", "--quiet")
	svc = blockService(t, root, minterDir, 50)
	if o := mintIn(t, svc, "m", "request"); o.ID != "m.RQ-308" {
		t.Fatalf("minted %s, want m.RQ-308 above n.F-307", o.ID)
	}
	if o := mintIn(t, svc, "n", "feature"); o.ID != "n.F-308" {
		t.Fatalf("minted %s, want n.F-308: the block holds every prefix and module", o.ID)
	}
	for _, id := range []string{"m.RQ-012", "n.F-307"} {
		if _, err := svc.ObjectGet(stdctx.Background(), api.ObjectGetInput{ID: id}); err != nil {
			t.Fatalf("%s is gone: %v", id, err)
		}
	}
}
