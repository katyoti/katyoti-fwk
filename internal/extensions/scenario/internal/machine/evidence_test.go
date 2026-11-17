// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// : a file system stamps files with the kernel's coarse clock
// (on Linux one tick, a few milliseconds behind time.Now()), so a session a
// stage writes just after it started can carry a time just before the
// stage's start. Stamping the file reproduces that on any OS.
func TestEvidenceKeepsAFileStampedWithinAClockTickBeforeTheStage(t *testing.T) {
	since := time.Now()
	cases := []struct {
		name string
		at   time.Time
		kept bool
	}{
		{"a tick before the start (a coarse clock)", since.Add(-5 * time.Millisecond), true},
		{"after the start", since.Add(time.Millisecond), true},
		{"an hour before the start (an earlier stage)", since.Add(-time.Hour), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "s4_generate")
			p := filepath.Join(dir, "llm.jsonl")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("journal"), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, f := range []string{p, dir} {
				if err := os.Chtimes(f, c.at, c.at); err != nil {
					t.Fatal(err)
				}
			}
			if got := writtenSince(dir, since); got != c.kept {
				t.Errorf("writtenSince = %v, want %v", got, c.kept)
			}
		})
	}
}
