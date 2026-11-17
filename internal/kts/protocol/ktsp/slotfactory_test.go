// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// mockConfig is a test config struct.
type mockConfig struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

// mockFactory is a test factory that creates slots with a configurable type.
type mockFactory struct {
	typeName string
}

func (f *mockFactory) TypeName() string { return f.typeName }

func (f *mockFactory) NewConfig() any {
	return &mockConfig{}
}

func (f *mockFactory) CreateSlot(name string, config any) (*Slot, error) {
	_ = config.(*mockConfig) // type assert to verify typed config works
	return NewSlot(name, DefaultInstance, "test slot", nil), nil
}

func TestSlotTypeRegistry_RegisterAndGet(t *testing.T) {
	r := NewSlotTypeRegistry()
	factory := &mockFactory{typeName: "agent"}

	if err := r.Register(factory); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := r.Get("agent")
	if !ok {
		t.Fatal("Get: factory not found")
	}
	if got.TypeName() != "agent" {
		t.Errorf("TypeName = %q, want %q", got.TypeName(), "agent")
	}
}

func TestSlotTypeRegistry_GetUnknown(t *testing.T) {
	r := NewSlotTypeRegistry()

	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get: expected false for unknown type")
	}
}

func TestSlotTypeRegistry_DuplicateRegister(t *testing.T) {
	r := NewSlotTypeRegistry()
	factory := &mockFactory{typeName: "agent"}

	if err := r.Register(factory); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	err := r.Register(factory)
	if err == nil {
		t.Fatal("second Register: expected error for duplicate")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("error = %q, want containing %q", err.Error(), "already registered")
	}
}

func TestSlotTypeRegistry_CreateSlot(t *testing.T) {
	r := NewSlotTypeRegistry()
	r.Register(&mockFactory{typeName: "agent"})

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(`name: my-agent
type: agent
capability: fast
`), &node); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}

	slot, err := r.CreateSlot("agent", "my-agent", *node.Content[0])
	if err != nil {
		t.Fatalf("CreateSlot: %v", err)
	}
	if slot.TypeName() != "my-agent" {
		t.Errorf("slot.TypeName = %q, want %q", slot.TypeName(), "my-agent")
	}
}

func TestSlotTypeRegistry_CreateSlotUnknownType(t *testing.T) {
	r := NewSlotTypeRegistry()

	var node yaml.Node
	yaml.Unmarshal([]byte(`name: x
type: unknown
`), &node)

	_, err := r.CreateSlot("unknown", "x", *node.Content[0])
	if err == nil {
		t.Fatal("CreateSlot: expected error for unknown type")
	}
	if !strings.Contains(err.Error(), "unknown slot type") {
		t.Errorf("error = %q, want containing %q", err.Error(), "unknown slot type")
	}
}

func TestSlotTypeRegistry_MultipleTypes(t *testing.T) {
	r := NewSlotTypeRegistry()
	r.Register(&mockFactory{typeName: "agent"})
	r.Register(&mockFactory{typeName: "skill"})
	r.Register(&mockFactory{typeName: "user_prompter"})

	for _, typeName := range []string{"agent", "skill", "user_prompter"} {
		_, ok := r.Get(typeName)
		if !ok {
			t.Errorf("Get(%q): not found", typeName)
		}
	}
}
