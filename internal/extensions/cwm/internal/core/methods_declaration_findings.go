// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// DeclarationFindings answers what is wrong with the universe's declarations as
// CWM reads them: what reading the catalog skipped or read leniently (a
// declaration that does not decode, one in a newer format), the object types
// it read and did not register, and every gate entry its provider's scope
// narrows (api.ScopeOf). The universe's health shows them; `ktw-compose
// lint-universe` computes the same answer from executables
// (api.DeclarationFindings).
func (s *Service) DeclarationFindings(_ stdctx.Context, _ api.DeclarationFindingsInput) (api.DeclarationFindingsOutput, error) {
	d := s.decl()
	decl := api.Declarations{
		Objects: d.Objects, Capabilities: d.Capabilities, Aspects: d.Aspects, Policies: d.Policies,
		HookTypes: d.HookTypes, Hooks: d.Hooks, EventTypes: d.EventTypes, Findings: d.Findings,
	}
	out := api.DeclarationFindings(decl, d.Grants)
	s.objTypesMu.RLock()
	for _, f := range s.typeRejections {
		out = append(out, f)
	}
	s.objTypesMu.RUnlock()
	out = append(out, s.conversionFindings()...)
	api.SortFindings(out)
	if out == nil {
		out = []api.DeclarationFinding{}
	}
	return api.DeclarationFindingsOutput{Findings: out}, nil
}

// recordRejection keeps an object type read and not registered as a finding.
// Called under objTypesMu.
func (s *Service) recordRejection(t api.ObjectType, msg string) {
	if s.typeRejections == nil {
		s.typeRejections = map[string]api.DeclarationFinding{}
	}
	s.typeRejections[t.Name] = api.DeclarationFinding{
		Rule: api.FindingRejected, Severity: api.FindingError, Kind: "object-type",
		Name: t.Name, Provider: t.Provider, Message: msg,
	}
}

// scope is the scope of the declarations in force, with the universe's grants.
func (s *Service) scope() api.Scope {
	d := s.decl()
	return api.NewScope(api.Declarations{Objects: d.Objects, Capabilities: d.Capabilities}, d.Grants)
}

// gatePoints is every point that may refuse a write (api.GatePoints).
func (s *Service) gatePoints() map[api.HookPoint]bool {
	return api.GatePoints(s.decl().HookTypes)
}

// eventPoints is every point an async hook entry may be declared at: the
// points CWM's own event types and every declared one name (api.EventPoints).
func (s *Service) eventPoints() map[api.HookPoint]bool {
	return api.EventPoints(s.decl().EventTypes)
}
