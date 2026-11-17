// SPDX-License-Identifier: Apache-2.0

package schemaapi

import (
	"encoding/json"
	"fmt"
)

// An extension travels as the DSL writes it and the canonical JSON keeps it:
// one flat object, `type` beside the payload's own keys —
// `{"type": "ui-form", "name": "default", "layout": "tabs", "groups": […]}`.
// Without these two methods a plain encoding (a KTSP answer, the GUI's
// re-marshal) kept only `type` and dropped the payload: every ui-form reached
// a form without its layout, groups or labels.

// MarshalJSON emits the flat form.
func (e Extension) MarshalJSON() ([]byte, error) {
	out := make(map[string]any, len(e.Payload)+1)
	for k, v := range e.Payload {
		if k == "type" {
			continue
		}
		out[k] = v
	}
	out["type"] = e.Kind
	return json.Marshal(out)
}

// UnmarshalJSON reads the flat form: `type` is the kind, every other key the
// payload.
func (e *Extension) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	kind, ok := m["type"].(string)
	if !ok {
		return fmt.Errorf("extension: a string `type` is required")
	}
	delete(m, "type")
	e.Kind = kind
	e.Payload = m
	return nil
}
