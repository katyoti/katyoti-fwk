// SPDX-License-Identifier: Apache-2.0

// Package core reads the verify and lint points: CWM runs the hooks
// (ObjectRunHooks) and answers them raw; this package counts a verify answer
// as PASS / FAIL / UNPROVEN and a lint answer as findings.
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// Service implements api.VerificationService over the CWMService in ctx.
type Service struct {
	// now replaces the clock in tests (a verdict's freshness).
	now func() time.Time
}

// New returns the service.
func New() *Service { return &Service{} }

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil || cwm == nil {
		return nil, fmt.Errorf("verification: no CWMService: %v", err)
	}
	return cwm, nil
}

// Verify runs the verify point over an object and everything under it, one
// object at a time, reporting each check as it answers (report.go): every
// check runs, a fresh verdict counts for nothing here.
func (s *Service) Verify(ctx context.Context, in api.VerifyInput) (api.VerifyOutput, error) {
	if in.ID == "" {
		return api.VerifyOutput{}, fmt.Errorf("%w: ID required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.VerifyOutput{}, err
	}
	return s.walkVerify(ctx, cwm, in.ID, nil)
}

// readVerify counts every verify answer.
func readVerify(r cwmapi.ObjectRunHooksOutput) api.VerifyOutput {
	out := api.VerifyOutput{Objects: r.Objects, Runs: len(r.Runs), Outcomes: []api.VerifyOutcome{}}
	for _, run := range r.Runs {
		o := verifyOutcome(run)
		out.Outcomes = append(out.Outcomes, o)
		switch o.Status {
		case "pass":
			out.Passed++
		case "fail":
			out.Failed = append(out.Failed, o)
		default:
			out.Unproven = append(out.Unproven, o)
		}
	}
	return out
}

// verifyOutcome reads one verify answer.
func verifyOutcome(r cwmapi.HookRun) api.VerifyOutcome {
	o := api.VerifyOutcome{ObjectID: r.ObjectID, URI: r.URI, Source: r.Source}
	if r.Error != "" {
		o.Status, o.Evidence = "hook-error", r.Error
		return o
	}
	var ans struct {
		Status   string `json:"status"`
		Evidence string `json:"evidence"`
		Desc     string `json:"desc"`
		Verdict  string `json:"verdict"`
		Timeout  string `json:"timeout"`
	}
	if len(r.Answer) == 0 || json.Unmarshal(r.Answer, &ans) != nil || ans.Status == "" {
		o.Status, o.Evidence = "hook-error", "answered something that is not a verify result {status}"
		return o
	}
	o.Status, o.Verdict, o.Evidence, o.Timeout = ans.Status, ans.Verdict, ans.Evidence, ans.Timeout
	if o.Evidence == "" {
		o.Evidence = ans.Desc
	}
	return o
}

// Lint runs the lint point over one object (with Subtree, and everything
// under it) or a module's objects. Exactly one of ID and Module, and Subtree
// only with ID: an input naming both, or neither, is refused here, before any
// hook runs.
func (s *Service) Lint(ctx context.Context, in api.LintInput) (api.LintOutput, error) {
	if (in.ID == "") == (in.Module == "") {
		return api.LintOutput{}, fmt.Errorf("%w: lint takes exactly one of id and module", cwmapi.ErrInvalidInput)
	}
	if in.Subtree && in.ID == "" {
		return api.LintOutput{}, fmt.Errorf("%w: subtree goes with id, not module", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.LintOutput{}, err
	}
	runs, err := cwm.ObjectRunHooks(ctx, cwmapi.ObjectRunHooksInput{Point: cwmapi.HookLint, ID: in.ID, Module: in.Module, Subtree: in.Subtree})
	if err != nil {
		return api.LintOutput{}, err
	}
	return readLint(runs, scopeSize(ctx, cwm, in)), nil
}

// scopeSize is how many objects a lint covered: one, the object and its
// descendants, or the module's.
func scopeSize(ctx context.Context, cwm cwmapi.CWMService, in api.LintInput) int {
	switch {
	case in.Module != "":
		l, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: in.Module})
		if err != nil {
			return 0
		}
		return len(l.Objects)
	case in.Subtree:
		tr, err := cwm.ObjectTree(ctx, cwmapi.ObjectTreeInput{Root: in.ID})
		if err != nil {
			return 0
		}
		return tr.Total
	default:
		return 1
	}
}

// readLint reads every lint answer as findings.
func readLint(r cwmapi.ObjectRunHooksOutput, objects int) api.LintOutput {
	out := api.LintOutput{Findings: []api.LintFinding{}, Objects: objects, Runs: len(r.Runs)}
	for _, run := range r.Runs {
		if run.Error != "" {
			out.Findings = append(out.Findings, lintHookError(run, run.Error))
			continue
		}
		var lr struct {
			Status   string `json:"status"`
			Findings []struct {
				Rule     string `json:"rule"`
				Message  string `json:"message"`
				Severity string `json:"severity"`
			} `json:"findings"`
		}
		if len(run.Answer) == 0 || json.Unmarshal(run.Answer, &lr) != nil || lr.Status == "" {
			out.Findings = append(out.Findings, lintHookError(run, "answered something that is not a lint result {status, findings}"))
			continue
		}
		for _, f := range lr.Findings {
			out.Findings = append(out.Findings, api.LintFinding{
				ObjectID: run.ObjectID, URI: run.URI, Source: run.Source,
				Rule: f.Rule, Message: f.Message, Severity: f.Severity,
			})
		}
	}
	return out
}

func lintHookError(r cwmapi.HookRun, why string) api.LintFinding {
	return api.LintFinding{
		ObjectID: r.ObjectID, URI: r.URI, Source: r.Source,
		Rule: "lint-hook-error", Message: r.URI + ": " + why, Severity: "error",
	}
}

// VerifyFunction is `verify()`: the object and everything under it, a
// criterion with a fresh verdict counted from it.
func (s *Service) VerifyFunction(ctx context.Context, in cwmapi.FunctionCall) (api.VerifySummary, error) {
	if in.Object == "" {
		return api.VerifySummary{}, fmt.Errorf("%w: verify() needs an object", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.VerifySummary{}, err
	}
	v, err := s.verifyFresh(ctx, cwm, in.Object)
	if err != nil {
		return api.VerifySummary{}, err
	}
	return api.VerifySummary{Runs: v.Runs, Passed: v.Passed, Failed: len(v.Failed), Unproven: len(v.Unproven), Green: v.Green()}, nil
}

// LintFunction is `lint()`: the object and everything under it, with the
// findings at or above the object type's `verification.gate_blocks_on`.
func (s *Service) LintFunction(ctx context.Context, in cwmapi.FunctionCall) (api.LintSummary, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.LintSummary{}, err
	}
	runs, err := cwm.ObjectRunHooks(ctx, cwmapi.ObjectRunHooksInput{Point: cwmapi.HookLint, ID: in.Object, Subtree: true})
	if err != nil {
		return api.LintSummary{}, err
	}
	l := readLint(runs, runs.Objects)
	bar := ""
	module, _ := cwmapi.ModuleOfObjectID(in.Object)
	if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.Object}); err == nil {
		if t, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: got.Object.Type}); err == nil {
			// The type's `verification` section; one that does not decode
			// leaves the default bar (CWM refuses such a type when declared).
			if vs, err := api.VerificationOf(t.ObjectType); err == nil {
				bar = vs.GateBlocksOn
			}
		}
	}
	out := api.LintSummary{Findings: len(l.Findings)}
	for _, f := range l.Findings {
		if BlocksAt(f.Severity, bar) {
			out.Blocking++
		}
	}
	return out, nil
}

// severityRank orders lint severities; an unknown one ranks as an error, so a
// misspelt severity cannot slip under the bar.
func severityRank(sev string) int {
	switch sev {
	case "info":
		return 1
	case "warn", "warning":
		return 2
	default:
		return 3
	}
}

// BlocksAt reports a finding at or above a type's gate_blocks_on (error when
// unset); `none` blocks nothing.
func BlocksAt(sev, bar string) bool {
	if bar == "" {
		bar = "error"
	}
	return bar != "none" && severityRank(sev) >= severityRank(bar)
}
