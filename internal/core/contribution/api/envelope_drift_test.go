// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
)

// The shipped envelope data-type and the Go envelope must agree, recursively.
//
// G3 says the meta-model is self-describing: a reader in any language resolves
// `system:/data-type/contribution` and learns what a contribution looks like.
// That was false for a while — the file described a 9-field envelope the code
// had retired (schema, schemaV, body, edges) — and nothing noticed, because
// nothing compared them. This does, at every nesting level: envelope, meta,
// icon, layer descriptor, provenance.
//
// There is no DSL emitter, so the comparison is between ASTs: the Go side
// through reflect.TypeAST, the shipped side through the parser. Named types in
// the DSL are resolved; inline objects on the Go side are walked in place.
func TestEnvelopeDataTypeMatchesGoEnvelope(t *testing.T) {
	path := filepath.Join("..", "embed", "data-type", "contribution.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("shipped envelope data-type not reachable: %v", err)
	}
	out, err := parser.ParseFile(context.Background(), schemaapi.ParseFileInput{Path: path})
	if err != nil {
		t.Fatalf("shipped envelope data-type does not parse: %v", err)
	}
	root, ok := out.Schema.Types[out.Schema.Root]
	if !ok {
		t.Fatalf("root type %q missing", out.Schema.Root)
	}
	goRoot, err := reflect.TypeAST[Contribution]()
	if err != nil {
		t.Fatalf("TypeAST[Contribution]: %v", err)
	}
	compareObjects(t, out.Schema, "Contribution", goRoot, root)
}

// compareObjects checks that two object types declare the same field names,
// then descends into every field that is an object on the Go side.
func compareObjects(t *testing.T, s *schemaapi.Schema, at string, goT, dslT *schemaapi.Type) {
	t.Helper()
	if goT.Kind != schemaapi.KindObject || dslT.Kind != schemaapi.KindObject {
		if goT.Kind != dslT.Kind {
			t.Errorf("%s: Go kind %q, declared kind %q", at, goT.Kind, dslT.Kind)
		}
		return
	}
	// A map on the Go side is an object with a catchall and no fields; the DSL
	// side must say the same, and the value types are compared below.
	if goT.Catchall != nil || dslT.Catchall != nil {
		if goT.Catchall == nil || dslT.Catchall == nil {
			t.Errorf("%s: catchall declared on one side only (Go=%v, declared=%v)", at, goT.Catchall != nil, dslT.Catchall != nil)
			return
		}
		compareObjects(t, s, at+"{}", deref(s, goT.Catchall), deref(s, dslT.Catchall))
		return
	}

	var missing, extra []string
	for name := range goT.Fields {
		if _, ok := dslT.Fields[name]; !ok {
			missing = append(missing, name)
		}
	}
	for name := range dslT.Fields {
		if _, ok := goT.Fields[name]; !ok {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("%s: Go fields with NO declaration (a polyglot reader cannot see them): %v", at, missing)
	}
	if len(extra) > 0 {
		t.Errorf("%s: declared fields the envelope no longer carries (a promise nothing keeps): %v", at, extra)
	}
	for name, gf := range goT.Fields {
		df, ok := dslT.Fields[name]
		if !ok {
			continue
		}
		gv, dv := gf, resolveField(s, df)
		if gv.Kind == schemaapi.KindArray && dv.Kind == schemaapi.KindArray && gv.Items != nil && dv.Items != nil {
			gv, dv = deref(s, gv.Items), deref(s, dv.Items)
		}
		if gv.Kind == schemaapi.KindObject {
			compareObjects(t, s, at+"."+name, gv, dv)
		}
	}
}

// resolveField follows a field declared as a bare type name (`type: LayerRef`)
// to its definition: the parser keeps the user type's name as the field's kind.
func resolveField(s *schemaapi.Schema, f *schemaapi.Type) *schemaapi.Type {
	if typ, ok := s.Types[string(f.Kind)]; ok {
		return typ
	}
	return f
}

func deref(s *schemaapi.Schema, r *schemaapi.TypeRef) *schemaapi.Type {
	if r.Inline != nil {
		return resolveField(s, r.Inline)
	}
	if typ, ok := s.Types[r.Name]; ok {
		return typ
	}
	return &schemaapi.Type{Kind: schemaapi.KindAny}
}
