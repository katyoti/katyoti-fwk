// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/methodsservice"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A RELEASE'S GRADE — a release is the committed slice of a project's wants:
// what targets it is what it carries, and what it names `judged-by` (among
// those) is what judges it. Its grade runs no check of its own: each member
// is verified by the verification service and the outcomes are rolled up by
// the method's Rollup — green only when every member passed.

// Grade rolls up the verification of what judges the release, else of what
// targets it. A member that resolves to no object is graded missing, never left
// out: a rollup over fewer members than the release names reads greener than
// the truth.
func (r *Releases) Grade(ctx context.Context, in api.GetInput) (api.GradeOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.GradeOutput{}, err
	}
	verification, err := application.UseService[verificationapi.VerificationService](ctx)
	if err != nil || verification == nil {
		return api.GradeOutput{}, fmt.Errorf("releases.Grade: no VerificationService: %v", err)
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.GradeOutput{}, err
	}
	out := api.GradeOutput{Release: rel.ID, Over: api.EdgeJudgedBy, Members: []api.MemberGrade{}}
	members := edgeTargets(rel, api.EdgeJudgedBy)
	if len(members) == 0 {
		out.Over = api.EdgeTargets
		if members, err = workOf(ctx, cwm, rel.ID); err != nil {
			return api.GradeOutput{}, err
		}
	}
	checks := make([]methodsservice.CheckStatus, 0, len(members))
	for _, id := range members {
		m := gradeMember(ctx, cwm, verification, id)
		out.Members = append(out.Members, m)
		checks = append(checks, methodsservice.CheckStatus{ID: id, Status: m.Status})
	}
	roll, err := methodsservice.New().Rollup(ctx, methodsservice.RollupInput{Checks: checks})
	if err != nil {
		return api.GradeOutput{}, err
	}
	out.Status, out.Total, out.Passed, out.Progress, out.Frontier = roll.Status, roll.Total, roll.Passed, roll.Progress, roll.Unmet
	return out, nil
}

// gradeMember verifies one member: pass when its verification is green, fail
// when a check failed, unproven when it proved nothing either way.
func gradeMember(ctx context.Context, cwm cwmapi.CWMService, v verificationapi.VerificationService, id string) api.MemberGrade {
	m := api.MemberGrade{ID: id}
	if _, _, err := loadAny(ctx, cwm, id); err != nil {
		if errors.Is(err, cwmapi.ErrNotFound) {
			m.Status, m.Reason = api.MemberMissing, "no object "+id
		} else {
			m.Status, m.Reason = api.MemberError, err.Error()
		}
		return m
	}
	res, err := v.Verify(ctx, verificationapi.VerifyInput{ID: id})
	if err != nil {
		m.Status, m.Reason = api.MemberError, err.Error()
		return m
	}
	m.Runs, m.Passed, m.Failed, m.Unproven = res.Runs, res.Passed, len(res.Failed), len(res.Unproven)
	for _, o := range res.Outcomes {
		if o.Timeout != "" {
			m.Extended = append(m.Extended, o.ObjectID+" ("+o.Timeout+")")
		}
	}
	switch {
	case res.Green():
		m.Status = api.MemberPass
	case len(res.Failed) > 0:
		m.Status = api.MemberFail
	default:
		m.Status = api.MemberUnproven
		if res.Runs == 0 {
			m.Reason = "nothing to verify"
		}
	}
	return m
}

// edgeTargets is the distinct targets of obj's edges of one kind, in order.
func edgeTargets(obj cwmapi.Object, kind string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range obj.Edges {
		if e.Kind == kind && e.To != "" && !seen[e.To] {
			seen[e.To] = true
			out = append(out, e.To)
		}
	}
	return out
}

// judgedOutside is what a release names judged-by among things that do not
// target it, sorted: a release is judged by a subset of what it carries.
func judgedOutside(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object) ([]string, error) {
	judged := edgeTargets(rel, api.EdgeJudgedBy)
	if len(judged) == 0 {
		return nil, nil
	}
	carried, err := workOf(ctx, cwm, rel.ID)
	if err != nil {
		return nil, err
	}
	in := map[string]bool{}
	for _, id := range carried {
		in[id] = true
	}
	var out []string
	for _, id := range judged {
		if !in[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

// judgedOutsideFinding is the lint finding for a release judged by what does
// not target it, or nil.
func judgedOutsideFinding(rel string, outside []string) *api.LintFinding {
	if len(outside) == 0 {
		return nil
	}
	return &api.LintFinding{Rule: "judged-outside-targets", Severity: "error",
		Message: fmt.Sprintf("%s is judged by %s, which does not target it — a release is judged by a subset of what targets it", rel, strings.Join(outside, ", "))}
}
