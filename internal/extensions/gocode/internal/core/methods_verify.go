// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/coverage"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/modulehealth"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// ModuleVerify is the verb behind `ktw-gocode module-verify`.
//
// For each use-case at the module: aggregates its `verifies`-linked
// test-cases via the TCID convention (L<n>-uc<NNN>-… → UC-NNN),
// runs the coverage state machine to derive a new status_cache,
// and writes both:
//
//   - one immutable `test-run` item recording the full verdict map
//   - per-UC `status_cache` field updates (via the typed body
//     re-encode + ItemUpdate)
//
// The status_cache writes are the ONLY legitimate path for that
// field per modules/code-gocode/concepts.md §4 — the lint detector flags every
// other writer as drift.
//
// v1 limitation: does NOT shell out to `go test` / `tasktest run`
// to populate test-case `last_run_status` fields. Reads the
// existing values and aggregates. Future work bundles
// GoToolService.GoTest into this verb's prologue.
func (s *GocodeServiceImpl) ModuleVerify(ctx stdctx.Context, in api.ModuleVerifyInput) (api.ModuleVerifyOutput, error) {
	if in.ID == "" {
		return api.ModuleVerifyOutput{}, fmt.Errorf("%w: ModuleVerify: id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ModuleVerifyOutput{}, fmt.Errorf("ModuleVerify: resolve CWMService: %w", err)
	}

	// Reuse the projection loader to gather everything in one pass.
	inputs, err := projection.LoadManifestInputs(ctx, cwm, in.ID)
	if err != nil {
		return api.ModuleVerifyOutput{}, fmt.Errorf("ModuleVerify: load inputs: %w", err)
	}

	// Test-cases aren't part of the manifest-projection inputs; fetch them.
	testCases, err := loadTestCases(ctx, cwm, in.ID)
	if err != nil {
		return api.ModuleVerifyOutput{}, fmt.Errorf("ModuleVerify: load test-cases: %w", err)
	}

	// Build the OD-by-id index for fail-deferred resolution.
	openODByID := map[string]api.OpenDecision{}
	for _, od := range inputs.OpenDecisions {
		if od.State == api.ODStateOpen {
			openODByID[od.ODID] = od
		}
	}

	now := time.Now().UTC()
	agg := coverage.Aggregate(coverage.AggregateInputs{
		Module:         inputs.Module,
		UseCases:       inputs.UseCases,
		TestCases:      testCases,
		OpenODByID:     openODByID,
		LayerFilter:    in.LayerFilter,
		UCFilter:       in.UCFilter,
		SeverityFilter: in.Severity,
		RanAt:          now,
	})

	// Map UC-NNN back to the substrate item UUID so we know which
	// item to ItemUpdate. The Aggregate output is keyed by UC-NNN
	// (the human id); status_cache writes target the item UUID.
	itemUUIDByUCID := map[string]string{}
	for _, uc := range inputs.UseCases {
		if normalized, ok := coverage.NormalizeUCID(uc.UCID); ok {
			itemUUIDByUCID[normalized] = uc.ID
		}
	}

	// 1. Write the test-run item first; we'll stamp each UC's
	//    last_run_id with the new item's UUID.
	testRunBody := map[string]any{
		"module":         inputs.Module.Name,
		"ran_at":         now.Format(time.RFC3339),
		"layer_filter":   defaultStr(in.LayerFilter, "all"),
		"uc_filter":      in.UCFilter,
		"totals":         agg.Totals,
		"tier_n_dod_met": tierNDoDMet(agg.Totals),
		"verdicts":       agg.TestRunVerdicts,
	}
	testRunBodyBytes, err := json.Marshal(testRunBody)
	if err != nil {
		return api.ModuleVerifyOutput{}, fmt.Errorf("ModuleVerify: encode test-run body: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(in.ID)
	if err != nil {
		return api.ModuleVerifyOutput{}, fmt.Errorf("ModuleVerify: owning module: %w", err)
	}
	// A run's authored id is its instant — runs are append-only and one per
	// verify, so the timestamp is both unique and meaningful in a filename.
	runLocal := "run-" + now.UTC().Format("20060102T150405Z")
	runObj := cwmapi.NewRecordObject(module, runLocal, "test-run", in.ID,
		fmt.Sprintf("verify %s", now.Format(time.RFC3339)), testRunBodyBytes)
	createOut, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: runObj})
	if err != nil {
		return api.ModuleVerifyOutput{}, fmt.Errorf("ModuleVerify: create test-run: %w", err)
	}
	testRunID := createOut.Object.ID

	// 2. Update each UC's status_cache (only when it changed OR
	//    last_verified_at needs refreshing). Calling UCSetStatus
	//    internally so the single write-path stays consistent.
	for ucID, verdict := range agg.PerUC {
		itemUUID, ok := itemUUIDByUCID[ucID]
		if !ok {
			continue // shouldn't happen given the build, but defensive
		}
		if verdict.Skipped {
			continue // skipped UCs don't get status_cache touched
		}
		_, err := s.UCSetStatus(ctx, api.UCSetStatusInput{
			ID:             itemUUID,
			Status:         verdict.NewStatus,
			FailDeferredBy: failDeferredBy(verdict, inputs.UseCases, itemUUID),
			TestRunID:      testRunID,
			LastVerifiedAt: now,
		})
		if err != nil {
			return api.ModuleVerifyOutput{}, fmt.Errorf("ModuleVerify: UCSetStatus(%s): %w", ucID, err)
		}
	}

	// 3. Compose the output. Blockers are populated when the run
	//    fails to meet tier-N DoD; carries the failure reasons for
	//    the operator + the promote-gate to consume.
	blockers := []string{}
	if !tierNDoDMet(agg.Totals) {
		for ucID, v := range agg.PerUC {
			if v.Verdict == api.UCStatusFail {
				if v.Reason != "" {
					blockers = append(blockers, fmt.Sprintf("%s: %s", ucID, v.Reason))
				} else {
					blockers = append(blockers, fmt.Sprintf("%s: failed", ucID))
				}
			}
			if v.Verdict == api.UCStatusNoTest {
				blockers = append(blockers, fmt.Sprintf("%s: no test", ucID))
			}
		}
	}

	// Derive 3-state module health from the just-computed totals +
	// open critical/high audit findings on the latest audit review.
	// Caller observes module BROKEN immediately after a regressing
	// verify run — this is the precondition the REGRESSED cascade
	// will key off when it lands (modules/code/concepts.md §4.3).
	var auditCrit, auditHigh int
	for _, f := range inputs.OpenFindings {
		if f.State != api.FindingStateOpen {
			continue
		}
		switch f.Severity {
		case api.FindingSeverityCritical:
			auditCrit++
		case api.FindingSeverityHigh:
			auditHigh++
		}
	}
	health := modulehealth.Derive(modulehealth.Inputs{
		UCTotals:      agg.Totals,
		AuditCritOpen: auditCrit,
		AuditHighOpen: auditHigh,
	})

	return api.ModuleVerifyOutput{
		TestRunID:   testRunID,
		Totals:      agg.Totals,
		TierNDoDMet: tierNDoDMet(agg.Totals),
		Health:      health,
		Blockers:    blockers,
	}, nil
}

// UCSetStatus is the only legitimate writer of use-case.status_cache.
// Re-encodes the typed body with the new status + verification
// timestamp + test-run pointer, then calls cwm.ItemUpdate.
//
// Lint detector StatusCacheDrift flags any item update that mutates
// status_cache outside this verb (planned with the lint-detectors
// task).
func (s *GocodeServiceImpl) UCSetStatus(ctx stdctx.Context, in api.UCSetStatusInput) (api.UCSetStatusOutput, error) {
	if in.ID == "" {
		return api.UCSetStatusOutput{}, fmt.Errorf("%w: UCSetStatus: id is required", api.ErrInvalidInput)
	}
	if in.Status == "" {
		return api.UCSetStatusOutput{}, fmt.Errorf("%w: UCSetStatus: status is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.UCSetStatusOutput{}, fmt.Errorf("UCSetStatus: resolve CWMService: %w", err)
	}

	// Read current item to preserve other typed fields.
	ucModule, err := cwmapi.ModuleOfObjectID(in.ID)
	if err != nil {
		return api.UCSetStatusOutput{}, fmt.Errorf("UCSetStatus: owning module: %w", err)
	}
	getOut, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: ucModule, ID: in.ID})
	if err != nil {
		return api.UCSetStatusOutput{}, fmt.Errorf("UCSetStatus: get UC %s: %w", in.ID, err)
	}
	if getOut.Object.Type != "use-case" {
		return api.UCSetStatusOutput{}, fmt.Errorf("%w: UCSetStatus: object %s is type %q, want use-case",
			api.ErrInvalidInput, in.ID, getOut.Object.Type)
	}
	ucBody := getOut.Object.FieldsJSON()

	// Decode existing body so we keep id/severity/title/layer_targets fields intact.
	var body map[string]any
	if len(ucBody) > 0 {
		if err := json.Unmarshal(ucBody, &body); err != nil {
			return api.UCSetStatusOutput{}, fmt.Errorf("UCSetStatus: decode existing body: %w", err)
		}
	}
	if body == nil {
		body = map[string]any{}
	}

	body["status_cache"] = string(in.Status)
	verifiedAt := in.LastVerifiedAt
	if verifiedAt.IsZero() {
		verifiedAt = time.Now().UTC()
	}
	body["last_verified_at"] = verifiedAt.Format(time.RFC3339)
	if in.TestRunID != "" {
		body["last_run_id"] = in.TestRunID
	}
	// The deferring decision is an edge, sticky while fail-deferred, removed
	// once the status is no longer fail-deferred.
	delete(body, "fail_deferred_by")
	deferredBy := getOut.Object.EdgeTo(api.EdgeDeferredBy)
	if in.Status != api.UCStatusFailDeferred {
		deferredBy = ""
	} else if in.FailDeferredBy != "" {
		deferredBy = cwmapi.QualifyObjectID(ucModule, in.FailDeferredBy)
	}

	newBody, err := json.Marshal(body)
	if err != nil {
		return api.UCSetStatusOutput{}, fmt.Errorf("UCSetStatus: re-encode body: %w", err)
	}

	// Carry the object forward and patch only content, so any capability this
	// verb does not know about survives the write.
	updated := getOut.Object
	updated.SetFieldsJSON(newBody)
	updated.SetEdgeTo(api.EdgeDeferredBy, deferredBy)
	updOut, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: ucModule, Object: updated, Revision: getOut.Revision})
	if err != nil {
		return api.UCSetStatusOutput{}, fmt.Errorf("UCSetStatus: ObjectUpdate: %w", err)
	}

	// Decode back into the typed UseCase for the response.
	uc, _ := decodeUseCaseFromObject(updOut.Object)
	return api.UCSetStatusOutput{UseCase: uc}, nil
}

// --- helpers ---------------------------------------------------------------

func loadTestCases(ctx stdctx.Context, cwm cwmapi.CWMService, moduleID string) ([]api.TestCase, error) {
	module, err := cwmapi.ModuleOfObjectID(moduleID)
	if err != nil {
		return nil, err
	}
	// Filter by type in the query rather than in the loop — the index can do it.
	out, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Parent: moduleID, Type: "test-case"})
	if err != nil {
		return nil, err
	}
	// The list's index copies carry no fields; each test case is read whole.
	objs, err := cwmapi.WholeObjects(ctx, cwm, out.Objects)
	if err != nil {
		return nil, err
	}
	var testCases []api.TestCase
	for i := range objs {
		o := objs[i]
		tc, err := decodeTestCaseFromObject(o)
		if err != nil {
			return nil, fmt.Errorf("decode test-case %s: %w", o.ID, err)
		}
		testCases = append(testCases, tc)
	}
	return testCases, nil
}

// decodeTestCaseFromObject maps one object onto the typed TestCase. This is the
// adapter seam: the Object shape is read HERE and nowhere else, so a capability
// rename is one edit rather than one per call site.
func decodeTestCaseFromObject(o cwmapi.Object) (api.TestCase, error) {
	content := o
	tc := api.TestCase{
		ID:     o.ID,
		NodeID: o.ParentID(),
		Title:  content.Name,
	}
	if _, local, err := cwmapi.SplitObjectID(o.ID); err == nil {
		tc.TCID = local // the authored id IS the human id now
	}
	if len(content.FieldsJSON()) == 0 {
		return tc, nil
	}
	var body struct {
		TCID          string            `json:"id"`
		Layer         api.UCLayer       `json:"layer"`
		TestRef       string            `json:"test_ref"`
		LastRunStatus api.TestRunStatus `json:"last_run_status"`
		LastRunID     string            `json:"last_run_id"`
		Negative      bool              `json:"negative"`
	}
	if err := json.Unmarshal(content.FieldsJSON(), &body); err != nil {
		return tc, err
	}
	if tc.TCID == "" {
		tc.TCID = body.TCID
	}
	tc.Layer = body.Layer
	tc.TestRef = body.TestRef
	tc.LastRunStatus = body.LastRunStatus
	tc.LastRunID = body.LastRunID
	tc.Negative = body.Negative
	return tc, nil
}

func decodeUseCaseFromObject(o cwmapi.Object) (api.UseCase, error) {
	content := o
	uc := api.UseCase{
		ID:     o.ID,
		NodeID: o.ParentID(),
		Title:  content.Name,
	}
	if _, local, err := cwmapi.SplitObjectID(o.ID); err == nil {
		uc.UCID = local // identity collapse: <module>.UC-NNN
	}
	uc.Severity = severityOf(o)
	if len(content.FieldsJSON()) == 0 {
		return uc, nil
	}
	var body struct {
		StatusCache    api.UCStatus `json:"status_cache"`
		LastVerifiedAt string       `json:"last_verified_at"`
		LastRunID      string       `json:"last_run_id"`
	}
	if err := json.Unmarshal(content.FieldsJSON(), &body); err != nil {
		return uc, err
	}
	uc.StatusCache = body.StatusCache
	uc.FailDeferredBy = api.DeferredByLocal(o.EdgeTo(api.EdgeDeferredBy))
	if t, err := time.Parse(time.RFC3339, body.LastVerifiedAt); err == nil {
		uc.LastVerifiedAt = &t
	}
	return uc, nil
}

// tierNDoDMet returns true iff every counted UC is pass OR
// fail-deferred. The tier-promote gate uses this as condition #2.
func tierNDoDMet(t api.TestRunTotals) bool {
	return t.Fail == 0 && t.NoTest == 0
}

// failDeferredBy looks up the original UC item's fail_deferred_by
// field by UUID — needed because UCSetStatus accepts FailDeferredBy
// as an explicit Input parameter, and the aggregator's verdict
// doesn't carry it.
func failDeferredBy(verdict coverage.UCVerdict, ucs []api.UseCase, itemUUID string) string {
	if verdict.NewStatus != api.UCStatusFailDeferred {
		return ""
	}
	for _, uc := range ucs {
		if uc.ID == itemUUID {
			return uc.FailDeferredBy
		}
	}
	return ""
}

func defaultStr(s, dflt string) string {
	if s == "" {
		return dflt
	}
	return s
}
