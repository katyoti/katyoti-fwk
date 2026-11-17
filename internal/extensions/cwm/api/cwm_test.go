// SPDX-License-Identifier: Apache-2.0

package api

import (
	"reflect"
	"strings"
	"testing"

	cwmreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
)

// TestCWMServiceMethodCount asserts the interface's exact method count,
// so a verb added or removed is a deliberate, counted change.
func TestCWMServiceMethodCount(t *testing.T) {
	// The object verbs, edges, project init and the
	// event stream, the comments, the declaration findings, the outbox, the change set, the module subscribers, the event raise and subscriptions, the catch-up release: the node / item / file / refactor verbs are gone (the cut).
	const want = 56
	tCWM := reflect.TypeOf((*CWMService)(nil)).Elem()
	got := tCWM.NumMethod()
	if got != want {
		t.Errorf("CWMService method count: got %d, want %d", got, want)
	}
}

// TestCWMServiceDefMethodCount asserts CWMServiceDef.Methods has the
// same count as the interface.
func TestCWMServiceDefMethodCount(t *testing.T) {
	const want = 56
	got := len(CWMServiceDef.Methods)
	if got != want {
		t.Errorf("CWMServiceDef.Methods count: got %d, want %d", got, want)
	}
}

// TestEveryCWMMethodHasItsDefinition asserts every CWMService method has
// its definition, keyed by its Go name (the only key the KTSP publisher
// reads), and every definition names a method: a definition no method
// matches silently drops its description, read-only flag, timeout and
// Hidden flag.
func TestEveryCWMMethodHasItsDefinition(t *testing.T) {
	tCWM := reflect.TypeOf((*CWMService)(nil)).Elem()
	names := map[string]bool{}
	for i := 0; i < tCWM.NumMethod(); i++ {
		name := tCWM.Method(i).Name
		names[name] = true
		if _, ok := CWMServiceDef.Methods[name]; !ok {
			t.Errorf("CWMService.%s has no definition in CWMServiceDef.Methods", name)
		}
	}
	for key := range CWMServiceDef.Methods {
		if !names[key] {
			t.Errorf("CWMServiceDef.Methods[%q] names no CWMService method", key)
		}
	}
}

// TestEventsSubscribeIsHidden asserts EventsSubscribe is flagged
// Hidden so auto-CLI generation skips its non-standard channel-
// returning shape.
func TestEventsSubscribeIsHidden(t *testing.T) {
	md, ok := CWMServiceDef.Methods["EventsSubscribe"]
	if !ok {
		t.Fatal("CWMServiceDef.Methods missing EventsSubscribe")
	}
	if !md.Hidden {
		t.Error("EventsSubscribe must be Hidden=true (channel-returning shape; not auto-CLI compatible)")
	}
}

// TestRenderTemplatesReferencedAreNonEmpty asserts that every method
// declaring a non-empty RenderTemplate references non-trivial content.
func TestRenderTemplatesReferencedAreNonEmpty(t *testing.T) {
	for key, md := range CWMServiceDef.Methods {
		if md.RenderTemplate == "" {
			continue
		}
		if strings.TrimSpace(md.RenderTemplate) == "" {
			t.Errorf("%s: RenderTemplate is whitespace-only", key)
		}
	}
}

// TestBuiltinEdgeKinds asserts the 4 generic CWM edge-kind constants.
func TestBuiltinEdgeKinds(t *testing.T) {
	wanted := map[EdgeType]string{
		EdgeTypeParentID:    "parent_id",
		EdgeTypeDerivedFrom: "derived-from",
		EdgeTypeSupersedes:  "supersedes",
	}
	for k, v := range wanted {
		if string(k) != v {
			t.Errorf("EdgeType %v: got string %q, want %q", k, string(k), v)
		}
	}
}

// TestSentinelErrorsDistinct asserts the sentinel errors all
// produce distinct messages.
func TestSentinelErrorsDistinct(t *testing.T) {
	errs := []error{
		ErrNotFound, ErrBudgetExceeded,
		ErrLockHeldByOtherProcess, ErrConflict,
		ErrCircularReference, ErrTypeRuleViolation, ErrInvalidInput,
	}
	seen := make(map[string]bool, len(errs))
	for _, err := range errs {
		msg := err.Error()
		if seen[msg] {
			t.Errorf("duplicate sentinel error message: %q", msg)
		}
		seen[msg] = true
	}
	if len(seen) != len(errs) {
		t.Errorf("sentinel error count: got %d distinct, want %d", len(seen), len(errs))
	}
}

// TestInterfaceMethodsHaveTypedIO asserts every method on CWMService
// (except EventsSubscribe) follows the typed Input → typed Output
// convention with exactly 2 args (ctx + Input) and 2 returns
// (Output, error).
func TestInterfaceMethodsHaveTypedIO(t *testing.T) {
	tCWM := reflect.TypeOf((*CWMService)(nil)).Elem()
	for i := 0; i < tCWM.NumMethod(); i++ {
		m := tCWM.Method(i)
		if m.Name == "EventsSubscribe" {
			continue
		}
		mt := m.Type
		if mt.NumIn() != 2 {
			t.Errorf("%s: NumIn=%d, want 2 (ctx + Input)", m.Name, mt.NumIn())
		}
		if mt.NumOut() != 2 {
			t.Errorf("%s: NumOut=%d, want 2 (Output, error)", m.Name, mt.NumOut())
		}
	}
}

// TestJSONSchemaForEveryIO is the load-bearing smoke test (DoD §
// "Smoke test passes: reflect.JSONSchema[T]() succeeds for every
// non-Subscribe I/O type"). Iterates every method on CWMService,
// extracts Input + Output types via reflection, and asserts
// JSONSchemaForType produces non-empty bytes.
func TestJSONSchemaForEveryIO(t *testing.T) {
	tCWM := reflect.TypeOf((*CWMService)(nil)).Elem()
	checked := 0
	for i := 0; i < tCWM.NumMethod(); i++ {
		m := tCWM.Method(i)
		if m.Name == "EventsSubscribe" {
			continue
		}
		mt := m.Type
		if mt.NumIn() < 2 || mt.NumOut() < 2 {
			continue
		}
		inputT := mt.In(1)
		outputT := mt.Out(0)

		if schema, err := cwmreflect.JSONSchemaForType(inputT); err != nil {
			t.Errorf("%s input (%s): JSONSchemaForType error: %v", m.Name, inputT.Name(), err)
		} else if len(schema) == 0 {
			t.Errorf("%s input (%s): JSONSchemaForType returned empty bytes", m.Name, inputT.Name())
		}
		if schema, err := cwmreflect.JSONSchemaForType(outputT); err != nil {
			t.Errorf("%s output (%s): JSONSchemaForType error: %v", m.Name, outputT.Name(), err)
		} else if len(schema) == 0 {
			t.Errorf("%s output (%s): JSONSchemaForType returned empty bytes", m.Name, outputT.Name())
		}
		checked++
	}
	// Every method but EventsSubscribe has typed I/O.
	if checked != 55 {
		t.Errorf("checked %d methods, expected 55 (56 minus EventsSubscribe)", checked)
	}
}

// TestRegistrationTypesSchema is a representative shape test for the
// kindreg types — confirms JSONSchema generation works on them too.
func TestRegistrationTypesSchema(t *testing.T) {
	if _, err := cwmreflect.JSONSchema[EdgeTypeRegistration](); err != nil {
		t.Errorf("EdgeTypeRegistration JSONSchema failed: %v", err)
	}
}
