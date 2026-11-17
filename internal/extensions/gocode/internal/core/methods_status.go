// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/coverage"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/modulehealth"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/lint"
)

// ModuleStatus is the read-only state report for a module. Returns
// the typed module + UC counters (4-verdict projection) + open OD
// count + open finding counts (critical/high) + the list of
// promote-gate conditions currently failing.
//
// Pure read — no items written. Cheap to call (re-uses the same
// loader as ModuleManifestWrite + ModuleVerify).
//
// Gates evaluated against modules/code-gocode/concepts.md §6 / the tier-promotion FSM:
//
//  1. Charter v ≥ 0.1
//  2. All UCs pass-or-fail-deferred (no fail, no no-test)
//  3. Latest audit review: 0 critical + 0 high findings open
//  4. (lint clean — deferred until lint-detectors verb lands)
//  5. (no API/code-symbol drift — deferred until go-ast-discovery lands)
//
// Gates 1-3 are computable today; 4-5 are listed as "pending" entries
// in the BlockingGates output so an LLM/operator sees what's still
// being checked manually.
func (s *GocodeServiceImpl) ModuleStatus(ctx stdctx.Context, in api.ModuleStatusInput) (api.ModuleStatusOutput, error) {
	if in.ID == "" {
		return api.ModuleStatusOutput{}, fmt.Errorf("%w: ModuleStatus: id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ModuleStatusOutput{}, fmt.Errorf("ModuleStatus: resolve CWMService: %w", err)
	}

	inputs, err := projection.LoadManifestInputs(ctx, cwm, in.ID)
	if err != nil {
		return api.ModuleStatusOutput{}, fmt.Errorf("ModuleStatus: load inputs: %w", err)
	}

	out := api.ModuleStatusOutput{
		Module: inputs.Module,
	}

	// UC totals: 4-verdict projection over current UC items'
	// status_cache values. These reflect the most-recent verify
	// run (since UCSetStatus is the only writer).
	for _, uc := range inputs.UseCases {
		out.UCTotals.UCTotal++
		switch coverage.VerdictOf(uc.StatusCache) {
		case api.UCStatusPass:
			out.UCTotals.Pass++
		case api.UCStatusFail:
			out.UCTotals.Fail++
		case api.UCStatusFailDeferred:
			out.UCTotals.FailDeferred++
		case api.UCStatusSkipped:
			out.UCTotals.Skipped++
		default:
			out.UCTotals.NoTest++
		}
	}

	// Open ODs
	for _, od := range inputs.OpenDecisions {
		if od.State == api.ODStateOpen {
			out.OpenODCount++
		}
	}

	// Audit findings (latest bundle only — older bundles' findings
	// are historical context, not promote-gate inputs)
	for _, f := range inputs.OpenFindings {
		if f.State != api.FindingStateOpen {
			continue
		}
		switch f.Severity {
		case api.FindingSeverityCritical:
			out.AuditCritOpen++
		case api.FindingSeverityHigh:
			out.AuditHighOpen++
		}
	}

	// Run the v1 lint detectors inline. This is the same logic the
	// promote gate uses (see methods_promote.gateInputsFrom) — unify
	// so the operator never sees one verb disagree with another about
	// whether the module is clean.
	_, lintErr, _, _ := lint.Run(lint.Inputs{
		Module:        inputs.Module,
		UseCases:      inputs.UseCases,
		OpenDecisions: inputs.OpenDecisions,
	}, nil)

	// Derive module health (3-state projection) and stamp it on the
	// returned module. Health is computed, not stored — recomputed on
	// every ModuleStatus call to stay current with UC + audit drift.
	// See modules/code/concepts.md §4.3.
	out.Module.Health = modulehealth.Derive(modulehealth.Inputs{
		UCTotals:      out.UCTotals,
		AuditCritOpen: out.AuditCritOpen,
		AuditHighOpen: out.AuditHighOpen,
		LintBroken:    lintErr > 0,
		// BuildFailing stays false until the GoTool integration lands
		// (Phase F).
	})

	// Blocking gates — listed in evaluation order so an operator
	// can fix them top-down.
	if inputs.CharterVersion == "" || inputs.CharterVersion < "0.1" {
		out.BlockingGates = append(out.BlockingGates, "charter version < 0.1")
	}
	if out.UCTotals.Fail > 0 {
		out.BlockingGates = append(out.BlockingGates,
			fmt.Sprintf("%d UC failing (must pass or fail-defer)", out.UCTotals.Fail))
	}
	if out.UCTotals.NoTest > 0 {
		out.BlockingGates = append(out.BlockingGates,
			fmt.Sprintf("%d UC with no test", out.UCTotals.NoTest))
	}
	if out.AuditCritOpen > 0 {
		out.BlockingGates = append(out.BlockingGates,
			fmt.Sprintf("%d critical audit finding(s) open", out.AuditCritOpen))
	}
	if out.AuditHighOpen > 0 {
		out.BlockingGates = append(out.BlockingGates,
			fmt.Sprintf("%d high-severity audit finding(s) open", out.AuditHighOpen))
	}
	if lintErr > 0 {
		out.BlockingGates = append(out.BlockingGates,
			fmt.Sprintf("%d lint error(s) — run `gocode module-lint --id %s` for details", lintErr, inputs.Module.ID))
	}
	// Gate 5 (API contract drift) not yet checkable — surface as
	// pending so the operator sees what's still inspected manually.
	out.BlockingGates = append(out.BlockingGates,
		"API contract drift (pending: go-ast-discovery task)",
	)

	return out, nil
}
