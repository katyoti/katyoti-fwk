// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	stdctx "context"
	"encoding/json"
	"fmt"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// LoadCharterInputs is the charter-publish counterpart to
// LoadManifestInputs. Reuses LoadManifestInputs internally for the
// shared bundle (module + UCs + ODs + audit) and additionally
// decodes the `charter` item's authored prose.
func LoadCharterInputs(ctx stdctx.Context, cwm cwmapi.CWMService, moduleID string) (CharterInputs, error) {
	mIn, err := LoadManifestInputs(ctx, cwm, moduleID)
	if err != nil {
		return CharterInputs{}, err
	}

	out := CharterInputs{
		Module:        mIn.Module,
		UseCases:      mIn.UseCases,
		UCFiles:       mIn.UCFiles,
		OpenDecisions: mIn.OpenDecisions,
		LatestTestRun: mIn.LatestTestRun,
		LatestAudit:   mIn.LatestAudit,
		OpenFindings:  mIn.OpenFindings,
	}

	// Charter item — read separately. The manifest loader records
	// CharterVersion but discards the rest of the item; we need the
	// authored prose fields here.
	itemsOut, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: moduleID, Type: "charter"})
	if err != nil {
		return CharterInputs{}, fmt.Errorf("list charter objects: %w", err)
	}
	charters, err := cwmapi.WholeObjects(ctx, cwm, itemsOut.Objects) // the prose is in the fields
	if err != nil {
		return CharterInputs{}, err
	}
	for i := range charters {
		it := charters[i]
		ch, err := decodeCharter(it)
		if err != nil {
			return CharterInputs{}, fmt.Errorf("decode charter %s: %w", it.ID, err)
		}
		// Pick the highest version (matches LoadManifestInputs's pattern).
		if out.CharterItem == nil || ch.Version > out.CharterItem.Version {
			out.CharterItem = &ch
		}
	}

	return out, nil
}

func decodeCharter(o cwmapi.Object) (Charter, error) {
	p := partsOf(o)
	c := Charter{}
	if len(p.Fields) == 0 {
		return c, nil
	}
	var body struct {
		Version         string `json:"version"`
		Status          string `json:"status"`
		ConceptsMD      string `json:"concepts_md"`
		APIContractMD   string `json:"api_contract_md"`
		LastPublishedAt string `json:"last_published_at"`
		LastPublishedTo string `json:"last_published_to"`
	}
	if err := json.Unmarshal(p.Fields, &body); err != nil {
		return c, err
	}
	c.Version = body.Version
	c.Status = body.Status
	c.ConceptsMD = body.ConceptsMD
	c.APIContractMD = body.APIContractMD
	if t, err := parseRFC3339(body.LastPublishedAt); err == nil {
		c.LastPublishedAt = t
	}
	c.LastPublishedTo = body.LastPublishedTo
	return c, nil
}
