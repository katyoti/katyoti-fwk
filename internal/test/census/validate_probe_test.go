// SPDX-License-Identifier: Apache-2.0

package census

import (
	"context"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// TestValidatorActuallyChecks pins what the source-side check actually catches,
// so the guarantee this milestone claims matches the validator's real depth.
//
// Worth pinning because the depth was not what the code said: the service's own
// comment claims field-level checks "ride a future schema-aware validator", but
// it already rejects a missing required field and a wrong field type. A probe
// beats a comment — and if the depth ever regresses, this fails rather than the
// claim quietly becoming false.
func TestValidatorActuallyChecks(t *testing.T) {
	Isolate(t)
	svc := schemaService(t)
	const uri = "test:/data-type/probe"
	if err := svc.RegisterDataType(context.Background(), schemaapi.RegisterDataTypeInput{
		URI: uri,
		Body: []byte(`$schema: ktw-schema/v1
name: probe
version: "1.0.0"
root: Probe
types:
  Probe:
    type: object
    fields:
      required_field:
        type: string
`),
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	for _, tc := range []struct {
		name string
		body string
		want bool // want valid
	}{
		{"well-formed body", `{"required_field":"x"}`, true},
		{"not JSON at all", `{not json`, false},
		{"missing a required field", `{}`, false},
		{"wrong field type", `{"required_field":123}`, false},
	} {
		out, err := svc.ValidateAgainst(context.Background(), schemaapi.ValidateAgainstInput{
			URI: uri, Body: []byte(tc.body),
		})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if out.Valid != tc.want {
			t.Errorf("%s: valid=%v want %v — the validator's depth changed; update what this milestone claims",
				tc.name, out.Valid, tc.want)
		}
	}
}
