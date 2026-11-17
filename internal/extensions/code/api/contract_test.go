// SPDX-License-Identifier: Apache-2.0

package api

import (
	"reflect"
	"strings"
	"testing"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
)

// TestCodeServiceMethodsDiscoverable pins the publishable convention
// (ctx, Input) → (Output, error) over the grown CodeService —
// AutoKTSPContainer derives the KTSP / agent-tool / CLI / HTTP
// surfaces from it; an off-shape method goes silently missing.
func TestCodeServiceMethodsDiscoverable(t *testing.T) {
	t.Parallel()
	iface := reflect.TypeOf((*CodeService)(nil)).Elem()
	methods := corereflect.DiscoverMethods(iface)

	want := 8 // 3 analysis + discover + 2 decision verbs + 2 projections
	if len(methods) != want {
		t.Fatalf("CodeService discovered methods = %d, want %d", len(methods), want)
	}
	for _, m := range methods {
		if m.InputType == nil || m.OutputType == nil {
			t.Errorf("%s: nil Input/Output type", m.Name)
			continue
		}
		if !strings.HasSuffix(m.OutputType.Name(), "Output") {
			t.Errorf("%s: output type %s does not follow the <X>Output convention", m.Name, m.OutputType.Name())
		}
	}
}

// TestLanguagePackMethodsDiscoverable does the same for the
// LanguagePack contract (D-1: every method serializable — packs may
// run in-process or behind a KTSP slot with the same shape).
func TestLanguagePackMethodsDiscoverable(t *testing.T) {
	t.Parallel()
	iface := reflect.TypeOf((*LanguagePack)(nil)).Elem()
	methods := corereflect.DiscoverMethods(iface)

	if want := 6; len(methods) != want {
		t.Fatalf("LanguagePack discovered methods = %d, want %d", len(methods), want)
	}
}

// TestContractTypesSchemaable generates a JSON schema for every
// contract I/O type — the D-2 gate: no funcs, no Go interfaces, no
// unschemaable fields anywhere in the contract.
func TestContractTypesSchemaable(t *testing.T) {
	t.Parallel()
	for _, iface := range []reflect.Type{
		reflect.TypeOf((*CodeService)(nil)).Elem(),
		reflect.TypeOf((*LanguagePack)(nil)).Elem(),
	} {
		for _, m := range corereflect.DiscoverMethods(iface) {
			for _, typ := range []reflect.Type{m.InputType, m.OutputType} {
				if typ == nil {
					continue
				}
				if _, err := corereflect.JSONSchemaForType(typ); err != nil {
					t.Errorf("%s: %s is not JSON-schema-able: %v", m.Name, typ.Name(), err)
				}
			}
		}
	}
}
