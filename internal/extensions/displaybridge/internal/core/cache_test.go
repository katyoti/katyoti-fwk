// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strconv"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// stubSchema returns a compiled JSON Schema for use as a cache value.
// The schema body is intentionally minimal (object with no constraints)
// because the cache treats values as opaque — the schema's contents do
// not affect cache mechanics.
func stubSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	url := "test://" + name
	if err := c.AddResource(url, strings.NewReader(`{"type":"object"}`)); err != nil {
		t.Fatalf("AddResource: %v", err)
	}
	s, err := c.Compile(url)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return s
}

func TestSchemaCache_PutAndGet(t *testing.T) {
	c := newSchemaCache(8)
	s := stubSchema(t, "User")
	c.Put("User", 3, s)

	got, ver, ok := c.Get("User")
	if !ok {
		t.Fatal("Get(User) ok=false, want true")
	}
	if got != s {
		t.Error("Get returned a different schema instance")
	}
	if ver != 3 {
		t.Errorf("version = %d, want 3", ver)
	}
}

func TestSchemaCache_Miss(t *testing.T) {
	c := newSchemaCache(8)
	if _, _, ok := c.Get("absent"); ok {
		t.Error("Get(absent) ok=true, want false")
	}
}

func TestSchemaCache_PutRefreshesExisting(t *testing.T) {
	c := newSchemaCache(8)
	s1 := stubSchema(t, "User")
	s2 := stubSchema(t, "User")
	c.Put("User", 1, s1)
	c.Put("User", 2, s2)

	got, ver, ok := c.Get("User")
	if !ok || got != s2 || ver != 2 {
		t.Errorf("after refresh: got=%v ver=%d ok=%v, want s2/2/true", got, ver, ok)
	}
	if c.Size() != 1 {
		t.Errorf("Size = %d, want 1 (refresh must not add a new entry)", c.Size())
	}
}

func TestSchemaCache_LRUEviction(t *testing.T) {
	c := newSchemaCache(3)
	c.Put("a", 1, stubSchema(t, "a"))
	c.Put("b", 1, stubSchema(t, "b"))
	c.Put("c", 1, stubSchema(t, "c"))

	// Touch "a" so it becomes MRU; "b" is now LRU.
	if _, _, ok := c.Get("a"); !ok {
		t.Fatal("Get(a) ok=false")
	}

	// Insert a 4th entry → "b" should be evicted (LRU after the touch).
	c.Put("d", 1, stubSchema(t, "d"))

	if _, _, ok := c.Get("b"); ok {
		t.Error("expected b evicted; still cached")
	}
	for _, name := range []string{"a", "c", "d"} {
		if _, _, ok := c.Get(name); !ok {
			t.Errorf("expected %s present; missing", name)
		}
	}
	if c.Size() != 3 {
		t.Errorf("Size = %d, want 3", c.Size())
	}
}

func TestSchemaCache_Invalidate(t *testing.T) {
	c := newSchemaCache(8)
	c.Put("a", 1, stubSchema(t, "a"))
	c.Put("b", 1, stubSchema(t, "b"))

	c.Invalidate("a")
	if _, _, ok := c.Get("a"); ok {
		t.Error("a should be gone after Invalidate")
	}
	if _, _, ok := c.Get("b"); !ok {
		t.Error("b should still be cached")
	}

	// No-op when missing.
	c.Invalidate("absent") // must not panic
}

func TestSchemaCache_Clear(t *testing.T) {
	c := newSchemaCache(8)
	c.Put("a", 1, stubSchema(t, "a"))
	c.Put("b", 1, stubSchema(t, "b"))

	c.Clear()
	if c.Size() != 0 {
		t.Errorf("Size = %d after Clear, want 0", c.Size())
	}
	if _, _, ok := c.Get("a"); ok {
		t.Error("a present after Clear")
	}
}

func TestSchemaCache_DefaultCapacityWhenNonPositive(t *testing.T) {
	c0 := newSchemaCache(0)
	if c0.capacity != schemaCacheCapacity {
		t.Errorf("capacity = %d, want default %d", c0.capacity, schemaCacheCapacity)
	}
	cNeg := newSchemaCache(-5)
	if cNeg.capacity != schemaCacheCapacity {
		t.Errorf("negative cap → %d, want default %d", cNeg.capacity, schemaCacheCapacity)
	}
}

func TestSchemaCache_LRU_FillBeyondCapacity(t *testing.T) {
	const cap = 10
	c := newSchemaCache(cap)
	for i := 0; i < cap*3; i++ {
		name := "s" + strconv.Itoa(i)
		c.Put(name, 1, stubSchema(t, name))
	}
	if c.Size() != cap {
		t.Errorf("Size = %d, want %d (capacity bound)", c.Size(), cap)
	}
	// The first cap*2 entries should have been evicted.
	for i := 0; i < cap*2; i++ {
		name := "s" + strconv.Itoa(i)
		if _, _, ok := c.Get(name); ok {
			t.Errorf("unexpectedly present: %s", name)
		}
	}
	// The last cap entries should remain.
	for i := cap * 2; i < cap*3; i++ {
		name := "s" + strconv.Itoa(i)
		if _, _, ok := c.Get(name); !ok {
			t.Errorf("expected present: %s", name)
		}
	}
}
