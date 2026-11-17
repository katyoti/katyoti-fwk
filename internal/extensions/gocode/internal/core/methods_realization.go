// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/realization"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// FeatureRealizationStatus is the souhait-side answer to "is feature F
// delivered?" — walks the feature's outgoing `wants` edges to its Use Cases,
// reads each UC's `status_cache`, derives a per-UC status, and returns the
// per-UC breakdown + the US-level worst-of aggregate (method-core §1/§3: a US
// is delivered when its wanted UCs pass their effective gate).
//
// Compute-on-read, no caching. Returns `not-started` when the US `wants` no
// UC ("no delivered behavior claimed yet"). The UC's own status is the leaf
// signal — there is no module tier-state hop (each UC `lives-in` its Module
// structurally, which is where its status was written by `verify`).
func (s *GocodeServiceImpl) FeatureRealizationStatus(ctx stdctx.Context, in api.FeatureRealizationStatusInput) (api.FeatureRealizationStatusOutput, error) {
	if in.FeatureID == "" {
		return api.FeatureRealizationStatusOutput{}, fmt.Errorf("%w: FeatureRealizationStatus: feature_id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.FeatureRealizationStatusOutput{}, fmt.Errorf("FeatureRealizationStatus: resolve CWMService: %w", err)
	}

	// List the US's outgoing `wants` edges (US node → Use Case items).
	edgesOut, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{
		ObjectID:  in.FeatureID,
		Kind:      cwmapi.EdgeType("wants"),
		Direction: cwmapi.DirectionOut,
	})
	if err != nil {
		return api.FeatureRealizationStatusOutput{}, fmt.Errorf("FeatureRealizationStatus: EdgeList: %w", err)
	}

	out := api.FeatureRealizationStatusOutput{FeatureID: in.FeatureID}
	statuses := make([]realization.Status, 0, len(edgesOut.Edges))

	for _, e := range edgesOut.Edges {
		ucTitle, statusCache := ucStatusOf(ctx, cwm, e.To)
		st := realization.DeriveUCStatus(statusCache)
		out.PerUC = append(out.PerUC, api.RealizingUCStatus{
			UCID:    e.To,
			UCTitle: ucTitle,
			Status:  string(st),
		})
		statuses = append(statuses, st)
	}

	out.Aggregate = string(realization.AggregateFeatureStatus(statuses))
	return out, nil
}

// ucStatusOf reads a wanted Use Case item's title + `status_cache`. On any
// read/decode error it returns an empty status — DeriveUCStatus maps that to
// `not-started`, so the aggregate degrades gracefully for that UC.
func ucStatusOf(ctx stdctx.Context, cwm cwmapi.CWMService, ucID string) (string, string) {
	module, err := cwmapi.ModuleOfObjectID(ucID)
	if err != nil {
		return "", ""
	}
	getOut, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: ucID})
	if err != nil {
		return "", ""
	}
	var body struct {
		StatusCache string `json:"status_cache"`
	}
	if raw := getOut.Object.FieldsJSON(); len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	return getOut.Object.Name, body.StatusCache
}
