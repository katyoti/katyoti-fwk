// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// A BINARY'S LOG IS BOUNDED — the long-lived core and every command-line call
// append to <logs>/<name>.log: past maxLogBytes it is rotated to <name>.log.1
// (one previous file kept, the older one dropped), at open and while writing.
// Several processes write one file: each checks the file's real size now and
// then, and reopens it when another process rotated it away. Each process
// opens it so that it can be moved while held (logopen_windows.go); when a
// move still fails, writing goes on in it and the next try is a whole limit
// later.

// maxLogBytes is the rotation limit (atomic: tests lower it).
var maxLogBytes = func() *atomic.Int64 { v := &atomic.Int64{}; v.Store(32 << 20); return v }()

// logRecheckEvery is how many writes pass between two reads of the file's
// real size (other processes append to it too).
const logRecheckEvery = 256

// rotatingLog is a log file that rotates past maxLogBytes.
type rotatingLog struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	size   int64
	writes int
}

// openRotatingLog opens path for appending, rotating it first when it is
// already past the limit.
func openRotatingLog(path string) (*rotatingLog, error) {
	if st, err := os.Stat(path); err == nil && st.Size() >= maxLogBytes.Load() {
		_ = shiftLog(path)
	}
	r := &rotatingLog{path: path}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *rotatingLog) open() error {
	f, err := openLogFile(r.path)
	if err != nil {
		return err
	}
	r.file = f
	r.size = 0
	if st, err := f.Stat(); err == nil {
		r.size = st.Size()
	}
	return nil
}

// Write appends p, rotating first when it would pass the limit.
func (r *rotatingLog) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		if err := r.open(); err != nil {
			return len(p), nil // best effort: a log that cannot open drops the line
		}
	}
	r.writes++
	if r.writes%logRecheckEvery == 0 {
		r.recheck()
	}
	if limit := maxLogBytes.Load(); r.size > 0 && r.size+int64(len(p)) > limit {
		r.rotate()
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

// recheck reads the file's real size, and reopens it when another process
// rotated it away.
func (r *rotatingLog) recheck() {
	held, err := r.file.Stat()
	if err != nil {
		return
	}
	if cur, err := os.Stat(r.path); err != nil || !os.SameFile(held, cur) {
		_ = r.file.Close()
		r.file = nil
		_ = r.open()
		return
	}
	if r.size >= 0 {
		r.size = held.Size()
	}
}

// rotate moves the log aside and opens a fresh one; when it cannot be moved,
// writing goes on in it and the next try is a whole limit later.
func (r *rotatingLog) rotate() {
	_ = r.file.Close()
	r.file = nil
	moved := shiftLog(r.path) == nil
	if err := r.open(); err != nil {
		return
	}
	if !moved {
		r.size = -maxLogBytes.Load()
	}
}

// shiftLog keeps one previous file: <path>.1 is replaced by <path>. The old
// <path>.1 is first moved to a name of its own and deleted - another process
// may still hold it, and Windows replaces no file held open but moves one, and
// deletes it once its last holder closes.
func shiftLog(path string) error {
	prev := path + ".1"
	if _, err := os.Stat(prev); err == nil {
		gone := fmt.Sprintf("%s.%d-%d.old", prev, os.Getpid(), time.Now().UnixNano())
		if err := os.Rename(prev, gone); err != nil {
			return err
		}
		_ = os.Remove(gone)
	}
	return os.Rename(path, prev)
}
