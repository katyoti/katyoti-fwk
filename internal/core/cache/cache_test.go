// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileCache_SetGet(t *testing.T) {
	dir := t.TempDir()
	c := NewFileCache(dir)

	type data struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	// Set.
	err := c.Set("test", "key1", data{Name: "hello", Value: 42})
	if err != nil {
		t.Fatal(err)
	}

	// Verify file exists.
	path := filepath.Join(dir, "test", HashKey("key1")+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}

	// Get.
	var got data
	found, err := c.Get("test", "key1", &got)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected cache hit")
	}
	if got.Name != "hello" || got.Value != 42 {
		t.Errorf("got %+v", got)
	}
}

func TestFileCache_Miss(t *testing.T) {
	dir := t.TempDir()
	c := NewFileCache(dir)

	var got string
	found, err := c.Get("test", "nonexistent", &got)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected cache miss")
	}
}

func TestFileCache_CorruptedEntry(t *testing.T) {
	dir := t.TempDir()
	c := NewFileCache(dir)

	// Write corrupted data directly.
	ns := filepath.Join(dir, "test")
	os.MkdirAll(ns, 0o755)
	path := filepath.Join(ns, HashKey("bad")+".json")
	os.WriteFile(path, []byte("not json!!!"), 0o644)

	var got string
	found, err := c.Get("test", "bad", &got)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("corrupted entry should be treated as miss")
	}
}

func TestCached_HitSkipsFn(t *testing.T) {
	dir := t.TempDir()
	c := NewFileCache(dir)

	calls := 0
	fn := func() (string, error) {
		calls++
		return "computed", nil
	}

	// First call — miss, fn executes.
	r1, err := Cached(c, "ns", "k", fn)
	if err != nil {
		t.Fatal(err)
	}
	if r1 != "computed" {
		t.Errorf("r1 = %q", r1)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}

	// Second call — hit, fn NOT called.
	r2, err := Cached(c, "ns", "k", fn)
	if err != nil {
		t.Fatal(err)
	}
	if r2 != "computed" {
		t.Errorf("r2 = %q", r2)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (should be cached)", calls)
	}
}

func TestCached_ErrorNotCached(t *testing.T) {
	dir := t.TempDir()
	c := NewFileCache(dir)

	calls := 0
	fn := func() (string, error) {
		calls++
		return "", fmt.Errorf("network error")
	}

	// First call — error, not cached.
	_, err := Cached(c, "ns", "k", fn)
	if err == nil {
		t.Fatal("expected error")
	}

	// Second call — fn called again (error was not cached).
	_, _ = Cached(c, "ns", "k", fn)
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (errors should not be cached)", calls)
	}
}

func TestCached_NilCache(t *testing.T) {
	fn := func() (string, error) { return "result", nil }
	r, err := Cached[string](nil, "ns", "k", fn)
	if err != nil {
		t.Fatal(err)
	}
	if r != "result" {
		t.Errorf("r = %q", r)
	}
}

func TestNopCache(t *testing.T) {
	c := NopCache{}
	_ = c.Set("ns", "k", "value")
	var got string
	found, _ := c.Get("ns", "k", &got)
	if found {
		t.Error("NopCache should never hit")
	}
}

func TestMakeKey_Deterministic(t *testing.T) {
	k1 := MakeKey("scrape", map[string]string{"url": "https://example.com"})
	k2 := MakeKey("scrape", map[string]string{"url": "https://example.com"})
	if k1 != k2 {
		t.Errorf("keys differ: %q != %q", k1, k2)
	}
}

func TestMakeKey_DifferentInputs(t *testing.T) {
	k1 := MakeKey("scrape", "https://a.example")
	k2 := MakeKey("scrape", "https://b.example")
	if k1 == k2 {
		t.Error("different inputs should produce different keys")
	}
}

func TestCleanKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Hello World", "hello world"},
		{"  multiple   spaces  ", "multiple spaces"},
		{"UPPERCASE", "uppercase"},
		{"", ""},
	}
	for _, tt := range tests {
		got := CleanKey(tt.in)
		if got != tt.want {
			t.Errorf("CleanKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFileCache_TTL_Expired(t *testing.T) {
	dir := t.TempDir()
	// TTL of 1 millisecond — everything expires instantly.
	c := NewFileCacheWithTTL(dir, 1*time.Millisecond)

	_ = c.Set("test", "k", "value")
	// Wait for expiry.
	time.Sleep(5 * time.Millisecond)

	var got string
	found, err := c.Get("test", "k", &got)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected miss — entry should be expired")
	}
}

func TestFileCache_TTL_Fresh(t *testing.T) {
	dir := t.TempDir()
	c := NewFileCacheWithTTL(dir, 1*time.Hour)

	_ = c.Set("test", "k", "value")

	var got string
	found, err := c.Get("test", "k", &got)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("expected hit — entry should still be fresh")
	}
	if got != "value" {
		t.Errorf("got %q", got)
	}
}

func TestFileCache_ZeroTTL_NeverExpires(t *testing.T) {
	dir := t.TempDir()
	c := NewFileCache(dir) // zero TTL = no expiry

	_ = c.Set("test", "k", "value")

	var got string
	found, _ := c.Get("test", "k", &got)
	if !found {
		t.Error("zero TTL should never expire")
	}
}

func TestHashKey_Stable(t *testing.T) {
	h1 := HashKey("test-input")
	h2 := HashKey("test-input")
	if h1 != h2 {
		t.Error("hash should be stable")
	}
	if len(h1) != 64 { // SHA256 hex = 64 chars
		t.Errorf("hash len = %d, want 64", len(h1))
	}
}
