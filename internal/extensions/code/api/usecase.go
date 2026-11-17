// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
	"time"
)

// UCSeverity classifies a use-case's criticality. Concepts.md §6.4.
type UCSeverity string

const (
	UCSeverityCore     UCSeverity = "core"     // module is broken without it
	UCSeveritySecurity UCSeverity = "security" // safety guarantee; negative DoD required
	UCSeverityQoL      UCSeverity = "qol"      // quality-of-life
	UCSeverityCosmetic UCSeverity = "cosmetic" // polish
)

// UCStatus is the 9-state coverage state plus the projected 4-verdict
// view. Internal states (InDev / Covered / Validated / Regressed)
// project to user-facing verdicts Pass / Fail / etc. Concepts.md §9.
type UCStatus string

const (
	// Projected verdicts (what `verify --format=text` prints)
	UCStatusPass         UCStatus = "pass"
	UCStatusFail         UCStatus = "fail"
	UCStatusNoTest       UCStatus = "no-test"
	UCStatusFailDeferred UCStatus = "fail-deferred"
	UCStatusSkipped      UCStatus = "skipped"

	// Internal states (status_cache writer's view)
	UCStatusInDev     UCStatus = "in-dev"
	UCStatusCovered   UCStatus = "covered"
	UCStatusValidated UCStatus = "validated"
	UCStatusRegressed UCStatus = "regressed"
)

// UCLayer is a test layer the UC expects to be exercised by.
type UCLayer string

const (
	UCLayerL1 UCLayer = "L1" // Go unit
	UCLayerL2 UCLayer = "L2" // .tasktest simulator
	UCLayerL3 UCLayer = "L3" // real-LLM end-to-end
)

// UseCase is the on-the-wire shape of a `use-case` item. Mirrors
// app/ktw-agents/embed/cwm/item_types/use-case.yaml.
type UseCase struct {
	// Item-substrate fields
	ID     string `json:"id"                       jsonschema:"Item UUID — distinct from the human-readable UC-NNN field"`
	NodeID string `json:"node_id"                  jsonschema:"Owning module-node UUID"`
	Title  string `json:"title"                    jsonschema:"One-line specification"`
	Body   string `json:"body,omitempty"           jsonschema:"DoD-per-UC markdown (layer-tagged test refs + negative DoD)"`

	// Typed body fields (use-case schema)
	UCID           string     `json:"uc_id"                       jsonschema:"Stable, gap-allowed identifier (UC-001, UC-053)"`
	Severity       UCSeverity `json:"severity"                    jsonschema:"core | security | qol | cosmetic"`
	StatusCache    UCStatus   `json:"status_cache,omitempty"      jsonschema:"Last verdict written by verify verb; honor-system protected"`
	LastVerifiedAt *time.Time `json:"last_verified_at,omitempty"  jsonschema:"When verify last wrote status_cache"`
	FailDeferredBy string     `json:"fail_deferred_by,omitempty"  jsonschema:"OD-id when status_cache = fail-deferred"`
	LayerTargets   []UCLayer  `json:"layer_targets,omitempty"     jsonschema:"Test layers expected to exercise this UC"`
	Tags           []string   `json:"tags,omitempty"`
}

// EdgeDeferredBy is the relation from a fail-deferred use case to the open
// decision that justifies the deferral (ktw-methods' `deferred-by` edge kind).
// A use case stores it as an edge, never as a field; UseCase.FailDeferredBy
// is its target's local id (OD-004).
const EdgeDeferredBy = "deferred-by"

// DeferredByLocal is the local id of a `deferred-by` target: “
// reads `OD-004`, the form FailDeferredBy carries.
func DeferredByLocal(target string) string {
	if i := strings.LastIndex(target, "."); i >= 0 {
		return target[i+1:]
	}
	return target
}
