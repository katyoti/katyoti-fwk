// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"testing"
)

// TestContentStore_residentLayerServesRepeatReads asserts the second read of a
// hash never touches disk. The probe is deleting the file between reads: a
// resident hit still serves it, a disk read could not.
func TestContentStore_residentLayerServesRepeatReads(t *testing.T) {
	dir := t.TempDir()
	s := NewContentStore(dir)
	body := []byte(`{"name":"doc-create"}`)
	hash := HashBody(body)
	if err := s.Put(hash, body); err != nil {
		t.Fatal(err)
	}

	got, ok := s.Get(hash)
	if !ok || string(got) != string(body) {
		t.Fatalf("first Get = %q, %v", got, ok)
	}

	// Remove the backing file. Only the resident layer can answer now.
	name := hashFileName(hash)
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
	got, ok = s.Get(hash)
	if !ok || string(got) != string(body) {
		t.Fatalf("second Get went to disk: %q, %v", got, ok)
	}
	if hits, _ := s.Stats(); hits == 0 {
		t.Error("no resident hits recorded")
	}
}

// TestContentStore_residentLayerCannotServeAWrongEntry asserts the property
// that makes this cache safe without invalidation: the key IS the content, so
// different content is a different key.
func TestContentStore_residentLayerCannotServeAWrongEntry(t *testing.T) {
	s := NewContentStore(t.TempDir())
	v1 := []byte(`{"v":1}`)
	v2 := []byte(`{"v":2}`)
	h1, h2 := HashBody(v1), HashBody(v2)
	if h1 == h2 {
		t.Fatal("distinct bodies hashed the same")
	}
	if err := s.Put(h1, v1); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(h2, v2); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		hash string
		want string
	}{{h1, `{"v":1}`}, {h2, `{"v":2}`}} {
		got, ok := s.Get(tc.hash)
		if !ok || string(got) != tc.want {
			t.Errorf("Get(%s) = %q, %v; want %q", tc.hash[:12], got, ok, tc.want)
		}
	}
}

// TestContentStore_oversizedBodyStaysOnDisk asserts one large body cannot
// evict the working set — it is served, but never admitted.
func TestContentStore_oversizedBodyStaysOnDisk(t *testing.T) {
	dir := t.TempDir()
	s := NewContentStore(dir)
	big := make([]byte, memMaxEntryBytes+1)
	for i := range big {
		big[i] = 'z'
	}
	hash := HashBody(big)
	if err := s.Put(hash, big); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.memGet(hashFileName(hash)); ok {
		t.Error("oversized body was admitted to the resident layer")
	}
	if got, ok := s.Get(hash); !ok || len(got) != len(big) {
		t.Errorf("oversized body should still be served from disk: %d, %v", len(got), ok)
	}
}
