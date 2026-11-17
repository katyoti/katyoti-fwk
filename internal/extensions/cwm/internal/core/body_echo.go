// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// checkBodyEcho enforces the amended guard (concepts.md §5, phase 7c):
// NOTHING LIVES IN BOTH. A prose body that restates a datum a capability
// field already holds — a `## Want` paragraph repeating the description — is
// refused in every mode: two copies of one fact drift, and the body is never
// parsed to notice. The one allowed echo is the title as the body's first
// heading.
func checkBodyEcho(obj api.Object, verb string) error {
	prose := obj.Body
	if len(prose) < 20 {
		return nil
	}
	body := prose
	if strings.HasPrefix(body, "#") {
		if nl := strings.IndexByte(body, '\n'); nl >= 0 {
			body = body[nl:] // the first heading may echo the title
		} else {
			body = ""
		}
	}
	for field, v := range map[string]string{"description": obj.Description} {
		if len(strings.TrimSpace(v)) >= 20 && strings.Contains(body, strings.TrimSpace(v)) {
			return fmt.Errorf("%w: %s: object %q: the body repeats its %s verbatim — a datum lives in its field OR in the prose, never both",
				api.ErrInvalidInput, verb, obj.ID, field)
		}
	}
	for field, v := range obj.Fields {
		str, isStr := v.(string)
		if isStr && len(strings.TrimSpace(str)) >= 20 && strings.Contains(body, strings.TrimSpace(str)) {
			return fmt.Errorf("%w: %s: object %q: the body repeats its field %s verbatim — a datum lives in its field OR in the prose, never both",
				api.ErrInvalidInput, verb, obj.ID, field)
		}
	}
	for cap, group := range obj.Capabilities {
		for field, v := range group {
			str, isStr := v.(string)
			if !isStr || len(strings.TrimSpace(str)) < 20 {
				continue // short values (a status, a kind, a name) are words, not prose
			}
			if strings.Contains(body, strings.TrimSpace(str)) {
				return fmt.Errorf("%w: %s: object %q: the body repeats %s.%s verbatim — a datum lives in its field OR in the prose, never both",
					api.ErrInvalidInput, verb, obj.ID, cap, field)
			}
		}
	}
	return nil
}
