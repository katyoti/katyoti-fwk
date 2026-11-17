// SPDX-License-Identifier: Apache-2.0

package api

import (
	"reflect"
	"testing"
)

func TestHasCode(t *testing.T) {
	cases := []struct {
		codes, q string
		want     bool
	}{
		{"APP001", "APP001", true},
		{"APP001", "app001", true},
		{"XS;APP001", "APP001", true},
		{"XS ; APP001 ", " xs ", true},
		{"APP0010", "APP001", false},
		{"APP001", "APP", false},
		{"", "APP001", false},
		{"APP001;", "", false},
		{";;", " ", false},
	}
	for _, c := range cases {
		if got := HasCode(c.codes, c.q); got != c.want {
			t.Errorf("HasCode(%q, %q) = %v, want %v", c.codes, c.q, got, c.want)
		}
	}
}

func TestCodeMatchesFirst(t *testing.T) {
	type app struct{ Name, Code string }
	code := func(a app) string { return a.Code }
	items := []app{
		{"Billing APP001 report", "APP900"},
		{"Other", "XS;APP001"},
		{"Third", "APP002"},
		{"Fourth", "app001"},
	}
	got := CodeMatchesFirst(items, code, " APP001 ")
	want := []app{items[1], items[3], items[0], items[2]}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CodeMatchesFirst = %v, want %v", got, want)
	}
	if got := CodeMatchesFirst(items, code, "  "); !reflect.DeepEqual(got, items) {
		t.Errorf("empty query reordered: %v", got)
	}
	if got := CodeMatchesFirst(items, code, "NONE"); !reflect.DeepEqual(got, items) {
		t.Errorf("no match reordered: %v", got)
	}
}
