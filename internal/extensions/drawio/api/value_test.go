// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"encoding/json"
	"strings"
	"testing"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// ─────────────────────────────────────────────────────────────────────
// TypeAST round-trip — top-level shape
// ─────────────────────────────────────────────────────────────────────

func TestResolver_Scalar_String(t *testing.T) {
	roundTripResolver(t, `"#FFFFFF"`, drawioapi.ResolverScalar, func(r drawioapi.Resolver) {
		if r.Scalar != "#FFFFFF" {
			t.Errorf("Scalar: want %q, got %v", "#FFFFFF", r.Scalar)
		}
	})
}

func TestResolver_Scalar_Number(t *testing.T) {
	roundTripResolver(t, `11`, drawioapi.ResolverScalar, func(r drawioapi.Resolver) {
		// JSON numbers decode to float64.
		if got, ok := r.Scalar.(float64); !ok || got != 11 {
			t.Errorf("Scalar number: want 11, got %v (%T)", r.Scalar, r.Scalar)
		}
	})
}

func TestResolver_Scalar_Bool(t *testing.T) {
	roundTripResolver(t, `true`, drawioapi.ResolverScalar, func(r drawioapi.Resolver) {
		if r.Scalar != true {
			t.Errorf("Scalar bool: want true, got %v", r.Scalar)
		}
	})
}

func TestResolver_Const(t *testing.T) {
	roundTripResolver(t, `{"const":"fontSizes.title"}`, drawioapi.ResolverConst, func(r drawioapi.Resolver) {
		if r.Const != "fontSizes.title" {
			t.Errorf("Const: want %q, got %q", "fontSizes.title", r.Const)
		}
	})
}

func TestResolver_Lookup_WithField(t *testing.T) {
	in := `{"lookup":"attributes.domain","table":"domainPalette","field":"fill","default":"#FFFFFF"}`
	roundTripResolver(t, in, drawioapi.ResolverLookup, func(r drawioapi.Resolver) {
		if r.Lookup == nil {
			t.Fatal("Lookup should be non-nil")
		}
		if r.Lookup.Path != "attributes.domain" {
			t.Errorf("Lookup.Path: want attributes.domain, got %q", r.Lookup.Path)
		}
		if r.Lookup.Table != "domainPalette" {
			t.Errorf("Lookup.Table: want domainPalette, got %q", r.Lookup.Table)
		}
		if r.Lookup.Field != "fill" {
			t.Errorf("Lookup.Field: want fill, got %q", r.Lookup.Field)
		}
		if r.Lookup.Default != "#FFFFFF" {
			t.Errorf("Lookup.Default: want #FFFFFF, got %v", r.Lookup.Default)
		}
	})
}

func TestResolver_Lookup_WithoutField(t *testing.T) {
	in := `{"lookup":"attributes.flow_type","table":"flowPalette","default":"#666666"}`
	roundTripResolver(t, in, drawioapi.ResolverLookup, func(r drawioapi.Resolver) {
		if r.Lookup.Field != "" {
			t.Errorf("Lookup.Field: want empty, got %q", r.Lookup.Field)
		}
	})
}

func TestResolver_Match(t *testing.T) {
	in := `{"match":"attributes.critical","cases":{"true":3,"false":1},"default":1}`
	roundTripResolver(t, in, drawioapi.ResolverMatch, func(r drawioapi.Resolver) {
		if r.Match == nil {
			t.Fatal("Match should be non-nil")
		}
		if r.Match.Path != "attributes.critical" {
			t.Errorf("Match.Path: want attributes.critical, got %q", r.Match.Path)
		}
		if len(r.Match.Cases) != 2 {
			t.Errorf("Match.Cases: want 2, got %d", len(r.Match.Cases))
		}
	})
}

func TestResolver_Coalesce(t *testing.T) {
	in := `{"coalesce":[{"const":"a"},{"const":"b"},"#FFFFFF"]}`
	roundTripResolver(t, in, drawioapi.ResolverCoalesce, func(r drawioapi.Resolver) {
		if len(r.Coalesce) != 3 {
			t.Errorf("Coalesce: want 3 elements, got %d", len(r.Coalesce))
		}
		if r.Coalesce[0].Kind() != drawioapi.ResolverConst {
			t.Errorf("Coalesce[0].Kind: want Const, got %s", r.Coalesce[0].Kind())
		}
		if r.Coalesce[2].Kind() != drawioapi.ResolverScalar {
			t.Errorf("Coalesce[2].Kind: want Scalar, got %s", r.Coalesce[2].Kind())
		}
	})
}

func TestResolver_Expr(t *testing.T) {
	in := `{"expr":"attributes.critical ? 3 : 1"}`
	roundTripResolver(t, in, drawioapi.ResolverExpr, func(r drawioapi.Resolver) {
		if r.Expr != "attributes.critical ? 3 : 1" {
			t.Errorf("Expr: want %q, got %q", "attributes.critical ? 3 : 1", r.Expr)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────
// Resolver — Unmarshal error cases
// ─────────────────────────────────────────────────────────────────────

func TestResolver_Unmarshal_UnknownDiscriminator(t *testing.T) {
	var r drawioapi.Resolver
	err := json.Unmarshal([]byte(`{"foo":"bar"}`), &r)
	if err == nil {
		t.Fatal("expected error for unknown discriminator")
	}
	if !strings.Contains(err.Error(), "discriminator") {
		t.Errorf("error should mention discriminator: %v", err)
	}
}

func TestResolver_Unmarshal_EmptyObject(t *testing.T) {
	var r drawioapi.Resolver
	err := json.Unmarshal([]byte(`{}`), &r)
	if err == nil {
		t.Fatal("expected error for empty object")
	}
}

func TestResolver_Marshal_ZeroFails(t *testing.T) {
	var r drawioapi.Resolver
	if _, err := json.Marshal(r); err == nil {
		t.Fatal("expected marshal error for zero Resolver")
	}
}

// ─────────────────────────────────────────────────────────────────────
// Resolver Kind / IsZero helpers
// ─────────────────────────────────────────────────────────────────────

func TestResolver_Kind_Helpers(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want drawioapi.ResolverKind
	}{
		{"scalar string", `"x"`, drawioapi.ResolverScalar},
		{"scalar number", `1`, drawioapi.ResolverScalar},
		{"scalar bool", `false`, drawioapi.ResolverScalar},
		{"const", `{"const":"x"}`, drawioapi.ResolverConst},
		{"lookup", `{"lookup":"x","table":"t","default":""}`, drawioapi.ResolverLookup},
		{"match", `{"match":"x","cases":{},"default":""}`, drawioapi.ResolverMatch},
		{"coalesce", `{"coalesce":[]}`, drawioapi.ResolverCoalesce},
		{"expr", `{"expr":"x"}`, drawioapi.ResolverExpr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var r drawioapi.Resolver
			if err := json.Unmarshal([]byte(tc.in), &r); err != nil {
				t.Fatal(err)
			}
			if got := r.Kind(); got != tc.want {
				t.Errorf("Kind: want %s, got %s", tc.want, got)
			}
			if r.IsZero() {
				t.Error("IsZero: should be false after Unmarshal")
			}
		})
	}
}

func TestResolver_Zero_IsZero(t *testing.T) {
	var r drawioapi.Resolver
	if !r.IsZero() {
		t.Error("zero Resolver should report IsZero=true")
	}
	if r.Kind() != drawioapi.ResolverInvalid {
		t.Errorf("zero Kind: want Invalid, got %s", r.Kind())
	}
}

// ─────────────────────────────────────────────────────────────────────
// Static enum constants
// ─────────────────────────────────────────────────────────────────────

func TestLayoutAlgorithm_Constants(t *testing.T) {
	if len(drawioapi.AllLayoutAlgorithms) != 4 {
		t.Errorf("AllLayoutAlgorithms: want 4 entries, got %d", len(drawioapi.AllLayoutAlgorithms))
	}
}

// ─────────────────────────────────────────────────────────────────────
// MarkerDef, DecorationSpec, LabelSpec, LabelSection — round-trip
// ─────────────────────────────────────────────────────────────────────

func roundTripResolver(t *testing.T, in string, wantKind drawioapi.ResolverKind, check func(drawioapi.Resolver)) {
	t.Helper()
	var r drawioapi.Resolver
	if err := json.Unmarshal([]byte(in), &r); err != nil {
		t.Fatalf("unmarshal %q: %v", in, err)
	}
	if r.Kind() != wantKind {
		t.Errorf("kind: want %s, got %s", wantKind, r.Kind())
	}
	check(r)

	out, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Reasonable normalisation: re-parse the output and the input as
	// generic JSON, then compare. JSON whitespace differences are fine.
	var inJSON, outJSON any
	_ = json.Unmarshal([]byte(in), &inJSON)
	_ = json.Unmarshal(out, &outJSON)
	if !jsonEqual(inJSON, outJSON) {
		t.Errorf("round-trip mismatch:\n  in:  %s\n  out: %s", in, string(out))
	}
}

func jsonEqual(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
