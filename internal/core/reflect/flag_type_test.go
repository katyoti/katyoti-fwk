// SPDX-License-Identifier: Apache-2.0

package reflect

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

type flagActor struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// A CLI flag's type says how its text becomes the field's value. A struct or
// a list of structs takes JSON: read as a plain string, the dispatcher could
// not decode it into the field (an actor, deliverables).
func TestGoTypeToFlagType(t *testing.T) {
	cases := map[string]struct {
		v    any
		want string
	}{
		"bool":          {true, "boolean"},
		"int":           {1, "int"},
		"float":         {1.5, "float"},
		"string":        {"x", "string"},
		"strings":       {[]string{}, "stringSlice"},
		"string map":    {map[string]string{}, "stringMap"},
		"any map":       {map[string]any{}, "jsonObject"},
		"struct":        {flagActor{}, "jsonValue"},
		"struct ptr":    {&flagActor{}, "jsonValue"},
		"struct slice":  {[]flagActor{}, "jsonValue"},
		"time":          {time.Time{}, "string"},
		"bytes":         {[]byte{}, "bytes"},
		"raw json":      {json.RawMessage{}, "jsonValue"}, // the JSON as written lands as is (an action's input), never quoted
		"string ptr":    {new(string), "string"},
		"slice of ints": {[]int{}, "jsonValue"},
	}
	for name, c := range cases {
		if got := GoTypeToFlagType(reflect.TypeOf(c.v)); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}
