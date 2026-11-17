// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// content_stream.go — moving content without holding it.
//
// Content addressing is the envelope's idea, so the operations that compute and
// check an address live with it rather than being re-implemented by each
// transport. Every function here STREAMS: a multi-gigabyte layer costs a buffer,
// not its own size. That is the whole reason content of arbitrary size can exist
// in this model at all.

// StreamToFile writes r to path, verifying the content against expectHash as it
// goes, and returns the number of bytes written.
//
// Verification happens DURING the copy, not after: nothing is read twice, and
// content never has to be resident to be checked. A mismatch removes the file —
// content that fails its own address is worse than no content, because a
// consumer would use it.
//
// The write lands beside the destination and is renamed into place, so the
// destination path never names a partial or unverified file; a reader racing
// the write sees either the old content or the whole new content.
//
// An empty expectHash skips verification, for a caller that HAS no address to
// check against. That is not the same as a caller that has one and ignores it.
func StreamToFile(path, expectHash string, r io.Reader) (int64, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("content: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".part-*")
	if err != nil {
		return 0, fmt.Errorf("content: create temp for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	fail := func(e error) (int64, error) {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return 0, e
	}

	var sum hash.Hash
	dst := io.Writer(tmp)
	if expectHash != "" {
		sum = sha256.New()
		dst = io.MultiWriter(tmp, sum)
	}
	n, err := io.Copy(dst, r)
	if err != nil {
		return fail(fmt.Errorf("content: stream to %s: %w", path, err))
	}
	if sum != nil {
		want := strings.TrimPrefix(expectHash, "sha256:")
		if got := hex.EncodeToString(sum.Sum(nil)); got != want {
			return fail(fmt.Errorf("content: does not match its address (want %s, got sha256:%s)", expectHash, got))
		}
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return 0, fmt.Errorf("content: close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return 0, fmt.Errorf("content: rename to %s: %w", path, err)
	}
	return n, nil
}

// HashReader returns the content address of everything read from r, and how
// many bytes that was, without holding any of it. Used where content arrives as
// a stream and its address must be computed rather than trusted.
func HashReader(r io.Reader) (string, int64, error) {
	sum := sha256.New()
	n, err := io.Copy(sum, r)
	if err != nil {
		return "", 0, err
	}
	return "sha256:" + hex.EncodeToString(sum.Sum(nil)), n, nil
}
