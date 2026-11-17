// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/tierpromotion"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/lint"
)

// ModulePromote evaluates the 5-condition promote gate
// (modules/code-gocode/concepts.md §6) and, when it passes, advances `tier_current` by 1 + writes
// the new `tier_state=promoted` field.
//
// On gate failure: returns a non-nil error + Blockers list (empty
// success-only fields). Caller renders Blockers verbatim to the
// operator.
//
// On `--force`: bypasses gate enforcement, records the bypass as an
// `forced-promotion` decision that applies on the module, and proceeds
// with the tier flip. The decision is the bypass trail the next
// audit review picks up.
func (s *GocodeServiceImpl) ModulePromote(ctx stdctx.Context, in api.ModulePromoteInput) (api.ModulePromoteOutput, error) {
	if in.ID == "" {
		return api.ModulePromoteOutput{}, fmt.Errorf("%w: ModulePromote: id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ModulePromoteOutput{}, fmt.Errorf("ModulePromote: resolve CWMService: %w", err)
	}

	inputs, err := projection.LoadManifestInputs(ctx, cwm, in.ID)
	if err != nil {
		return api.ModulePromoteOutput{}, fmt.Errorf("ModulePromote: load inputs: %w", err)
	}

	// Default target = module.tier_target.
	targetTier := in.TargetTier
	if targetTier <= 0 {
		targetTier = inputs.Module.TierTarget
	}
	if targetTier <= inputs.Module.TierCurrent {
		return api.ModulePromoteOutput{}, fmt.Errorf(
			"%w: ModulePromote: target tier %d not above current %d",
			api.ErrInvalidInput, targetTier, inputs.Module.TierCurrent)
	}

	// Compute UC + audit counters (same logic as ModuleStatus).
	gates := tierpromotion.EvaluateGates(gateInputsFrom(inputs))

	if !gates.Passes && !in.Force {
		return api.ModulePromoteOutput{
			Blockers: gates.Blockers,
		}, fmt.Errorf("ModulePromote: gate refused (%d blocker(s); pass --force to override)", len(gates.Blockers))
	}

	// Forced path — record the bypass as an answered decision for the
	// next audit review to pick up.
	if !gates.Passes && in.Force {
		if err := writeForcedPromotionAudit(ctx, cwm, inputs.Module, gates.Blockers); err != nil {
			return api.ModulePromoteOutput{}, fmt.Errorf("ModulePromote: write forced-promotion log: %w", err)
		}
	}

	// Flip tier_current + tier_state. ObjectUpdate replaces wholesale, so
	// patchCapability carries every other capability forward.
	updated, err := patchCapability(ctx, cwm, in.ID, codeCapability, map[string]any{
		"tier_current": targetTier,
		"tier_state":   string(api.TierStatePromoted),
	})
	if err != nil {
		return api.ModulePromoteOutput{}, fmt.Errorf("ModulePromote: patch tier: %w", err)
	}
	_ = updated

	return api.ModulePromoteOutput{
		NewTier:  targetTier,
		Forced:   in.Force && !gates.Passes,
		GatesMet: gates.GatesMet,
		Blockers: nil,
	}, nil
}

// ModulePlanTier v0: marks the module as `planning` (tier_state) and
// stamps the requested target tier. Task-filing (the "compute tier-N
// gaps, file todos" half) is deferred — a stub that bumps the FSM
// state is enough to unblock the verify/promote chain for v1
// acceptance. The gap-analysis half lands in a follow-up.
func (s *GocodeServiceImpl) ModulePlanTier(ctx stdctx.Context, in api.ModulePlanTierInput) (api.ModulePlanTierOutput, error) {
	if in.ID == "" {
		return api.ModulePlanTierOutput{}, fmt.Errorf("%w: ModulePlanTier: id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ModulePlanTierOutput{}, fmt.Errorf("ModulePlanTier: resolve CWMService: %w", err)
	}

	// Pull current module so we know its current tier (for output).
	planModule, err := cwmapi.ModuleOfObjectID(in.ID)
	if err != nil {
		return api.ModulePlanTierOutput{}, fmt.Errorf("ModulePlanTier: owning module: %w", err)
	}
	getOut, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: planModule, ID: in.ID})
	if err != nil {
		return api.ModulePlanTierOutput{}, fmt.Errorf("ModulePlanTier: ObjectGet: %w", err)
	}
	if ok, err := projection.IsModule(ctx, cwm, in.ID); err != nil {
		return api.ModulePlanTierOutput{}, fmt.Errorf("ModulePlanTier: %w", err)
	} else if !ok {
		return api.ModulePlanTierOutput{}, fmt.Errorf(
			"%w: ModulePlanTier: object %s (type %q) is not a module",
			api.ErrInvalidInput, in.ID, getOut.Object.Type)
	}

	target := in.TargetTier
	if target <= 0 {
		target = moduleAttrInt(getOut.Object, "tier_current") + 1
	}

	if _, err := patchCapability(ctx, cwm, in.ID, codeCapability, map[string]any{
		"tier_target": target,
		"tier_state":  string(api.TierStatePlanning),
	}); err != nil {
		return api.ModulePlanTierOutput{}, fmt.Errorf("ModulePlanTier: patch tier: %w", err)
	}

	return api.ModulePlanTierOutput{
		GapSummary: fmt.Sprintf(
			"v0: stamped tier_target=%d + tier_state=planning. Gap-analysis + task-filing deferred to a follow-up task.",
			target),
	}, nil
}

// gateInputsFrom collects the typed bundle EvaluateGates expects from
// the projection inputs. Mirrors ModuleStatus's gate computation —
// pulled into a helper so the same inputs feed both the status read
// and the promote write.
//
// Also runs the lint v1 detector set inline so `LintBroken` reflects
// the real `severity: error` count. This unifies the promote-gate's
// gate #4 with the standalone `gocode module-lint` verb — both surface
// the same answer.
func gateInputsFrom(inputs projection.ManifestInputs) tierpromotion.GateInputs {
	gi := tierpromotion.GateInputs{
		TierCurrent:    inputs.Module.TierCurrent,
		TierTarget:     inputs.Module.TierTarget,
		CharterVersion: inputs.CharterVersion,
	}
	for _, uc := range inputs.UseCases {
		gi.UCTotal++
		// Use the same coverage projection ModuleStatus uses.
		switch verdictOf(uc.StatusCache) {
		case api.UCStatusFail:
			gi.UCTotalFail++
		case api.UCStatusNoTest:
			gi.UCTotalNoTest++
		}
	}
	for _, f := range inputs.OpenFindings {
		if f.State != api.FindingStateOpen {
			continue
		}
		switch f.Severity {
		case api.FindingSeverityCritical:
			gi.AuditCritOpen++
		case api.FindingSeverityHigh:
			gi.AuditHighOpen++
		}
	}

	// Run lint inline so gate #4 reflects the same answer as
	// `gocode module-lint`. Drift detectors stay false here (drift
	// detection needs filesystem access wired through the lint verb
	// itself — when that lands the lint output already feeds back
	// through this same path).
	_, lintErrCount, _, _ := lint.Run(lint.Inputs{
		Module:        inputs.Module,
		UseCases:      inputs.UseCases,
		OpenDecisions: inputs.OpenDecisions,
	}, nil)
	gi.LintBroken = lintErrCount > 0

	return gi
}

// verdictOf collapses the internal 9-state UCStatus into the 4-verdict
// projection. Mirrors coverage.VerdictOf — duplicated here to keep the
// promote-evaluation path free of the coverage package's other
// concerns (state-machine transitions, fail-defer reasoning, etc.).
func verdictOf(s api.UCStatus) api.UCStatus {
	switch s {
	case api.UCStatusValidated, api.UCStatusCovered, api.UCStatusPass:
		return api.UCStatusPass
	case api.UCStatusInDev, api.UCStatusRegressed, api.UCStatusFail:
		return api.UCStatusFail
	case api.UCStatusFailDeferred:
		return api.UCStatusFailDeferred
	case api.UCStatusSkipped:
		return api.UCStatusSkipped
	default:
		return api.UCStatusNoTest
	}
}

// writeForcedPromotionAudit records a forced-promotion bypass as a decision that
// APPLIES on the module: the question (promote with gates unmet?), the
// choice (force), the gates bypassed, and who chose it in its lifecycle
// history. A forced promotion is the method's signed, visible waiver, so it is
// a decision like any other — found where the module's decisions are, never
// re-derived. The next audit review picks it up as a critical input.
func writeForcedPromotionAudit(ctx stdctx.Context, cwm cwmapi.CWMService, mod api.Module, blockers []string) error {
	module, err := cwmapi.ModuleOfObjectID(mod.ID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	// The waiver's id records what it waived and when, so it is findable.
	local := fmt.Sprintf("forced-promotion-tier-%d-%s", mod.TierTarget, now.Format("20060102T150405Z"))
	obj := cwmapi.Object{
		ID: cwmapi.QualifyObjectID(module, local), Type: decisionsapi.DecisionType, Parent: mod.ID,
		Name: fmt.Sprintf("Promote to tier %d with %d gate(s) unmet?", mod.TierTarget, len(blockers)),
	}
	var b strings.Builder
	b.WriteString("## Question\n\nThe promote gate refused; the operator passed --force. Gates unmet:\n\n")
	for _, g := range blockers {
		fmt.Fprintf(&b, "- %s\n", g)
	}
	b.WriteString("\n## Decision\n\nforce. The operator bypassed the promote gate; the tier was raised anyway.\n")
	obj.Body = b.String()
	if err := obj.SetCapability(decisionsapi.CapDecision, map[string]any{
		"options": []string{"force"}, "selected": []string{"force"},
		"raised": today, "resolved": today, "raised_by": "promote --force", "source": "ktw-gocode:module/promote",
	}); err != nil {
		return err
	}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: obj}); err != nil {
		return err
	}
	_, err = cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{
		ID: obj.ID, To: decisionsapi.StatusApplies, Note: "promote --force",
		By: cwmapi.Actor{ID: "ktw-gocode", Kind: cwmapi.ActorAgent},
	})
	return err
}
