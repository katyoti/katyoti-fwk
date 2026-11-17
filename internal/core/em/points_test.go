// SPDX-License-Identifier: Apache-2.0

package em

import (
	"errors"
	"testing"
)

func TestListPoint_AppendAndEntries(t *testing.T) {
	p := NewListPoint[string]()

	if err := p.Contribute("a"); err != nil {
		t.Fatal(err)
	}
	if err := p.Contribute("b"); err != nil {
		t.Fatal(err)
	}
	if err := p.Contribute("c"); err != nil {
		t.Fatal(err)
	}

	entries := p.Entries()
	if len(entries) != 3 {
		t.Fatalf("Entries() len = %d, want 3", len(entries))
	}
	if entries[0] != "a" || entries[1] != "b" || entries[2] != "c" {
		t.Fatalf("Entries() = %v, want [a b c]", entries)
	}
}

func TestListPoint_EntriesReturnsCopy(t *testing.T) {
	p := NewListPoint[string]()
	p.Contribute("x")

	entries := p.Entries()
	entries[0] = "modified"

	original := p.Entries()
	if original[0] != "x" {
		t.Fatal("Entries() did not return a copy — mutation leaked")
	}
}

func TestListPoint_Empty(t *testing.T) {
	p := NewListPoint[int]()
	entries := p.Entries()
	if len(entries) != 0 {
		t.Fatalf("Entries() len = %d, want 0", len(entries))
	}
}

// --- IndexedPoint tests ---

type named struct {
	Name  string
	Value int
}

func nameKey(n named) string { return n.Name }

func TestIndexedPoint_AddAndGet(t *testing.T) {
	p := NewIndexedPoint[named](nameKey)

	if err := p.Contribute(named{"alpha", 1}); err != nil {
		t.Fatal(err)
	}
	if err := p.Contribute(named{"beta", 2}); err != nil {
		t.Fatal(err)
	}

	got := p.Get("alpha")
	if got.Value != 1 {
		t.Fatalf("Get(alpha) = %v; want {alpha 1}", got)
	}

	got = p.Get("beta")
	if got.Value != 2 {
		t.Fatalf("Get(beta) = %v; want {beta 2}", got)
	}

	got = p.Get("gamma")
	if got.Name != "" {
		t.Fatalf("Get(gamma) = %v; want zero value", got)
	}
}

func TestIndexedPoint_Has(t *testing.T) {
	p := NewIndexedPoint[named](nameKey)
	p.Contribute(named{"x", 1})

	if !p.Has("x") {
		t.Fatal("Has(x) = false, want true")
	}
	if p.Has("y") {
		t.Fatal("Has(y) = true, want false")
	}
}

func TestIndexedPoint_Keys(t *testing.T) {
	p := NewIndexedPoint[named](nameKey)
	p.Contribute(named{"c", 3})
	p.Contribute(named{"a", 1})
	p.Contribute(named{"b", 2})

	keys := p.Keys()
	if len(keys) != 3 {
		t.Fatalf("Keys() len = %d, want 3", len(keys))
	}
	// Insertion order preserved
	if keys[0] != "c" || keys[1] != "a" || keys[2] != "b" {
		t.Fatalf("Keys() = %v, want [c a b]", keys)
	}
}

func TestIndexedPoint_Entries_InsertionOrder(t *testing.T) {
	p := NewIndexedPoint[named](nameKey)
	p.Contribute(named{"z", 26})
	p.Contribute(named{"a", 1})

	entries := p.Entries()
	if len(entries) != 2 {
		t.Fatalf("Entries() len = %d, want 2", len(entries))
	}
	if entries[0].Name != "z" || entries[1].Name != "a" {
		t.Fatalf("Entries() = %v, want [{z 26} {a 1}]", entries)
	}
}

func TestIndexedPoint_DuplicateKey_Error(t *testing.T) {
	p := NewIndexedPoint[named](nameKey)
	if err := p.Contribute(named{"x", 1}); err != nil {
		t.Fatal(err)
	}

	err := p.Contribute(named{"x", 2})
	if err == nil {
		t.Fatal("expected ErrDuplicateKey, got nil")
	}

	var dupErr ErrDuplicateKey
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected ErrDuplicateKey, got %T: %v", err, err)
	}
	if dupErr.Key != "x" {
		t.Fatalf("ErrDuplicateKey.Key = %q, want %q", dupErr.Key, "x")
	}
}

func TestIndexedPoint_AllowReplace(t *testing.T) {
	p := NewIndexedPoint[named](nameKey, AllowReplace())

	if err := p.Contribute(named{"x", 1}); err != nil {
		t.Fatal(err)
	}
	if err := p.Contribute(named{"x", 99}); err != nil {
		t.Fatalf("AllowReplace should permit overwrite, got: %v", err)
	}

	got := p.Get("x")
	if got.Value != 99 {
		t.Fatalf("Get(x).Value = %d, want 99", got.Value)
	}

	// Keys should not have duplicates
	keys := p.Keys()
	if len(keys) != 1 {
		t.Fatalf("Keys() len = %d after replace, want 1", len(keys))
	}
}

func TestIndexedPoint_EntriesMap(t *testing.T) {
	p := NewIndexedPoint[named](nameKey)
	p.Contribute(named{"alpha", 1})
	p.Contribute(named{"beta", 2})

	m := p.EntriesMap()
	if len(m) != 2 {
		t.Fatalf("EntriesMap() len = %d, want 2", len(m))
	}
	if m["alpha"].Value != 1 {
		t.Fatalf("EntriesMap()[alpha].Value = %d, want 1", m["alpha"].Value)
	}
	if m["beta"].Value != 2 {
		t.Fatalf("EntriesMap()[beta].Value = %d, want 2", m["beta"].Value)
	}

	// Returned map is a copy — mutations don't affect the point
	m["alpha"] = named{"alpha", 99}
	got := p.Get("alpha")
	if got.Value != 1 {
		t.Fatal("EntriesMap() did not return a copy — mutation leaked")
	}
}
