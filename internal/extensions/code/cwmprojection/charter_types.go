// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
)

// CharterInputs is the typed bundle the verify+publish path gathers
// before composing charter.md. Mirrors the ManifestInputs shape for
// the projected data + adds the authored prose fields (concepts_md,
// api_contract_md, version) from the charter item.
type CharterInputs struct {
	Module      gcapi.Module
	CharterItem *Charter // nil when no charter item exists yet
	UseCases    []gcapi.UseCase
	// UCFiles are the use cases with their criteria and the statuses of the
	// latest test-run verdicts: the Test plan section and every UC status the
	// charter shows are rendered from them (ManifestInputs.UCFiles).
	UCFiles       []*ucfile.UCFile
	OpenDecisions []gcapi.OpenDecision
	// LatestTestRun is the most recent verify result; nil when none.
	LatestTestRun *gcapi.TestRun
	// LatestAudit is the most recent audit review; nil when none.
	LatestAudit *gcapi.AuditBundle
	// OpenFindings is the un-closed audit findings on the latest bundle.
	OpenFindings []gcapi.AuditFinding
}

// Charter is the lightweight wire-shape for the `charter` item.
// Same fields as the YAML schema (modules/code/verification.md §3); kept here so
// the api/ package doesn't need to grow this Charter — it's a
// projection-internal concept.
type Charter struct {
	Version         string    `json:"version"`
	Status          string    `json:"status"`
	ConceptsMD      string    `json:"concepts_md"`
	APIContractMD   string    `json:"api_contract_md"`
	LastPublishedAt time.Time `json:"last_published_at"`
	LastPublishedTo string    `json:"last_published_to"`
}
