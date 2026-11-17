// SPDX-License-Identifier: Apache-2.0

// Package schemalang asserts that every schema a binary can hand to a consumer
// is written in the one language the universe speaks.
//
// # The thing this catches
//
// A schema has two audiences with different tastes: the registry, the form
// renderers and the census read the ktw-schema DSL; a model reads JSON Schema.
// For a while the codebase served both by ACCEPTING both — `output-schema:` was
// authored in either language and a sniff downstream decided which. That works
// until a document is handed to the audience that cannot read it, and then
// nothing reports it: an LLM given a DSL document as a tool's `parameters`
// simply produces the wrong call, and a data-type whose body is JSON Schema
// registers nowhere and 404s in a consumer that cannot say why.
//
// So: one authored language, derived on the way out. This package checks the
// half the census cannot see — the schemas a STRATEGY provides in Go, which are
// never published as contributions but override and are overridden by the ones
// that are.
package schemalang

import (
	"context"
	"testing"

	schemaparser "github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// AssertStrategyDefaults checks that every registered machine strategy's
// default input/output schema parses as the DSL and compiles to JSON Schema.
//
// Discovered from the domain rather than listed here, so a strategy added
// tomorrow is covered without anyone remembering this file. It must run against
// an application that has already been built — that is when strategies reach
// the domain — which is why it takes no extensions of its own and is called
// from a test that has one.
func AssertStrategyDefaults(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()

	entries := engine.MachineDomain.Types.Entries()
	if len(entries) == 0 {
		t.Fatalf("schemalang[%s]: no machine strategies registered — "+
			"this check ran before the application was built, and would "+
			"have passed by checking nothing", name)
	}

	checked := 0
	for _, td := range entries {
		sp, ok := td.Impl.(engine.SchemaProvider)
		if !ok {
			continue
		}
		for _, s := range []struct {
			role string
			body []byte
		}{
			{"input", sp.DefaultInputSchema()},
			{"output", sp.DefaultOutputSchema()},
		} {
			// No default is a normal answer: most strategies take whatever
			// the task declares.
			if len(s.body) == 0 {
				continue
			}
			checked++
			if _, err := schemaparser.CompileJSONSchema(ctx, s.body); err != nil {
				t.Errorf("schemalang[%s]: strategy %q default %s schema is not the DSL: %v\n  %s\n\n"+
					"Strategy defaults and TASK.md `%s-schema:` blocks override each other, so they "+
					"must be one language. Write the DSL here; JSON Schema is compiled at the edge "+
					"that speaks it (schemaparser.CompileJSONSchema).",
					name, td.Name, s.role, err, s.body, s.role)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("schemalang[%s]: %d strategies registered but none declared a default schema — "+
			"a check that examined nothing reads exactly like a check that passed", name, len(entries))
	}
	t.Logf("schemalang[%s]: %d strategy default schema(s) compile", name, checked)
}
