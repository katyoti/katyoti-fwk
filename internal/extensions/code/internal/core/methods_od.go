// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ODOpen creates a decision item attached to a module node.
// ODs hold the rationale trail for fail-deferred ACs + any
// cross-cutting design question that needs an explicit answer before
// the next audit cycle. State starts at "open"; ODResolve flips to
// "resolved".
//
// Ported from gocode (Track B #4 phase 1) with one contract change:
// the OD-NNN id is ALLOCATED here (max existing + 1 at the module),
// not caller-supplied — the v0.2 input carries only Title + Proposal.
func (s *codeService) ODOpen(ctx stdctx.Context, in api.ODOpenInput) (api.ODOpenOutput, error) {
	if in.ModuleID == "" {
		return api.ODOpenOutput{}, fmt.Errorf("%w: ODOpen: module_id is required", api.ErrInvalidInput)
	}
	if in.Title == "" {
		return api.ODOpenOutput{}, fmt.Errorf("%w: ODOpen: title is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ODOpenOutput{}, fmt.Errorf("ODOpen: resolve CWMService: %w", err)
	}

	ods, err := listODs(ctx, cwm, in.ModuleID)
	if err != nil {
		return api.ODOpenOutput{}, fmt.Errorf("ODOpen: list existing ODs: %w", err)
	}
	odID := nextODID(ods)

	od := api.OpenDecision{
		NodeID: in.ModuleID,
		ODID:   odID,
		Title:  in.Title,
		Body:   in.Proposal,
		State:  api.ODStateOpen,
	}
	body, err := json.Marshal(odBody(od))
	if err != nil {
		return api.ODOpenOutput{}, fmt.Errorf("ODOpen: marshal body: %w", err)
	}
	odModule, err := cwmapi.ModuleOfObjectID(in.ModuleID)
	if err != nil {
		return api.ODOpenOutput{}, fmt.Errorf("ODOpen: owning module: %w", err)
	}
	// odID (OD-NNN) IS the authored identity — the method models an open
	// decision as a first-class object, so it gets a findable name.
	odObj := cwmapi.NewRecordObject(odModule, odID, "decision", in.ModuleID, in.Title, body)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: odModule, Object: odObj}); err != nil {
		return api.ODOpenOutput{}, fmt.Errorf("ODOpen: ObjectCreate: %w", err)
	}
	return api.ODOpenOutput{ODID: odID}, nil
}

// ODResolve flips an OD (addressed by its stable OD-NNN within the
// module) to state="resolved" with the supplied resolution summary.
// After resolve, any AC carrying `fail_deferred_by: OD-NNN` becomes
// vulnerable to status recomputation on the next verify run
// (deferral lifted).
func (s *codeService) ODResolve(ctx stdctx.Context, in api.ODResolveInput) (api.ODResolveOutput, error) {
	if in.ModuleID == "" {
		return api.ODResolveOutput{}, fmt.Errorf("%w: ODResolve: module_id is required", api.ErrInvalidInput)
	}
	if in.ODID == "" {
		return api.ODResolveOutput{}, fmt.Errorf("%w: ODResolve: od_id is required", api.ErrInvalidInput)
	}
	if in.Resolution == "" {
		return api.ODResolveOutput{}, fmt.Errorf("%w: ODResolve: resolution is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ODResolveOutput{}, fmt.Errorf("ODResolve: resolve CWMService: %w", err)
	}

	ods, err := listODs(ctx, cwm, in.ModuleID)
	if err != nil {
		return api.ODResolveOutput{}, fmt.Errorf("ODResolve: list ODs: %w", err)
	}
	var target *api.OpenDecision
	for i := range ods {
		if ods[i].ODID == in.ODID {
			target = &ods[i]
			break
		}
	}
	if target == nil {
		return api.ODResolveOutput{}, fmt.Errorf("%w: ODResolve: no decision %s at module %s",
			api.ErrInvalidInput, in.ODID, in.ModuleID)
	}

	now := time.Now().UTC()
	target.State = api.ODStateResolved
	target.Resolution = in.Resolution
	target.ResolvedAt = &now

	body, err := json.Marshal(odBody(*target))
	if err != nil {
		return api.ODResolveOutput{}, fmt.Errorf("ODResolve: marshal body: %w", err)
	}
	tgtModule, err := cwmapi.ModuleOfObjectID(target.ID)
	if err != nil {
		return api.ODResolveOutput{}, fmt.Errorf("ODResolve: owning module: %w", err)
	}
	cur, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: tgtModule, ID: target.ID})
	if err != nil {
		return api.ODResolveOutput{}, fmt.Errorf("ODResolve: ObjectGet: %w", err)
	}
	// Carry the object forward — ObjectUpdate replaces wholesale.
	updated := cur.Object
	updated.SetFieldsJSON(body)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: tgtModule, Object: updated, Revision: cur.Revision}); err != nil {
		return api.ODResolveOutput{}, fmt.Errorf("ODResolve: ObjectUpdate: %w", err)
	}
	return api.ODResolveOutput{ODID: in.ODID}, nil
}

// listODs returns the decoded decision items at a node.
func listODs(ctx stdctx.Context, cwm cwmapi.CWMService, nodeID string) ([]api.OpenDecision, error) {
	module, err := cwmapi.ModuleOfObjectID(nodeID)
	if err != nil {
		return nil, err
	}
	out, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Parent: nodeID, Type: "decision"})
	if err != nil {
		return nil, err
	}
	// The list's index copies carry no fields — the od_id nextODID numbers
	// from is one — so each decision is read whole.
	objs, err := cwmapi.WholeObjects(ctx, cwm, out.Objects)
	if err != nil {
		return nil, err
	}
	ods := make([]api.OpenDecision, 0, len(objs))
	for _, it := range objs {
		var od api.OpenDecision
		if raw := it.FieldsJSON(); len(raw) > 0 {
			_ = json.Unmarshal(raw, &od)
		}
		od.ID = it.ID
		od.NodeID = it.ParentID()
		od.Title = it.Name
		ods = append(ods, od)
	}
	return ods, nil
}

// nextODID allocates the next OD-NNN at a module: max existing + 1.
// Tombstoned/resolved ODs keep their number — IDs are never reused.
func nextODID(ods []api.OpenDecision) string {
	maxN := 0
	for _, od := range ods {
		var n int
		if _, err := fmt.Sscanf(od.ODID, "OD-%d", &n); err == nil && n > maxN {
			maxN = n
		}
	}
	return fmt.Sprintf("OD-%03d", maxN+1)
}

// odBody returns the JSON shape persisted as the item's Body —
// byte-compatible with the items gocode's v0 verbs wrote.
func odBody(od api.OpenDecision) map[string]any {
	m := map[string]any{
		"od_id": od.ODID,
		"state": od.State,
	}
	if od.Body != "" {
		m["body"] = od.Body
	}
	if od.Resolution != "" {
		m["resolution"] = od.Resolution
	}
	if od.BlockingTier != nil {
		m["blocking_tier"] = *od.BlockingTier
	}
	if od.ResolvedAt != nil {
		m["resolved_at"] = od.ResolvedAt.Format(time.RFC3339)
	}
	return m
}
