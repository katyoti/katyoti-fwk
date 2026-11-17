// SPDX-License-Identifier: Apache-2.0

package api

import cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

// Deliverables (modules/doc-engine/): a document written for someone - an
// architecture study, a flow matrix, a technology watch - is a `document`
// object carrying the `deliverable` capability: what wrote it, for whom, in
// which language and format, and where it stands in its own workflow
// (draft, in review, validated, published). Its files are attached to it
// (`attaches`), shown inline in its Files tab; the objects it was written
// from are its `produced-from` edges, so it can be written again from them.

const (
	// CapabilityDeliverable is the capability a document carries when it is
	// written for someone; its group is CapDeliverable.
	CapabilityDeliverable = "deliverable"
	// WorkflowDeliverable is the workflow the capability's state follows.
	WorkflowDeliverable = "deliverable"
	// EdgeProducedFrom links a deliverable to an object it was written from.
	EdgeProducedFrom = "produced-from"
)

// CapDeliverable is the `deliverable` group on a document.
type CapDeliverable struct {
	Template string `json:"template"           jsonschema:"title=Template,description=What wrote it: a content template or a document type such as etude-architecture"`
	Audience string `json:"audience,omitempty" jsonschema:"title=Audience,description=Who it is written for such as an architecture board"`
	Language string `json:"language,omitempty" jsonschema:"title=Language,description=The language it is written in such as fr"`
	Format   string `json:"format,omitempty"   jsonschema:"title=Format,description=The form of its main file,enum=markdown|confluence|drawio|csv|word|pdf|html"`
	State    string `json:"state,omitempty"    jsonschema:"title=State,description=Where it stands: written then reviewed then validated by a person then published,enum=draft|in-review|validated|published"`
	// Trail is its moves - who submitted, validated or published it and when
	// ; written by the transition only.
	Trail []cwmapi.LifecycleEntry `json:"trail,omitempty" jsonschema:"title=Trail,description=Its moves: who moved it and when; written by the transition"`
}
