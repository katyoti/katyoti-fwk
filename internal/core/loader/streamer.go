// SPDX-License-Identifier: Apache-2.0

package loader

import (
	"io"
	"os"
	"path/filepath"
	"time"
)

// streamer.go — reading a file that is too large to return.
//
// `Loader.ReadFile` returns `[]byte`, which is the right shape for the content
// this system is mostly made of: skills, tasks, schemas, configs — all small,
// all cheaper to hold than to re-read. It is the wrong shape for a file in the
// bulk band, where returning the content IS the problem: a multi-gigabyte
// dataset would be read into memory, and on `FSLoader` cached there, just to
// compute its address.
//
// So this is an OPTIONAL interface rather than four more methods on `Loader`.
// Only a source that can genuinely answer these implements it — the filesystem
// can seek and stat; an embedded FS is already resident and has nothing to
// gain; a compound loader answers for whichever member holds the file.
// Widening `Loader` would force every implementation to answer a question only
// some of them have.

// Streamer is implemented by a loader whose files can be sized and read
// incrementally.
//
// A caller asks for it and falls back to `ReadFile` when it is absent, which is
// what keeps this additive: nothing that works today stops working because its
// loader does not stream.
type Streamer interface {
	// StatFile reports what is known about a file without reading it. `ok`
	// is false when the file does not exist or is a directory — an absent
	// file is a normal answer here, not an error, matching ReadFile's nil-nil
	// convention.
	StatFile(path string) (st FileStat, ok bool)

	// OpenFile returns a reader over the file. The caller closes it.
	//
	// Nothing is read when it returns, so a caller that changes its mind —
	// having learnt the size from StatFile, say — pays for the handle and
	// not the content.
	OpenFile(path string) (io.ReadCloser, error)
}

// FileStat is what a Streamer knows about a file before reading it.
//
// Size decides the band; ModTime, with Size, is the stamp a caller keys a
// derived value on — a content address, say — so a file that has not changed
// is not streamed again to learn what it already told us.
type FileStat struct {
	Size int64
	// ModTime is the file's modification time; zero when the source keeps
	// none, in which case Size alone is the stamp.
	ModTime time.Time
}

// StatFile implements Streamer.
func (l *FSLoader) StatFile(name string) (FileStat, bool) {
	fi, err := os.Stat(filepath.Join(l.rootDir, filepath.FromSlash(name)))
	if err != nil || fi.IsDir() {
		return FileStat{}, false
	}
	return FileStat{Size: fi.Size(), ModTime: fi.ModTime()}, true
}

// OpenFile implements Streamer.
//
// Deliberately does NOT consult the byte cache. A caller reaching for a stream
// wants the content it is about to read, not a copy of it held for later — and
// populating the cache from here would defeat the reason this exists.
func (l *FSLoader) OpenFile(name string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(l.rootDir, filepath.FromSlash(name)))
}

var _ Streamer = (*FSLoader)(nil)
