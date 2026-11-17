// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"testing"
)

// testSlot is a minimal slot interface for testing SlotRegistration.
type testSlot interface {
	Echo(msg string) string
}

// testSlotImpl implements testSlot.
type testSlotImpl struct{ prefix string }

func (s *testSlotImpl) Echo(msg string) string { return s.prefix + msg }

// testSlotConfig implements SlotCreator[testSlot].
type testSlotConfig struct {
	Prefix string `yaml:"prefix"`
}

func (c *testSlotConfig) Create() (testSlot, error) {
	return &testSlotImpl{prefix: c.Prefix}, nil
}

func TestSlotRegistration_FullyPopulated(t *testing.T) {
	reg := SlotRegistration[testSlot]{
		Name:     "test-slot",
		SlotType: SlotTypeSkill,
		Factory: func() SlotCreator[testSlot] {
			return &testSlotConfig{Prefix: ">>"}
		},
		SkeletonBuilder: func(name string, impl testSlot) *Slot {
			return NewSlot(name, DefaultInstance, "test", nil)
		},
		StubBuilder: func(stub *GenericStub) testSlot {
			return &testSlotImpl{prefix: "stub:"}
		},
	}

	if reg.Name != "test-slot" {
		t.Errorf("got name %q", reg.Name)
	}
	if reg.SlotType != SlotTypeSkill {
		t.Errorf("got slot type %q", reg.SlotType)
	}

	// Factory produces a creator that builds an implementation
	creator := reg.Factory()
	impl, err := creator.Create()
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := impl.Echo("hello"); got != ">>hello" {
		t.Errorf("got %q, want %q", got, ">>hello")
	}

	// SkeletonBuilder produces a Slot
	slot := reg.SkeletonBuilder("myslot", impl)
	if slot.TypeName() != "myslot" {
		t.Errorf("skeleton slot type %q", slot.TypeName())
	}

	// StubBuilder produces a typed proxy
	stub := reg.StubBuilder(nil)
	if got := stub.Echo("x"); got != "stub:x" {
		t.Errorf("stub got %q, want %q", got, "stub:x")
	}
}

func TestSlotRegistration_OptionalFieldsNil(t *testing.T) {
	reg := SlotRegistration[testSlot]{
		Name:     "minimal",
		SlotType: SlotTypeSkill,
		Factory: func() SlotCreator[testSlot] {
			return &testSlotConfig{}
		},
		// SkeletonBuilder and StubBuilder intentionally nil
	}

	if reg.SkeletonBuilder != nil {
		t.Error("SkeletonBuilder should be nil")
	}
	if reg.StubBuilder != nil {
		t.Error("StubBuilder should be nil")
	}

	// Factory still works
	creator := reg.Factory()
	impl, err := creator.Create()
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := impl.Echo("test"); got != "test" {
		t.Errorf("got %q, want %q", got, "test")
	}
}
