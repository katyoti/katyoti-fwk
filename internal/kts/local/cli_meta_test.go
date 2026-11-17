// SPDX-License-Identifier: Apache-2.0

package local

import (
	"reflect"
	"testing"
)

type cliFilter struct {
	Module string `json:"module,omitempty"`
	Group  string `json:"group,omitempty"`
}

type cliPull struct {
	cliFilter
	ID     string    `json:"id,omitempty"`
	Group  string    `json:"group,omitempty"` // wins over the embedded one
	Nested cliFilter `json:"nested,omitempty"`
}

// An embedded struct's fields are flags of their own, as encoding/json reads
// them; a named struct field stays one flag; an outer field wins over an
// embedded one of the same name (cobra refuses a flag twice).
func TestCLIMetaHoistsEmbeddedFields(t *testing.T) {
	meta := BuildCLIMeta(reflect.TypeOf(cliPull{}))
	var got []string
	for _, f := range meta.Flags {
		got = append(got, f.Name)
	}
	want := []string{"id", "group", "nested", "module"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flags = %v, want %v", got, want)
	}
}
