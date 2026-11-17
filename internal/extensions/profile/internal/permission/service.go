// SPDX-License-Identifier: Apache-2.0

// Package permission serves the permission codes: the listing of
// every act's code, declared or derived, and the check that answers for a code
// and the principal a call carries. Declaration only: until grants exist the
// check allows everything (study human-interaction, decision A4), so every
// place that asks it keeps working when grants come.
package permission

import (
	"context"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

// Service is the PermissionService over its sources.
type Service struct {
	// Person is the universe's person: whom a call with no cause, and an agent,
	// acts for.
	Person func() string
	// Sources lists the sources read at each listing: the binary's callables
	// and every contributed source.
	Sources func(ctx context.Context) []api.PermissionSource
}

var _ api.PermissionService = Service{}

// Codes lists every requirement of every source, one per code and act, sorted
// by code then by what needs it.
func (s Service) Codes(ctx context.Context, in api.PermissionCodesInput) (api.PermissionCodesOutput, error) {
	seen := map[string]bool{}
	out := []api.Requirement{}
	if s.Sources == nil {
		return api.PermissionCodesOutput{Requirements: out}, nil
	}
	for _, src := range s.Sources(ctx) {
		reqs, err := src.Requirements(ctx)
		if err != nil {
			return api.PermissionCodesOutput{}, fmt.Errorf("permission codes: source %s: %w", src.Name(), err)
		}
		for _, r := range reqs {
			if in.Kind != "" && r.Kind != in.Kind {
				continue
			}
			if err := api.ValidateCode(r.Code); err != nil {
				return api.PermissionCodesOutput{}, fmt.Errorf("permission codes: source %s, %s %s: %w", src.Name(), r.Kind, r.Of, err)
			}
			key := string(r.Code) + "\x00" + r.Kind + "\x00" + r.Of
			if seen[key] {
				continue
			}
			seen[key] = true
			if r.Source == "" {
				r.Source = src.Name()
			}
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return out[i].Of < out[j].Of
	})
	return api.PermissionCodesOutput{Requirements: out}, nil
}

// Check answers for a well-formed code: allowed, for the principal the call
// carries (or the one given) - no grant exists yet, so nothing is refused.
func (s Service) Check(ctx context.Context, in api.PermissionCheckInput) (api.PermissionCheckOutput, error) {
	if err := api.ValidateCode(in.Code); err != nil {
		return api.PermissionCheckOutput{}, err
	}
	person := ""
	if s.Person != nil {
		person = s.Person()
	}
	p := api.PrincipalOf(ctx, person)
	if in.Principal != nil {
		p = *in.Principal
	}
	return api.PermissionCheckOutput{
		Allowed:   true,
		Code:      in.Code,
		Principal: p,
		Reason:    "no grant is defined yet: everything is allowed to the universe's person (iam, decision A4)",
	}, nil
}

// Callables is the source of the methods, actions and tasks a binary offers:
// each callable URI with its derived code.
type Callables struct {
	List func(ctx context.Context) ([]string, error)
}

// Name names the source.
func (Callables) Name() string { return "callables" }

// Requirements derives the code of each callable URI; a URI that is no
// callable (a resource, an event) is skipped.
func (c Callables) Requirements(ctx context.Context) ([]api.Requirement, error) {
	uris, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.Requirement, 0, len(uris))
	for _, u := range uris {
		code, kind, err := api.CallableCode(u)
		if err != nil {
			continue
		}
		out = append(out, api.Requirement{Code: code, Kind: kind, Of: u})
	}
	return out, nil
}
