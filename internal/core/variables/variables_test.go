// SPDX-License-Identifier: Apache-2.0

package variables

import (
	"os"
	"testing"
)

func TestExpandVars_RegisteredDomain(t *testing.T) {
	RegisterResolver("test", func(key string) (string, bool) {
		if key == "hello" {
			return "world", true
		}
		return "", false
	})
	defer func() {
		mu.Lock()
		delete(resolvers, "test")
		mu.Unlock()
	}()

	result := ExpandVars("key: ${{test:hello}}")
	if result != "key: world" {
		t.Fatalf("got %q, want %q", result, "key: world")
	}
}

func TestExpandVars_UnresolvedKey(t *testing.T) {
	RegisterResolver("test2", func(key string) (string, bool) {
		return "", false
	})
	defer func() {
		mu.Lock()
		delete(resolvers, "test2")
		mu.Unlock()
	}()

	result := ExpandVars("key: ${{test2:missing}}")
	if result != "key: ${{test2:missing}}" {
		t.Fatalf("got %q, want placeholder kept", result)
	}
}

func TestExpandVars_NoDomain(t *testing.T) {
	result := ExpandVars("key: ${{nodomain}}")
	if result != "key: ${{nodomain}}" {
		t.Fatalf("got %q, want placeholder kept", result)
	}
}

func TestExpandVars_UnknownDomain(t *testing.T) {
	result := ExpandVars("key: ${{vault:my-secret}}")
	if result != "key: ${{vault:my-secret}}" {
		t.Fatalf("got %q, want placeholder kept", result)
	}
}

func TestExpandVars_MultipleVars(t *testing.T) {
	RegisterResolver("m", func(key string) (string, bool) {
		switch key {
		case "a":
			return "A", true
		case "b":
			return "B", true
		}
		return "", false
	})
	defer func() {
		mu.Lock()
		delete(resolvers, "m")
		mu.Unlock()
	}()

	result := ExpandVars("${{m:a}}-${{m:b}}")
	if result != "A-B" {
		t.Fatalf("got %q, want %q", result, "A-B")
	}
}

func TestExpandVars_NoVars(t *testing.T) {
	result := ExpandVars("plain text with no variables")
	if result != "plain text with no variables" {
		t.Fatalf("got %q", result)
	}
}

func TestExpandVars_DoesNotMatchSingleBrace(t *testing.T) {
	os.Setenv("TEST_VAR_SINGLE", "val")
	defer os.Unsetenv("TEST_VAR_SINGLE")

	result := ExpandVars("key: ${env:TEST_VAR_SINGLE}")
	if result != "key: ${env:TEST_VAR_SINGLE}" {
		t.Fatalf("single-brace should not match, got %q", result)
	}
}

func TestExpandVarsBytes(t *testing.T) {
	RegisterResolver("b", func(key string) (string, bool) {
		if key == "x" {
			return "X", true
		}
		return "", false
	})
	defer func() {
		mu.Lock()
		delete(resolvers, "b")
		mu.Unlock()
	}()

	result := ExpandVarsBytes([]byte("val=${{b:x}}"))
	if string(result) != "val=X" {
		t.Fatalf("got %q", string(result))
	}
}

func TestRegisterResolver_PanicOnDuplicate(t *testing.T) {
	RegisterResolver("dup", func(key string) (string, bool) { return "", false })
	defer func() {
		mu.Lock()
		delete(resolvers, "dup")
		mu.Unlock()
	}()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()
	RegisterResolver("dup", func(key string) (string, bool) { return "", false })
}

func TestHasResolver(t *testing.T) {
	if HasResolver("nonexistent") {
		t.Fatal("should not have resolver")
	}
	RegisterResolver("exists", func(key string) (string, bool) { return "", false })
	defer func() {
		mu.Lock()
		delete(resolvers, "exists")
		mu.Unlock()
	}()
	if !HasResolver("exists") {
		t.Fatal("should have resolver")
	}
}

// Keys names each key of one domain a text reads, once, in order; another
// domain's placeholder and a placeholder without a domain are not its keys.
func TestKeysNamesEachKeyOfItsDomainOnce(t *testing.T) {
	data := []byte("base: ${{env:B_URL}}\nkey: ${{env:A_KEY}}\nagain: ${{env:B_URL}}\nvault: ${{secret:S}}\nbare: ${{NODOMAIN}}\n")
	got := Keys(data, "env")
	if len(got) != 2 || got[0] != "B_URL" || got[1] != "A_KEY" {
		t.Fatalf("Keys = %v, want [B_URL A_KEY]", got)
	}
	if got := Keys(data, "secret"); len(got) != 1 || got[0] != "S" {
		t.Fatalf("Keys(secret) = %v, want [S]", got)
	}
	if got := Keys([]byte("nothing here"), "env"); len(got) != 0 {
		t.Fatalf("Keys of a text without placeholders = %v", got)
	}
}
