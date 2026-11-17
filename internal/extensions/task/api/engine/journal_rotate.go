// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// A SESSION'S JOURNAL IS BOUNDED — a run that lives for ever (a kept-alive
// task) writes for ever: past maxJournalBytes a journal file is rotated
// (llm.jsonl → llm.1.jsonl → llm.2.jsonl), the oldest dropped. Its readers see
// the recent part; its numbering goes on (lastSeq reads the rotated file too).

// maxJournalBytes is the rotation limit (atomic: tests change it while runs
// of an earlier test still write).
var maxJournalBytes = func() *atomic.Int64 { v := &atomic.Int64{}; v.Store(64 << 20); return v }()

const keptJournals = 2

// rotatingFile is a journal's file with its size.
type rotatingFile struct {
	file  *os.File
	path  string
	size  int64
	sized bool
}

// write appends line, rotating first when it would pass the limit. Called with
// the journal's mutex held.
func (r *rotatingFile) write(line []byte) error {
	if r.file == nil {
		return nil
	}
	if !r.sized {
		if st, err := r.file.Stat(); err == nil {
			r.size = st.Size()
		}
		r.sized = true
	}
	if limit := maxJournalBytes.Load(); r.size > 0 && r.size+int64(len(line)) > limit {
		r.rotate()
	}
	n, err := r.file.Write(line)
	r.size += int64(n)
	return err
}

// rotate moves the journal aside and opens a fresh one. If the file cannot be
// moved (Windows: a reader has it open), writing goes on in it and the next
// try is a whole limit later.
func (r *rotatingFile) rotate() {
	_ = r.file.Close()
	err := shiftJournals(r.path)
	f, openErr := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if openErr != nil {
		r.file = nil
		return
	}
	r.file = f
	if err == nil {
		r.size = 0
	} else {
		r.size = -maxJournalBytes.Load() // retry a whole limit later
	}
}

func rotatedPath(path string, i int) string {
	return fmt.Sprintf("%s.%d.jsonl", strings.TrimSuffix(path, ".jsonl"), i)
}

func shiftJournals(path string) error {
	_ = os.Remove(rotatedPath(path, keptJournals))
	for i := keptJournals - 1; i >= 1; i-- {
		_ = os.Rename(rotatedPath(path, i), rotatedPath(path, i+1))
	}
	return os.Rename(path, rotatedPath(path, 1))
}

func (r *rotatingFile) close() error {
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

// SetMaxJournalBytesForTest sets the rotation limit and returns the previous
// one (tests in other packages).
func SetMaxJournalBytesForTest(n int64) int64 {
	return maxJournalBytes.Swap(n)
}
