// SPDX-License-Identifier: Apache-2.0

package tsgen

import (
	"strings"
	"testing"
)

const descriptor = `{
  "name": "ktw-univers",
  "ktsp": {
    "name": "ktw-univers",
    "slot_types": [
      {
        "type": "marketplace",
        "methods": [
          {
            "name": "apply-config-set",
            "description": "Apply a config-set to a universe",
            "inputSchema": {
              "type": "object",
              "required": ["name"],
              "properties": {
                "name":         {"type": "string", "description": "Config-set to apply"},
                "force":        {"type": "boolean"},
                "values":       {"type": "object", "additionalProperties": {"type": "string"}},
                "universeHome": {"type": "string"}
              }
            },
            "outputSchema": {
              "type": "object",
              "properties": {
                "missing": {"type": "array", "items": {"type": "string"}},
                "keys": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "required": ["name"],
                    "properties": {
                      "name":    {"type": "string"},
                      "present": {"type": "boolean"},
                      "count":   {"type": "number"}
                    }
                  }
                }
              }
            }
          },
          {
            "name": "list-config-sets",
            "description": "List config-sets",
            "outputSchema": {
              "type": "array",
              "items": {"type": "object", "properties": {"name": {"type": "string"}}}
            }
          }
        ]
      }
    ]
  }
}`

func gen(t *testing.T) string {
	t.Helper()
	out, err := Generate([]byte(descriptor), Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	src, ok := out["marketplace"]
	if !ok {
		t.Fatalf("no marketplace module generated (got %v)", keys(out))
	}
	return src
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func mustContain(t *testing.T, src string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(src, w) {
			t.Errorf("generated client missing %q:\n%s", w, src)
		}
	}
}

// The slot path must be built from the WIRE name (kebab), not from a prettified
// identifier — getting this wrong produces a client that compiles and 404s.
func TestGenerate_SlotPathUsesWireName(t *testing.T) {
	mustContain(t, gen(t),
		`dispatch("ktw-univers:marketplace/apply-config-set"`,
		`dispatch("ktw-univers:marketplace/list-config-sets"`,
	)
}

func TestGenerate_TypesAndOptionality(t *testing.T) {
	src := gen(t)
	mustContain(t, src,
		"export interface ApplyConfigSetInput {",
		"name: string",                    // required -> no `?`
		"force?: boolean",                 // not required -> optional
		"values?: Record<string, string>", // additionalProperties -> a map
		"missing?: string[]",              // array of scalars
	)
	if strings.Contains(src, "name?: string\n  force") {
		t.Errorf("a required property must not be optional:\n%s", src)
	}
}

// A nested object schema is INLINED by the descriptors, so the generator must
// inline the TS too rather than emit a dangling named type.
func TestGenerate_InlinesNestedObjects(t *testing.T) {
	src := gen(t)
	mustContain(t, src, "keys?: {")
	if strings.Contains(src, "undefined") && strings.Contains(src, "keys?: undefined") {
		t.Errorf("nested object collapsed:\n%s", src)
	}
}

// A method with no input must take no argument — otherwise every call site has to
// pass a meaningless `{}`.
func TestGenerate_NoInputMethodTakesNoArgument(t *testing.T) {
	src := gen(t)
	mustContain(t, src,
		"export type ListConfigSetsInput = void",
		"listConfigSets: (): Promise<ListConfigSetsOutput> =>",
		`dispatch("ktw-univers:marketplace/list-config-sets", undefined)`,
	)
}

func TestGenerate_ClientFactoryAndDoc(t *testing.T) {
	src := gen(t)
	mustContain(t, src,
		"export function makeMarketplaceClient(dispatch: KtspDispatch) {",
		"applyConfigSet: (input: ApplyConfigSetInput): Promise<ApplyConfigSetOutput> =>",
		"/** Apply a config-set to a universe */",
		"DO NOT EDIT",
	)
}

func TestGenerate_SlotTypeFilter(t *testing.T) {
	out, err := Generate([]byte(descriptor), Options{SlotTypes: []string{"nope"}})
	if err == nil {
		t.Fatalf("filtering to an absent slot type should error, got %v", keys(out))
	}
	out, err = Generate([]byte(descriptor), Options{SlotTypes: []string{"marketplace"}})
	if err != nil || len(out) != 1 {
		t.Fatalf("filter should yield exactly the marketplace module: %v %v", keys(out), err)
	}
}

// Constructs the generator does not model must FAIL LOUDLY. Emitting `any` would
// put a wrong type at a call site, which is worse than not generating at all.
func TestGenerate_RefusesUnmodelledConstructs(t *testing.T) {
	for label, schema := range map[string]string{
		"$ref":  `{"type":"object","properties":{"x":{"$ref":"#/$defs/Y"}}}`,
		"oneOf": `{"type":"object","properties":{"x":{"oneOf":[{"type":"string"}]}}}`,
		"weird": `{"type":"object","properties":{"x":{"type":"tuple"}}}`,
	} {
		d := `{"name":"p","ktsp":{"name":"p","slot_types":[{"type":"s","methods":[
			{"name":"m","inputSchema":` + schema + `}]}]}}`
		if _, err := Generate([]byte(d), Options{}); err == nil {
			t.Errorf("%s must be refused, not silently emitted", label)
		}
	}
}

func TestGenerate_ProviderFallbackAndErrors(t *testing.T) {
	// ktsp.name wins; falling back to the binary name keeps a minimal descriptor working.
	d := `{"name":"fallback","ktsp":{"slot_types":[{"type":"s","methods":[{"name":"m"}]}]}}`
	out, err := Generate([]byte(d), Options{})
	if err != nil {
		t.Fatalf("provider should fall back to the binary name: %v", err)
	}
	mustContain(t, out["s"], `dispatch("fallback:s/m"`)

	if _, err := Generate([]byte(`{"ktsp":{"slot_types":[]}}`), Options{}); err == nil {
		t.Error("a descriptor with no provider name must error")
	}
	if _, err := Generate([]byte("not json"), Options{}); err == nil {
		t.Error("invalid JSON must error")
	}
}
