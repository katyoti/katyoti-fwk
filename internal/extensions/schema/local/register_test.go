// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	schemalocal "github.com/katyoti/katyoti-fwk/internal/extensions/schema/local"
)

// register_test.go — a schema that quietly fails to register becomes a URI that
// 404s later, in a consumer that cannot explain why. These assert the failure
// is never quiet.

type recordingSvc struct {
	schemaapi.SchemaService
	registered map[string]string
	fail       map[string]bool
}

func (s *recordingSvc) RegisterDataType(_ context.Context, in schemaapi.RegisterDataTypeInput) error {
	if s.fail[in.URI] {
		return errUnparseable
	}
	if s.registered == nil {
		s.registered = map[string]string{}
	}
	s.registered[in.URI] = string(in.Body)
	return nil
}

var errUnparseable = &parseErr{}

type parseErr struct{}

func (*parseErr) Error() string { return "link target \"Missing\" is not declared" }

func dataType(provider, id, dsl string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: provider, Type: contribapi.ResourceTypeDataType, ID: id}
	c.SetSource([]byte(dsl), "application/yaml")
	return c
}

// The DSL comes from the source layer, which is where it now lives — and only
// there.
func TestRegistersFromTheSourceLayer(t *testing.T) {
	svc := &recordingSvc{}
	cs := []contribapi.Contribution{
		dataType("ktw-docs", "skill", "name: skill\n"),
		dataType("ktw-docs", "task", "name: task\n"),
		{Provider: "ktw-docs", Type: "skill", ID: "not-a-data-type"}, // ignored
	}
	n, failed := schemalocal.RegisterDataTypes(context.Background(), svc, cs, nil)
	if n != 2 || len(failed) != 0 {
		t.Fatalf("registered %d with %d failures, want 2 and 0", n, len(failed))
	}
	if got := svc.registered["ktw-docs:/data-type/skill"]; got != "name: skill\n" {
		t.Errorf("registered %q — the DSL did not arrive as written", got)
	}
}

// A data-type whose content the index does not carry is REPORTED, not skipped.
// Silently skipping is how 480 method-I/O data-types once vanished.
func TestUnreachableSchemaIsReportedNotSkipped(t *testing.T) {
	svc := &recordingSvc{}
	c := dataType("ktw-docs", "big", "name: big\n")
	// A declared layer whose content is not carried — the index plane.
	c.DropInline()
	ref := c.Layers[contribapi.LayerSource]
	ref.Size = 1 << 20 // past the inline band, so DropInline drops it
	ref.Inline = nil
	c.Layers[contribapi.LayerSource] = ref

	n, failed := schemalocal.RegisterDataTypes(context.Background(), svc, []contribapi.Contribution{c}, nil)
	if n != 0 {
		t.Fatalf("registered %d, want 0", n)
	}
	if len(failed) != 1 {
		t.Fatalf("reported %d failures, want 1 — an unreachable schema must not be silent", len(failed))
	}
	if failed[0].URI != "ktw-docs:/data-type/big" {
		t.Errorf("failure names %q", failed[0].URI)
	}

	// With a resolver it registers, which is what the resolver is for.
	svc2 := &recordingSvc{}
	n, failed = schemalocal.RegisterDataTypes(context.Background(), svc2,
		[]contribapi.Contribution{c},
		func(string) ([]byte, error) { return []byte("name: big\n"), nil })
	if n != 1 || len(failed) != 0 {
		t.Fatalf("with a resolver: registered %d, %d failures", n, len(failed))
	}
}

// A data-type that declares NO content is not a failure — it has nothing to
// register. Distinct from one that declares content nobody can reach.
func TestADataTypeWithNoContentIsNotAFailure(t *testing.T) {
	svc := &recordingSvc{}
	c := contribapi.Contribution{Provider: "p", Type: contribapi.ResourceTypeDataType, ID: "empty"}
	n, failed := schemalocal.RegisterDataTypes(context.Background(), svc, []contribapi.Contribution{c}, nil)
	if n != 0 || len(failed) != 0 {
		t.Errorf("registered %d with %d failures, want 0 and 0", n, len(failed))
	}
}

// A DSL the parser rejects is reported with the parser's own words — the only
// account of the failure that will exist.
func TestAnUnparseableSchemaIsReportedWithItsReason(t *testing.T) {
	svc := &recordingSvc{fail: map[string]bool{"p:/data-type/broken": true}}
	n, failed := schemalocal.RegisterDataTypes(context.Background(), svc,
		[]contribapi.Contribution{dataType("p", "broken", "name: broken\n")}, nil)
	if n != 0 || len(failed) != 1 {
		t.Fatalf("registered %d with %d failures", n, len(failed))
	}
	if failed[0].Reason == "" {
		t.Error("failure carries no reason")
	}
}
