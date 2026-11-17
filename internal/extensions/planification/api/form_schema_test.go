// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/reflect"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The filing form is drawn from FileInput's schema: each field says what it
// wants, and kind and priority are value lists — a plain-text jsonschema tag
// is dropped silently, so this guards the `description=` form.
func TestTheFilingFormSaysWhatEachFieldWants(t *testing.T) {
	raw, err := json.Marshal(reflect.MustJSONSchema[api.FileInput]())
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Properties map[string]struct {
			Description string `json:"description"`
			Enum        []any  `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"title", "details", "about", "on", "module", "kind", "priority", "by"} {
		if s.Properties[f].Description == "" {
			t.Errorf("%s has no description", f)
		}
	}
	if len(s.Properties["kind"].Enum) != len(api.Kinds) || len(s.Properties["priority"].Enum) != 4 {
		t.Errorf("kind %v, priority %v; want their value lists", s.Properties["kind"].Enum, s.Properties["priority"].Enum)
	}
}
