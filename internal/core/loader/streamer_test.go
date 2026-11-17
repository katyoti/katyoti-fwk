// SPDX-License-Identifier: Apache-2.0

package loader

import (
	"os"
	"path/filepath"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// A file in the bulk band must be addressable without being held. The whole
// point of the band is content too large to fit in memory, so computing its
// hash through ReadFile would defeat the reason it exists.
func TestStreamerAddressesAFileWithoutHoldingIt(t *testing.T) {
	root := t.TempDir()
	content := make([]byte, 3<<20) // 3 MiB — past BandControl
	for i := range content {
		content[i] = byte(i)
	}
	if err := os.WriteFile(filepath.Join(root, "big.bin"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	l := NewFSLoader(root)

	st, ok := l.StatFile("big.bin")
	if !ok {
		t.Fatal("StatFile did not find the file")
	}
	size := st.Size
	if size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", size, len(content))
	}
	if st.ModTime.IsZero() {
		t.Error("StatFile reports no modification time for a file on disk")
	}
	if contribapi.BandOf(size) != contribapi.BandNameBulk {
		t.Fatalf("a %d-byte file is not in the bulk band", size)
	}

	rc, err := l.OpenFile("big.bin")
	if err != nil {
		t.Fatal(err)
	}
	hash, n, err := contribapi.HashReader(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if n != size {
		t.Errorf("streamed %d bytes, stat said %d", n, size)
	}
	if want := contribapi.HashContent(content); hash != want {
		t.Errorf("streamed hash %s != whole-content hash %s", hash, want)
	}

	// And the stream did not populate the read cache: a caller reaching for a
	// stream is avoiding residency, not asking for it later.
	if _, cached := l.cached("big.bin"); cached {
		t.Error("OpenFile populated the byte cache")
	}
}

func TestStreamerReportsAnAbsentFileRatherThanErroring(t *testing.T) {
	l := NewFSLoader(t.TempDir())
	if _, ok := l.StatFile("nope.bin"); ok {
		t.Error("StatFile claimed a missing file exists")
	}
	rc, err := l.OpenFile("nope.bin")
	if err == nil {
		_ = rc.Close()
		t.Error("OpenFile opened a missing file")
	}
}

// A loader that cannot stream is the normal case, and callers fall back. The
// interface assertion is what keeps that check honest.
func TestNotEveryLoaderStreams(t *testing.T) {
	var l Loader = NewFSLoader(t.TempDir())
	if _, ok := l.(Streamer); !ok {
		t.Error("FSLoader no longer satisfies Streamer")
	}
}
