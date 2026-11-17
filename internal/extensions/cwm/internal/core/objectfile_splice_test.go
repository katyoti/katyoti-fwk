// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// The module shape the planification session put on trains: a folded
// description and aligned remarks, which a re-encoding unfolded and collapsed.
const authoredTrainModule = `# header note
id: m.x
type: module
name: x
description: >-
  A long folded description that goes on and on across more than one line
  of authored text, well past eighty columns wide.

capabilities:
  checkable:
    kind: none   # m.UC-012
    note: hi     # aligned
`

func writeThrough(t *testing.T, authored string, change func(*api.Object)) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "m.cwm.yaml")
	if err := os.WriteFile(path, []byte(authored), 0o644); err != nil {
		t.Fatal(err)
	}
	obj, err := ReadObjectFile(path)
	if err != nil {
		t.Fatal(err)
	}
	change(&obj)
	if err := WriteObjectFile(path, obj); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	return string(b)
}

// TestAnEdgeLeavesTheAuthoredTextAlone: adding an edge appends it; every
// other line is the one authored, byte for byte.
func TestAnEdgeLeavesTheAuthoredTextAlone(t *testing.T) {
	got := writeThrough(t, authoredTrainModule, func(o *api.Object) {
		o.Edges = append(o.Edges, api.ObjectEdge{Kind: "on-train", To: "t.a"})
	})
	want := authoredTrainModule + "edges:\n  - kind: on-train\n    to: t.a\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestAChangedFieldIsTheOnlyChangedLine: a nested scalar changes on its own
// line, keeping its remark; its siblings and the folded block are untouched.
func TestAChangedFieldIsTheOnlyChangedLine(t *testing.T) {
	got := writeThrough(t, authoredTrainModule, func(o *api.Object) {
		o.Capabilities = cloneCaps(o.Capabilities)
		o.Capabilities["checkable"]["kind"] = "command"
	})
	want := strings.Replace(authoredTrainModule, "    kind: none   # m.UC-012\n", "    kind: command # m.UC-012\n", 1)
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	got = writeThrough(t, authoredTrainModule, func(o *api.Object) {
		o.Capabilities = cloneCaps(o.Capabilities)
		delete(o.Capabilities["checkable"], "note")
	})
	want = strings.Replace(authoredTrainModule, "    note: hi     # aligned\n", "", 1)
	if got != want {
		t.Fatalf("removing a field — got:\n%s\nwant:\n%s", got, want)
	}
}

// TestRewritingTheCorpusChangesNoByte: every object file of the repository,
// read and written back unchanged, is the same file. What a write puts on disk
// is encodeObjectFile's bytes at that path (writeObjectFileRev writes them as
// they are), so each file is encoded where it stands and compared in memory:
// copying and writing six thousand files cost two minutes on Windows and timed
// the package out under a full go test./.... The write itself is
// proven on authored files by the round-trip tests above.
func TestRewritingTheCorpusChangesNoByte(t *testing.T) {
	long.Test(t)
	t.Parallel() // waits mostly on the disk, beside the other corpus tests
	root := filepath.Join("..", "..", "..", "..", "..", "modules")
	var n, differ int
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !(strings.HasSuffix(p, api.ObjectFileSuffix) || strings.HasSuffix(p, api.ObjectFileSuffixMD)) {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		obj, err := decodeObjectFile(p, src)
		if err != nil {
			return nil // not an object file this test is about
		}
		got, err := encodeObjectFile(p, obj)
		if err != nil {
			t.Errorf("%s: %v", p, err)
			return nil
		}
		n++
		if !bytes.Equal(got, src) {
			differ++
			if differ <= 3 && os.Getenv("ZZDIFF") != "" {
				t.Errorf("%s changed on an unchanged write:\n--- authored\n%s\n--- written\n%s", p, src, got)
			}
		}
		return nil
	})
	if n < 100 {
		t.Fatalf("only %d object files found under %s", n, root)
	}
	if differ > 0 {
		t.Errorf("%d of %d object files changed on an unchanged write", differ, n)
	}
}

// TestAReadDuringAWriteDoesNotFail: readers polling an object while it is
// rewritten never see an error (on Windows the replace is a sharing violation,
// or the file briefly missing inside the replace, that a reader retries through
// — atomicio's replaceState).
func TestAReadDuringAWriteDoesNotFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.cwm.yaml")
	if err := os.WriteFile(path, []byte(authoredTrainModule), 0o644); err != nil {
		t.Fatal(err)
	}
	obj, err := ReadObjectFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := ReadObjectFile(path); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	for i := 0; i < 200; i++ {
		obj.Capabilities = cloneCaps(obj.Capabilities)
		obj.Capabilities["checkable"]["note"] = strings.Repeat("x", i%7+1)
		if err := WriteObjectFile(path, obj); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	close(stop)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("a read during a write failed: %v", err)
	}
}
