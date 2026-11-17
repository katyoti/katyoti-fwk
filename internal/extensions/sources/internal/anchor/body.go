// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	"encoding/json"
	"fmt"
	"time"
)

// SourceDocBody is the typed view of a source-document object's
// content body. Mirrors the schema of the source-document type; the tags are
// JSON because body travels as JSON.
type SourceDocBody struct {
	SourceURI           string    `json:"source_uri"`
	SectionPath         []string  `json:"section_path,omitempty"`
	SectionAnchorLegacy string    `json:"section_anchor_legacy,omitempty"`
	ContentHash         string    `json:"content_hash"`
	ContentSnapshot     string    `json:"content_snapshot,omitempty"`
	IngestedAt          time.Time `json:"ingested_at,omitempty"`
	IngestedBy          string    `json:"ingested_by,omitempty"`
	AnchorStatus        string    `json:"anchor_status,omitempty"`
	LastReanchorAt      time.Time `json:"last_reanchor_at,omitempty"`
}

// decodeSourceDocBody unmarshals a content body into a typed
// SourceDocBody. Empty body → zero-value body, no error.
func decodeSourceDocBody(raw json.RawMessage) (SourceDocBody, error) {
	var b SourceDocBody
	if len(raw) == 0 {
		return b, nil
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return b, fmt.Errorf("sources: decode source-document body: %w", err)
	}
	return b, nil
}

// DecodeBody is decodeSourceDocBody for the service: the typed view of a
// source-document's fields.
func DecodeBody(raw json.RawMessage) (SourceDocBody, error) { return decodeSourceDocBody(raw) }

// Anchor status constants matching the kind YAML enum.
const (
	AnchorStatusValid   = "valid"
	AnchorStatusStale   = "stale"
	AnchorStatusMissing = "missing"
)
