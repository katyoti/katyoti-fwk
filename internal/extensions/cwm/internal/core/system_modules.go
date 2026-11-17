// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// SYSTEM MODULES (decided with the user 2026-09-26).
//
// THE RULE: `sys-<name>` modules hold the platform's COMMITTED state;
// `.katyoti` holds its UNCOMMITTED state. An operational space the platform
// keeps for itself — the inbox and the messages filed into it — is a module
// named `sys-<name>`, committed with the repository, beside the functional
// module that describes the concept (`inbox` documents the inbox; `sys-inbox`
// holds its messages). What must not be committed stays in `.katyoti`.
//
// The prefix is RESERVED BOTH WAYS, so it can never disagree with the fact it
// signals: a module named `sys-*` must carry the `system-module` capability,
// and a module carrying it must be named `sys-*`.
//
// The prefix and the capability are the API's (api.SystemModulePrefix,
// api.CapSystemModule): a consumer tells a system module apart by the
// capability through api.IsSystemModule / api.UserModules, never by the name.
//
// The rule is refused on every write (ObjectCreate, ObjectUpdate, a change-set
// commit) and REPORTED on every object read (validityProblems): a module file
// written by hand that breaks it carries the problem, like any other invalid
// object.

// checkSystemModuleName enforces the reservation on a MODULE object (a bare,
// parentless id). It applies in every validation mode: a reserved name is not
// a type-checking nicety that a migration may relax.
func (s *Service) checkSystemModuleName(obj api.Object, verb string) error {
	if strings.Contains(obj.ID, ".") {
		return nil // not a module object
	}
	named := strings.HasPrefix(obj.ID, api.SystemModulePrefix)
	carries := s.hasEffective(obj, api.CapSystemModule)
	switch {
	case named && !carries:
		return fmt.Errorf("%w: %s: %q uses the reserved prefix %q — only a module carrying the %s capability may",
			api.ErrInvalidInput, verb, obj.ID, api.SystemModulePrefix, api.CapSystemModule)
	case carries && !named:
		return fmt.Errorf("%w: %s: %q carries %s and must be named %s<name>",
			api.ErrInvalidInput, verb, obj.ID, api.CapSystemModule, api.SystemModulePrefix)
	}
	// What it operates is a capability the universe declares: the schema
	// types the field, the name is checked here.
	if op := api.SystemModuleOperates(obj); carries && op != "" {
		if _, declared := s.capabilitiesByName()[op]; !declared {
			return fmt.Errorf("%w: %s: %q operates %q, which no capability of the universe is named",
				api.ErrInvalidInput, verb, obj.ID, op)
		}
	}
	return nil
}
