// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TYPE SECTIONS AGAINST THEIR CAPABILITY. A type's section (`context:`,
// `pane:`, `verification:`) is a capability's settings on that type: CWM keeps
// it as written and checks it, when the type is declared, against the
// capability's `type_schema` — it never reads what it means. A section naming
// no declared capability, or a capability with no `type_schema`, is refused
// like an undeclared capability; one that does not conform to its schema is
// refused the same way. Refused means the TYPE is not registered (error mode)
// — the way an unscannable folder is — and is asked again on the next
// refresh, when the capability may have arrived; warn logs and keeps it.

// declarationMode is how a type DECLARATION is checked: the configured
// validation mode, without the no-types-declared exemption — that exemption is
// about objects of an unmigrated workspace, and a type being declared is the
// opposite of unmigrated.
func (s *Service) declarationMode() string {
	switch s.Config.ObjectTypeValidation {
	case api.TypeValidationOff, api.TypeValidationWarn:
		return s.Config.ObjectTypeValidation
	default:
		return api.TypeValidationError
	}
}

// sectionProblems is every problem of a type's sections, given the declared
// capabilities. No capability declared at all ⇒ nothing is checked: a
// workspace without capabilities has nothing to check a section against.
func (s *Service) sectionProblems(t api.ObjectType, caps []api.Capability) []string {
	if len(t.Sections) == 0 || len(caps) == 0 {
		return nil
	}
	declared := make(map[string]api.Capability, len(caps))
	for _, c := range caps {
		declared[c.Name] = c
	}
	names := make([]string, 0, len(t.Sections))
	for n := range t.Sections {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []string
	for _, name := range names {
		c, ok := declared[name]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("section %q names no declared capability", name))
			continue
		case c.TypeSchema == "":
			out = append(out, fmt.Sprintf("section %q: capability %q declares no type_schema, so it has no section on a type", name, name))
			continue
		}
		sch, ok := s.capabilitySchema(c.TypeSchema)
		if !ok {
			// Unresolved: the section cannot be checked. Said once, never
			// passed silently as valid.
			if _, seen := s.unresolvedSchemas.LoadOrStore(c.TypeSchema, true); !seen {
				slog.Warn("cwm: a type_schema is not resolved; the sections using it are not checked", "schema", c.TypeSchema, "capability", name)
			}
			continue
		}
		raw, err := json.Marshal(t.Sections[name])
		var v any
		if err == nil {
			err = json.Unmarshal(raw, &v)
		}
		if err == nil {
			err = sch.Validate(v)
		}
		if err != nil {
			out = append(out, fmt.Sprintf("section %q (%s): %s", name, c.TypeSchema, strings.Join(validationProblems(err), "; ")))
		}
	}
	return out
}
