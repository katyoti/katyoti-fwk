// SPDX-License-Identifier: Apache-2.0

package el

import (
	"reflect"
	"testing"
)

// evalFrom answers an expression from a fixed table; a name not in it does
// not resolve (the caller's "unknown name" / "evaluation error").
func evalFrom(vals map[string]any, asked *[]string) func(string) (any, bool) {
	return func(expr string) (any, bool) {
		if asked != nil {
			*asked = append(*asked, expr)
		}
		v, ok := vals[expr]
		return v, ok
	}
}

func TestInterpolate(t *testing.T) {
	vals := map[string]any{
		"inputs.name": "Ada",
		"state.count": 3,
		"inputs.tags": []any{"a", "b"},
		"inputs.flag": true,
	}
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"non-string kept as is", 42, 42},
		{"plain text untouched", "hello world", "hello world"},
		{"whole template keeps the typed value", "${state.count}", 3},
		{"whole template is trimmed", "${  inputs.name  }", "Ada"},
		{"whole template of a list keeps the list", "${inputs.tags}", []any{"a", "b"}},
		{"whole unknown template stays the template", "${nope}", "${nope}"},
		{"one embedded template spliced as string", "hi ${inputs.name}!", "hi Ada!"},
		{"several templates spliced", "${inputs.name} has ${state.count} items", "Ada has 3 items"},
		{"two adjacent whole-looking templates splice", "${inputs.name}${state.count}", "Ada3"},
		{"non-string embedded value spliced as JSON", "tags=${inputs.tags} flag=${inputs.flag}", `tags=["a","b"] flag=true`},
		{"unknown embedded template becomes empty", "x=${nope};", "x=;"},
		{"unclosed template kept verbatim", "a ${inputs.name", "a ${inputs.name"},
		{"quotes inside the expression reach eval", `say ${"quoted"}`, "say "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Interpolate(tc.in, evalFrom(vals, nil))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Interpolate(%#v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// An expression ends at the brace that CLOSES its template: braces, brackets
// and parentheses nest and quoted strings are skipped, so a map literal or a
// quoted "}" stays inside the expression.
func TestInterpolate_ExpressionEndsAtItsClosingBrace(t *testing.T) {
	vals := map[string]any{
		`{"a": 1}.a`:          "X",
		`"}" + inputs.name`:   "}Ada",
		`'a\'}' + x`:          "Q",
		"`}`":                 "R",
		`f({k: [1, {b: 2}]})`: "N",
		`{"a": {"b": 1}}`:     map[string]any{"a": map[string]any{"b": 1}},
		`x[")"] + (y)`:        "P",
		`"esc \" }"`:          "E",
		`{a: "}"}["a"]`:       "M",
	}
	cases := []struct {
		name  string
		in    string
		want  any
		asked []string
	}{
		{"map literal embedded", `v=${ {"a": 1}.a } end`, "v=X end", []string{`{"a": 1}.a`}},
		{"double-quoted brace", `[${"}" + inputs.name}]`, "[}Ada]", []string{`"}" + inputs.name`}},
		{"single-quoted brace with escaped quote", `${'a\'}' + x}!`, "Q!", []string{`'a\'}' + x`}},
		{"raw-quoted brace", "<${`}`}>", "<R>", []string{"`}`"}},
		{"backslash does not escape in a raw string", "${`a\\`}", "${`a\\`}", []string{"`a\\`"}},
		{"nested brackets and parens", "${f({k: [1, {b: 2}]})}.", "N.", []string{`f({k: [1, {b: 2}]})`}},
		{"whole nested map keeps the typed value", `${{"a": {"b": 1}}}`, map[string]any{"a": map[string]any{"b": 1}}, []string{`{"a": {"b": 1}}`}},
		{"quoted paren does not nest", `${x[")"] + (y)}`, "P", []string{`x[")"] + (y)`}},
		{"escaped double quote", `${"esc \" }"}`, "E", []string{`"esc \" }"`}},
		{"map with quoted brace then index", `a ${{a: "}"}["a"]} b`, "a M b", []string{`{a: "}"}["a"]`}},
		{"unclosed nesting stays text", `a ${ {b: 1 } c`, `a ${ {b: 1 } c`, nil},
		{"unclosed quote stays text", `a ${ "} c`, `a ${ "} c`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var asked []string
			got := Interpolate(tc.in, evalFrom(vals, &asked))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Interpolate(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
			if !reflect.DeepEqual(asked, tc.asked) {
				t.Errorf("eval asked %q, want %q", asked, tc.asked)
			}
			if te := TemplateExpressions(tc.in); !reflect.DeepEqual(te, tc.asked) {
				t.Errorf("TemplateExpressions(%q) = %q, want %q", tc.in, te, tc.asked)
			}
		})
	}
}

func TestInterpolate_EvalAskedTrimmedExpressions(t *testing.T) {
	var asked []string
	Interpolate("a ${ x } b ${y}", evalFrom(nil, &asked))
	if !reflect.DeepEqual(asked, []string{"x", "y"}) {
		t.Errorf("eval asked %q, want [x y]", asked)
	}
}

func TestTemplateExpressions(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"plain text has none", "hello", nil},
		{"one template", "${inputs.name}", []string{"inputs.name"}},
		{"trimmed", "${  a.b  }", []string{"a.b"}},
		{"several in order", "x ${a} y ${b} z ${c}", []string{"a", "b", "c"}},
		{"empty template", "${}", []string{""}},
		{"quotes kept", `${"q" + a}`, []string{`"q" + a`}},
		{"nested brace stays inside", "${ {a: 1}.a }", []string{"{a: 1}.a"}},
		{"quoted brace stays inside", `${ "}" } and ${b}`, []string{`"}"`, "b"}},
		{"unclosed template ignored", "${a} and ${b", []string{"a"}},
		{"lone dollar is text", "$a {b}", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TemplateExpressions(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("TemplateExpressions(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TemplateExpressions lists exactly what Interpolate asks eval for.
func TestTemplateExpressions_MatchesInterpolateAsks(t *testing.T) {
	for _, s := range []string{"a ${x} b ${ y.z } c", "${one}", "${p}${q}"} {
		var asked []string
		Interpolate(s, evalFrom(nil, &asked))
		if got := TemplateExpressions(s); !reflect.DeepEqual(got, asked) {
			t.Errorf("%q: TemplateExpressions=%q, Interpolate asked %q", s, got, asked)
		}
	}
}
