// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"testing"
)

type cachedBody struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// TestDecodeCached_sameHashDecodesOnce asserts a second decode of the same
// content into the same type does not re-parse. The probe is a body whose
// bytes are replaced with garbage between calls: a cache hit returns the
// first decode, a miss would error.
func TestDecodeCached_sameHashDecodesOnce(t *testing.T) {
	const hash = "sha256:decode-cache-test-1"
	first, err := decodeCached[cachedBody](hash, []byte(`{"name":"alpha"}`))
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "alpha" {
		t.Fatalf("first decode = %+v", first)
	}
	again, err := decodeCached[cachedBody](hash, []byte(`not json at all`))
	if err != nil {
		t.Fatalf("second call re-parsed instead of hitting the cache: %v", err)
	}
	if again.Name != "alpha" {
		t.Fatalf("cached decode = %+v", again)
	}
}

// TestDecodeCached_keyedByType asserts the target type is part of the key —
// the same bytes decoded into two different types must not collide.
func TestDecodeCached_keyedByType(t *testing.T) {
	const hash = "sha256:decode-cache-test-2"
	body := []byte(`{"name":"beta","tags":["x"]}`)

	a, err := decodeCached[cachedBody](hash, body)
	if err != nil {
		t.Fatal(err)
	}
	type other struct {
		Name string `json:"name"`
	}
	b, err := decodeCached[other](hash, body)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "beta" || b.Name != "beta" || len(a.Tags) != 1 {
		t.Fatalf("a = %+v, b = %+v", a, b)
	}
}

// TestDecodeCached_changedContentIsNotStale asserts an edit is seen: different
// bytes hash differently, so they are a different key and no invalidation is
// needed. This is what makes a mutable source safe to cache.
func TestDecodeCached_changedContentIsNotStale(t *testing.T) {
	before := []byte(`{"name":"v1"}`)
	after := []byte(`{"name":"v2"}`)

	v1, err := decodeCached[cachedBody]("sha256:edit-before", before)
	if err != nil {
		t.Fatal(err)
	}
	v2, err := decodeCached[cachedBody]("sha256:edit-after", after)
	if err != nil {
		t.Fatal(err)
	}
	if v1.Name != "v1" || v2.Name != "v2" {
		t.Fatalf("v1 = %+v, v2 = %+v", v1, v2)
	}
}

// TestDecodeCached_unaddressedContentIsNotCached asserts a body with no
// content address is decoded every time rather than cached under a key that
// cannot identify it.
func TestDecodeCached_unaddressedContentIsNotCached(t *testing.T) {
	a, err := decodeCached[cachedBody]("", []byte(`{"name":"one"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := decodeCached[cachedBody]("", []byte(`{"name":"two"}`))
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "one" || b.Name != "two" {
		t.Fatalf("a = %+v, b = %+v", a, b)
	}
	if _, err := decodeCached[cachedBody]("", []byte(`nope`)); err == nil {
		t.Error("malformed unaddressed body should error, not be cached")
	}
}

// TestDecodeCached_matchesPlainUnmarshal asserts the cached path produces
// exactly what json.Unmarshal would.
func TestDecodeCached_matchesPlainUnmarshal(t *testing.T) {
	body := []byte(`{"name":"gamma","tags":["p","q"]}`)
	var want cachedBody
	if err := json.Unmarshal(body, &want); err != nil {
		t.Fatal(err)
	}
	got, err := decodeCached[cachedBody]("sha256:parity", body)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || len(got.Tags) != len(want.Tags) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
