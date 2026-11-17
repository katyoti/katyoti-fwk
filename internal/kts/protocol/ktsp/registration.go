// SPDX-License-Identifier: Apache-2.0

package ktsp

// SlotRegistration bundles everything needed to register a slot type
// with the EM. It is the unit of contribution from an extension to a
// slot-type domain.
//
// T is the slot's Go interface type (e.g., DocConverterSlot).
// Factory creates a config object that can be unmarshaled from YAML and
// then called to produce an implementation of T.
// SkeletonBuilder and StubBuilder are optional — nil means "not available"
// (e.g., code generation hasn't been run yet, or the slot type is local-only).
type SlotRegistration[T any] struct {
	// Name identifies this slot type (e.g., "doc-converter").
	Name string

	// SlotType is the KTSP slot type constant (e.g., SlotTypeSkill).
	SlotType SlotType

	// Factory creates a new config object that can be unmarshaled from YAML
	// and produces an implementation of T via Create(). Required.
	Factory func() SlotCreator[T]

	// SkeletonBuilder wraps a T implementation into a KTSP Slot for
	// server-side dispatch. Optional — nil if no skeleton is available.
	SkeletonBuilder func(name string, impl T) *Slot

	// StubBuilder creates a typed client proxy from a GenericStub.
	// Optional — nil if no stub is available.
	StubBuilder func(stub *GenericStub) T
}

// SlotCreator produces an instance of T from configuration.
// Implementations are YAML-unmarshalable config structs whose Create
// method builds the slot implementation.
//
// This mirrors strategies.TypedConfig but is defined here to avoid a
// circular dependency between ktsp and kits/strategies.
type SlotCreator[T any] interface {
	Create() (T, error)
}
