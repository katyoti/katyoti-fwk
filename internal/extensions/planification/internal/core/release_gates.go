// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A RELEASE IS PROMOTED ONCE ITS GATES PASS (decision) - a
// frozen release is delivered as candidates (rc1, rc2...), each with its
// build; a candidate is promoted to the official version only when every gate
// passes on it: every increment the release carries done, every increment of
// another stream one of them depends on done, no open P0 fix on the release or
// on what it carries, the end-to-end test green on the candidate's build, and
// the alpha sign-off - a person's approval naming it. Promotion ships the
// candidate's own build: nothing is rebuilt.

// Gates answers where a release stands toward its promotion.
func (r *Releases) Gates(ctx context.Context, in api.GatesInput) (api.GatesOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.GatesOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.GatesOutput{}, err
	}
	return gatesOf(ctx, cwm, rel, in.Candidate)
}

// gatesOf reads a release's gates on one of its candidates (the last one when
// none is named).
func gatesOf(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, name string) (api.GatesOutput, error) {
	f := releaseFields(rel)
	out := api.GatesOutput{Release: rel.ID, Stage: stageOf(rel), Gates: []api.GateLine{}}
	var cand *api.Candidate
	for i := range f.Candidates {
		if name == "" || f.Candidates[i].Name == name {
			cand = &f.Candidates[i]
		}
	}
	if name != "" && cand == nil {
		return out, invalid("Gates: %s has no candidate %s", rel.ID, name)
	}
	if cand != nil {
		out.Candidate, out.Build = cand.Name, cand.Build
	}
	carried := carriedIncrements(ctx, cwm, rel)

	// Its increments done.
	var open []string
	for _, inc := range carried {
		if st := releaseState(inc); st != api.IncrementDone && st != api.IncrementDropped {
			open = append(open, titleOf(inc))
		}
	}
	out.Gates = append(out.Gates, gateLine(api.GateIncrements, len(open) == 0, "open: "+strings.Join(open, ", ")))

	// The increments of other streams they depend on done.
	sm := edgeTo(rel, api.EdgeInStream)
	var deps []string
	seenDep := map[string]bool{}
	for _, inc := range carried {
		for _, d := range edgesTo(inc, api.EdgeDependsOn) {
			do, _, err := loadAny(ctx, cwm, d)
			if err != nil || do.Type != api.IncrementType || (sm != "" && edgeTo(do, api.EdgeInStream) == sm) || seenDep[d] {
				continue
			}
			seenDep[d] = true
			if st := releaseState(do); st != api.IncrementDone && st != api.IncrementDropped {
				deps = append(deps, titleOf(do)+" ("+titleOfID(ctx, cwm, edgeTo(do, api.EdgeInStream))+")")
			}
		}
	}
	out.Gates = append(out.Gates, gateLine(api.GateDependencies, len(deps) == 0, "open: "+strings.Join(deps, ", ")))

	// No open P0 fix on it, nor on what it carries.
	var fixes []string
	for _, id := range workOnRelease(ctx, cwm, rel, carried) {
		o, err := loadObject(ctx, cwm, id)
		if err != nil || !hasRequest(o) {
			continue
		}
		// What its testing filed: every request filed since the candidate
		// was delivered, of any kind, ended ones included.
		if cand != nil && !before(fieldsOf(o).FiledAt, cand.At) && !slices.Contains(out.Filed, id) {
			out.Filed = append(out.Filed, id)
		}
		if planEnded(planState(o)) || fieldsOf(o).Kind != api.KindFix {
			continue
		}
		if p, _ := planOf(o); p.Priority == "P0" {
			fixes = append(fixes, titleOf(o))
		}
	}
	out.Gates = append(out.Gates, gateLine(api.GateNoOpenP0Fix, len(fixes) == 0, "open: "+strings.Join(fixes, ", ")))

	// The end-to-end test green on the candidate's build, and its sign-off.
	e2e, why := false, "no candidate delivered yet"
	if cand != nil {
		why = ""
	}
	signed, swhy := false, "no candidate delivered yet"
	if cand != nil {
		why = "not run on " + cand.Name
		for _, g := range cand.Gates {
			if g.Name == api.GateEndToEnd {
				e2e, why = g.Passed, g.Why
				if why == "" {
					why = "failed on " + cand.Name + " (no reason recorded)"
				}
			}
		}
		signed, swhy = cand.SignedOff != nil, "no person signed "+cand.Name+" off"
	}
	out.Gates = append(out.Gates, gateLine(api.GateEndToEnd, e2e, why), gateLine(api.GateSignOff, signed, swhy))
	if l, ok := changeLogLine(ctx, cwm, rel); ok {
		out.Gates = append(out.Gates, l)
	}
	out.Ready = cand != nil && !slices.ContainsFunc(out.Gates, func(g api.GateLine) bool { return !g.Passed })
	return out, nil
}

// changeLogLine is the release's ready-to-release gate as a gate line: its
// unmet requirements and their reasons (a person's confirmation, asked in the
// Inbox), and the work open to meet them. Without the gates service there is
// no line.
func changeLogLine(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object) (api.GateLine, bool) {
	gates, err := application.UseService[gatesapi.GatesService](ctx)
	if err != nil {
		return api.GateLine{}, false
	}
	chk, err := gates.Check(ctx, gatesapi.CheckInput{ID: rel.ID, Gate: api.GateReadyToRelease})
	if err != nil {
		return api.GateLine{}, false
	}
	var why []string
	for _, it := range chk.Checklist.Unmet() {
		why = append(why, it.Label+": "+it.Reason)
	}
	if work, _ := incoming(ctx, cwm, rel.ID, api.EdgeResolves); len(work) > 0 {
		for _, id := range work {
			if o, err := load(ctx, cwm, id); err == nil && !planEnded(planState(o)) {
				why = append(why, "its work is open: "+titleOf(o))
			}
		}
	}
	return gateLine(api.GateChangeLog, chk.Checklist.Passes, strings.Join(why, "; ")), true
}

// before is whether time a is before time b (RFC 3339, with or without
// fractions); an unreadable one is compared as text.
func before(a, b string) bool {
	ta, ea := time.Parse(time.RFC3339Nano, a)
	tb, eb := time.Parse(time.RFC3339Nano, b)
	if ea != nil || eb != nil {
		return a < b
	}
	return ta.Truncate(time.Second).Before(tb.Truncate(time.Second))
}

// gateLine is a gate's line: its reason only when it does not pass.
func gateLine(name string, passed bool, why string) api.GateLine {
	if passed {
		why = ""
	}
	return api.GateLine{Name: name, Passed: passed, Why: why}
}

// stageOf is a release's stage (api.ReleaseStage).
func stageOf(rel cwmapi.Object) string {
	return api.ReleaseStage(api.Release{State: releaseState(rel), ReleaseFields: releaseFields(rel)})
}

// carriedIncrements is the increments a release carries: those shipping in it.
func carriedIncrements(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object) []cwmapi.Object {
	ids, _ := incoming(ctx, cwm, rel.ID, api.EdgeShipsIn)
	var out []cwmapi.Object
	for _, id := range ids {
		if o, _, err := loadAny(ctx, cwm, id); err == nil && o.Type == api.IncrementType {
			out = append(out, o)
		}
	}
	return out
}

// workOnRelease is the requests about a release: the release's own, the direct
// work targeting it, and what targets the increments it carries.
func workOnRelease(ctx context.Context, cwm cwmapi.CWMService, rel cwmapi.Object, carried []cwmapi.Object) []string {
	var out []string
	if hasRequest(rel) {
		out = append(out, rel.ID)
	}
	direct, _ := workOf(ctx, cwm, rel.ID)
	out = append(out, direct...)
	for _, inc := range carried {
		ids, _ := workOf(ctx, cwm, inc.ID)
		out = append(out, ids...)
	}
	return out
}

// RecordGate records a gate's result on a candidate's build: the end-to-end
// test on a clean machine.
func (r *Releases) RecordGate(ctx context.Context, in api.RecordGateInput) (api.ReleaseOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.ReleaseOutput{}, err
	}
	if in.Gate != api.GateEndToEnd {
		return api.ReleaseOutput{}, invalid("RecordGate: %q is not a gate recorded on a build (e2e)", in.Gate)
	}
	return onCandidate(ctx, in.ID, in.Candidate, func(c *api.Candidate) error {
		res := api.GateResult{Name: in.Gate, Passed: in.Passed, Why: strings.TrimSpace(in.Why), At: time.Now().UTC().Format(time.RFC3339), By: in.By}
		c.Gates = slices.DeleteFunc(c.Gates, func(g api.GateResult) bool { return g.Name == in.Gate })
		c.Gates = append(c.Gates, res)
		return nil
	})
}

// SignOff is the alpha sign-off: a person approves a candidate.
func (r *Releases) SignOff(ctx context.Context, in api.CandidateInput) (api.ReleaseOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.ReleaseOutput{}, err
	}
	if in.By.Kind != "person" {
		return api.ReleaseOutput{}, invalid("SignOff: the alpha sign-off is a person's - %s is a %s", in.By.ID, in.By.Kind)
	}
	return onCandidate(ctx, in.ID, in.Candidate, func(c *api.Candidate) error {
		if strings.TrimSpace(c.Letter) == "" {
			return invalid("SignOff: %s %s has no letter - the sign-off approves it (releases letter writes it)", in.ID, in.Candidate)
		}
		c.SignedOff = &api.SignOff{By: in.By, At: time.Now().UTC().Format(time.RFC3339)}
		return nil
	})
}

// Promote promotes a candidate to the official version once every gate passes
// on it: its build is the release, nothing rebuilt, and the release is released.
func (r *Releases) Promote(ctx context.Context, in api.CandidateInput) (api.ReleaseOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.ReleaseOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	if st := releaseState(rel); st != api.ReleaseFrozen {
		return api.ReleaseOutput{}, invalid("Promote: %s is %s - a frozen release's candidate is promoted", in.ID, st)
	}
	cs := releaseFields(rel).Candidates
	if len(cs) == 0 {
		return api.ReleaseOutput{}, invalid("Promote: %s has no candidate (releases deliver makes one)", in.ID)
	}
	name := in.Candidate
	if name == "" {
		name = cs[len(cs)-1].Name
	}
	if last := cs[len(cs)-1].Name; name != last {
		return api.ReleaseOutput{}, invalid("Promote: %s is not %s's last candidate (%s): the last one becomes the official version", name, in.ID, last)
	}
	g, err := gatesOf(ctx, cwm, rel, name)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	// The candidate's gates; the change log is the release's own guard's,
	// tried by the move below (its refusal files the work, then asks the
	// confirmation).
	var failing []string
	for _, l := range g.Gates {
		if !l.Passed && l.Name != api.GateChangeLog {
			failing = append(failing, l.Name+" ("+l.Why+")")
		}
	}
	if len(failing) > 0 {
		return api.ReleaseOutput{}, invalid("Promote: %s %s is not ready - %s", in.ID, name, strings.Join(failing, "; "))
	}
	note := "promoted " + name
	if g.Build != "" {
		note += " (build " + g.Build + ", nothing rebuilt)"
	}
	if strings.TrimSpace(in.Note) != "" {
		note += ": " + strings.TrimSpace(in.Note)
	}
	// Released first - its own gates may still refuse it; only then is the
	// candidate recorded as the official build.
	if _, err := NewReleases().Release(ctx, api.MoveInput{ID: in.ID, By: in.By, Note: note}); err != nil {
		return api.ReleaseOutput{}, err
	}
	rel, err = loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	f := releaseFields(rel)
	for i := range f.Candidates {
		f.Candidates[i].Promoted = f.Candidates[i].Name == name
	}
	out, err := setFields(ctx, cwm, rel, f)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	return api.ReleaseOutput{Release: toRelease(out)}, nil
}

// onCandidate changes one candidate of a frozen release, not promoted yet.
func onCandidate(ctx context.Context, id, name string, change func(*api.Candidate) error) (api.ReleaseOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	o, err := loadRelease(ctx, cwm, id)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	f := releaseFields(o)
	i := slices.IndexFunc(f.Candidates, func(c api.Candidate) bool { return c.Name == name })
	if i < 0 {
		return api.ReleaseOutput{}, invalid("%s has no candidate %q (releases deliver makes one)", id, name)
	}
	if f.Candidates[i].Promoted || releaseState(o) != api.ReleaseFrozen {
		return api.ReleaseOutput{}, invalid("%s %s is settled: its release is %s - a candidate is recorded on while its release is frozen", id, name, stageOf(o))
	}
	if err := change(&f.Candidates[i]); err != nil {
		return api.ReleaseOutput{}, err
	}
	out, err := setFields(ctx, cwm, o, f)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	return api.ReleaseOutput{Release: toRelease(out)}, nil
}
