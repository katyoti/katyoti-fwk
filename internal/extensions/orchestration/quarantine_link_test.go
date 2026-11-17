// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// symlinkOrSkip makes a symbolic link, or skips the test with the reason when
// the system refuses one (Windows without developer mode or the privilege).
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot make a symbolic link here (on Windows it needs developer mode): %v", err)
	}
}

// folderLinkOrSkip links a folder: a symbolic link, or on Windows a junction
// when symbolic links are refused (a junction needs no privilege) - the guard
// must hold for both.
func folderLinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if os.Symlink(target, link) == nil {
		return
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if err == nil {
			return
		}
		t.Skipf("cannot make a symbolic link nor a junction here: %v %s", err, out)
	}
	t.Skip("cannot make a symbolic link here")
}

// A link inside a quarantined folder is never followed out of the quarantine
// : it is not listed as memory, the keep refuses the folder
// naming it, and nothing - neither the linked file nor the folder's own files -
// reaches the person's memory, which is joined into their agents' prompts.
func TestAKeepNeverFollowsALinkOutOfTheQuarantine(t *testing.T) {
	root := t.TempDir()
	key := MemoryKey{Tenant: "t1", Universe: "u1", Person: "alice"}
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("OUTSIDE-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideDir := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(outsideDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "deep.md"), []byte("OUTSIDE-DEEP"), 0o600); err != nil {
		t.Fatal(err)
	}
	names := quarantineOf(t, root, map[string]string{memoryEntrypoint: "KEPT-TEXT"})
	folder := filepath.Join(root, quarantineDir, names[0])
	folderLinkOrSkip(t, outsideDir, filepath.Join(folder, "linked-dir"))
	fileLinked := os.Symlink(secret, filepath.Join(folder, "leak.md")) == nil
	if !fileLinked {
		t.Log("no symbolic link to a file here (Windows without developer mode): the linked folder alone is checked")
	}

	got, err := QuarantinedMemory(root)
	if err != nil || len(got) != 1 {
		t.Fatalf("listing: %+v %v", got, err)
	}
	for _, f := range got[0].Files {
		if strings.Contains(f, "leak") || strings.Contains(f, "linked-dir") || strings.Contains(f, "deep") {
			t.Errorf("a link is listed as memory: %v", got[0].Files)
		}
	}

	err = AssignQuarantinedMemory(root, names[0], key)
	if err == nil || !strings.Contains(err.Error(), "linked-dir") || (fileLinked && !strings.Contains(err.Error(), "leak.md")) {
		t.Fatalf("a folder holding links was kept, or the refusal does not name them: %v", err)
	}
	dest := NewMemoryManager(root, key).MemoryDir()
	_ = filepath.WalkDir(dest, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			t.Errorf("something reached the memory despite the refusal: %s", p)
		}
		return nil
	})
	if p := NewMemoryManager(root, key).LoadMemoryPrompt(); strings.Contains(p, "OUTSIDE") {
		t.Fatalf("a linked file reached the prompt: %q", p)
	}
	if _, err := os.Stat(folder); err != nil {
		t.Errorf("the quarantine folder went after a refusal: %v", err)
	}
	if b, err := os.ReadFile(secret); err != nil || string(b) != "OUTSIDE-SECRET" {
		t.Errorf("the link's target was touched: %q %v", b, err)
	}

	// Deleting the folder removes the links, never what they point to.
	if err := DeleteQuarantinedMemory(root, names[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secret); err != nil {
		t.Errorf("delete removed the link's target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outsideDir, "deep.md")); err != nil {
		t.Errorf("delete removed the linked folder's content: %v", err)
	}
}

// A quarantine entry that is itself a link to a folder is no quarantined
// folder: not listed, and keep and delete refuse it, the folder it points to
// untouched.
func TestAQuarantineEntryThatIsALinkIsRefused(t *testing.T) {
	root := t.TempDir()
	key := MemoryKey{Tenant: "t1", Universe: "u1", Person: "alice"}
	quarantineOf(t, root, map[string]string{memoryEntrypoint: "x"})
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, memoryEntrypoint), []byte("OUTSIDE-MEMORY"), 0o600); err != nil {
		t.Fatal(err)
	}
	const linkName = "20261105T100000Z-linked"
	folderLinkOrSkip(t, outside, filepath.Join(root, quarantineDir, linkName))

	if got, _ := QuarantinedMemory(root); len(got) != 1 || got[0].Name == linkName {
		t.Errorf("a linked entry is listed: %+v", got)
	}
	if err := AssignQuarantinedMemory(root, linkName, key); err == nil {
		t.Error("keep of a linked entry accepted")
	}
	if p := NewMemoryManager(root, key).LoadMemoryPrompt(); strings.Contains(p, "OUTSIDE-MEMORY") {
		t.Fatalf("a linked entry reached the prompt: %q", p)
	}
	if err := DeleteQuarantinedMemory(root, linkName); err == nil {
		t.Error("delete of a linked entry accepted")
	}
	if b, err := os.ReadFile(filepath.Join(outside, memoryEntrypoint)); err != nil || string(b) != "OUTSIDE-MEMORY" {
		t.Errorf("the linked folder was touched: %q %v", b, err)
	}
}

// openRoot opens dir as an os.Root closed at the test's end.
func openRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// A file swapped for a link after the walk is still refused by the read: the
// guard does not rest on the walk alone.
func TestReadingAQuarantinedFileNeverFollowsALink(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("OUTSIDE-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain.md"), []byte("PLAIN"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := openRoot(t, dir)
	if b, err := readQuarantinedFile(r, "plain.md"); err != nil || string(b) != "PLAIN" {
		t.Fatalf("a plain file: %q %v", b, err)
	}
	symlinkOrSkip(t, secret, filepath.Join(dir, "swapped.md"))
	if b, err := readQuarantinedFile(r, "swapped.md"); err == nil || strings.Contains(string(b), "OUTSIDE") {
		t.Fatalf("a link was read through: %q %v", b, err)
	}
}

// A hard link to a file outside the quarantine is a regular file with that
// file's identity; the keep refuses it by its link count, naming it, and
// nothing reaches the person's memory.
func TestAKeepRefusesAHardLinkedFile(t *testing.T) {
	root := t.TempDir()
	key := MemoryKey{Tenant: "t1", Universe: "u1", Person: "alice"}
	// The secret sits beside the quarantine, on the same volume (a hard link
	// cannot cross volumes), outside the folder kept.
	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("OUTSIDE-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	names := quarantineOf(t, root, map[string]string{memoryEntrypoint: "KEPT-TEXT"})
	folder := filepath.Join(root, quarantineDir, names[0])
	if err := os.Link(secret, filepath.Join(folder, "hard.md")); err != nil {
		t.Skipf("cannot make a hard link here: %v", err)
	}

	err := AssignQuarantinedMemory(root, names[0], key)
	if err == nil || !strings.Contains(err.Error(), "hard.md") {
		t.Fatalf("a folder holding a hard link was kept, or the refusal does not name it: %v", err)
	}
	if p := NewMemoryManager(root, key).LoadMemoryPrompt(); strings.Contains(p, "OUTSIDE") || strings.Contains(p, "KEPT-TEXT") {
		t.Fatalf("something reached the prompt despite the refusal: %q", p)
	}
	if _, err := os.Stat(folder); err != nil {
		t.Errorf("the quarantine folder went after a refusal: %v", err)
	}
	if b, err := os.ReadFile(secret); err != nil || string(b) != "OUTSIDE-SECRET" {
		t.Errorf("the hard link's other name was touched: %q %v", b, err)
	}
}

// A sub-folder swapped for a link or a junction between the walk and the read
// does not lead out of the quarantined folder: every path component is
// resolved inside it (os.Root), so the read is refused.
func TestASubFolderSwappedForALinkIsNotReadThrough(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "notes.md"), []byte("INSIDE"), 0o600); err != nil {
		t.Fatal(err)
	}
	files, _, odd, err := quarantinedFiles(dir)
	if err != nil || len(odd) != 0 || len(files) != 1 || files[0] != "sub/notes.md" {
		t.Fatalf("the walk: %v %v %v", files, odd, err)
	}
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "notes.md"), []byte("OUTSIDE-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	// After the walk, the sub-folder is swapped for a link to the outside.
	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}
	folderLinkOrSkip(t, outside, sub)

	if b, err := readQuarantinedFiles(dir, []string{"sub/notes.md"}); err == nil || strings.Contains(string(bytesOf(b)), "OUTSIDE") {
		t.Fatalf("a swapped sub-folder was read through: %q %v", bytesOf(b), err)
	}
}

func bytesOf(b [][]byte) []byte {
	var out []byte
	for _, x := range b {
		out = append(out, x...)
	}
	return out
}
