// SPDX-License-Identifier: Apache-2.0

package api

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A capability's Go struct and its DSL must declare the SAME FIELDS.
//
// WHY THIS DID NOT EXIST AND SHOULD HAVE. The project has a census (which
// contributions ship), (that a capability NAME is declared) and probes
// for object shape — but nothing compared a capability struct's fields against
// the DSL that describes it. Adding a field to one and forgetting the other
// was silent: the struct would carry a value the schema never mentions, or the
// schema would promise a field nothing can hold. "The drift guard passes" was
// a statement about the DSL parsing, not about the fields agreeing.
func TestCapabilityStructsMatchTheirDSL(t *testing.T) {
	root := repoRootFromAPI(t)
	core := filepath.Join(root, "internal", "extensions", "cwm", "contrib", "embed", "cwm", "data-types")

	cases := []struct {
		dir, dsl string
		typ      any
	}{
		{core, "object-core.dsl.yaml", Identity{}},
	}

	for _, tc := range cases {
		t.Run(tc.dsl, func(t *testing.T) {
			declared, err := dslFieldNames(filepath.Join(tc.dir, tc.dsl))
			if err != nil {
				t.Fatalf("read %s: %v", tc.dsl, err)
			}
			// A FLOOR: with the parse wrong this would compare two empty sets
			// and pass, which is the vacuous green this project keeps finding.
			if len(declared) == 0 {
				t.Fatalf("%s: no fields parsed — nothing was compared", tc.dsl)
			}

			inGo := jsonFieldNames(tc.typ)
			if len(inGo) == 0 {
				t.Fatalf("%T: no json-tagged fields found — nothing was compared", tc.typ)
			}

			for _, f := range inGo {
				if !hasField(declared, f) {
					t.Errorf("%T has field %q that %s does not declare — a value the schema never mentions",
						tc.typ, f, tc.dsl)
				}
			}
			for _, f := range declared {
				if !hasField(inGo, f) {
					t.Errorf("%s declares %q but %T has no such field — a promise nothing can hold",
						tc.dsl, f, tc.typ)
				}
			}
		})
	}
}

// dslFieldNames reads the field keys under the DSL's single `fields:` block.
//
// Deliberately shallow: it reads the TOP-LEVEL fields of the root type, which
// is what a capability field-group is. A nested-type DSL would need a real
// parser, and pretending to handle one here would make the guard lie about
// what it checked.
var dslFieldRe = regexp.MustCompile(`^      ([a-z_][a-z0-9_]*):\s*$`)

func dslFieldNames(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	inFields := false
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "fields:" {
			inFields = true
			continue
		}
		if !inFields {
			continue
		}
		// A line at or above the `types:` indent ends the block.
		if line != "" && !strings.HasPrefix(line, "      ") {
			break
		}
		if m := dslFieldRe.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
		}
	}
	sort.Strings(out)
	return out, nil
}

// jsonFieldNames returns the json tag names of a struct's exported fields.
func jsonFieldNames(v any) []string {
	rt := reflect.TypeOf(v)
	var out []string
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		out = append(out, strings.Split(tag, ",")[0])
	}
	sort.Strings(out)
	return out
}

func hasField(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func repoRootFromAPI(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("go.mod not found")
	return ""
}
