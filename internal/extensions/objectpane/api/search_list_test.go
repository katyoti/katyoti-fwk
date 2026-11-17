// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

// A form field bound to one select sends its criterion as a string: the
// search reads it as the list a multi-select sends (the Objects panel failed
// with "cannot unmarshal string into ... SearchInput.types of type []string").
func TestSearchCriteriaReadAStringAsAList(t *testing.T) {
	cases := map[string]List{
		`{"types": ["decision", "feature"]}`: {"decision", "feature"},
		`{"types": "decision"}`:              {"decision"},
		`{"types": "decision, feature"}`:     {"decision", "feature"},
		`{"types": ""}`:                      nil,
		`{}`:                                 nil,
	}
	for in, want := range cases {
		var got SearchInput
		if err := json.Unmarshal([]byte(in), &got); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if !reflect.DeepEqual(got.Types, want) {
			t.Errorf("%s: types %q, want %q", in, got.Types, want)
		}
	}
	var bad SearchInput
	if err := json.Unmarshal([]byte(`{"types": 3}`), &bad); err == nil {
		t.Error("a number is not a list of types: want an error")
	}
}
