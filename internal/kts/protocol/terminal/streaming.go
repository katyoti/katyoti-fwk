// SPDX-License-Identifier: Apache-2.0

// Producer helpers for the LLM streaming protocol: the patch op
// constructors (`terminal.ReplaceOp(path, v)`) and the renderer
// registration events, the typed structs left in events.go for
// serialization.
//
// See modules/display/streaming-protocol.md.

package terminal

import (
	"encoding/json"
	"time"
)

// nowTS returns the current time as an RFC 3339 timestamp. Matches the
// existing TeamWorkflowStartedEvent / TeamMessageEvent format.
func nowTS() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ── Patch op constructors ──────────────────────────────────────────────

// RFC6902Op creates a JSON Patch (RFC 6902) operation. `op` must be one
// of: add, remove, replace, move, copy, test. `path` is a JSON pointer
// (RFC 6901). For move/copy, supply `from`; for add/replace/test, supply
// `value` (JSON-marshaled).
func RFC6902Op(op, path string, value any, from string) UIPatchOp {
	var raw json.RawMessage
	if value != nil {
		raw, _ = json.Marshal(value)
	}
	return UIPatchOp{
		Kind:  "rfc6902",
		Op:    op,
		Path:  path,
		Value: raw,
		From:  from,
	}
}

// RFC7396Op creates a JSON Merge Patch (RFC 7396) operation. The merge
// patch object replaces the artifact's value (with merge semantics:
// nested objects merged, null deletes, arrays replaced).
func RFC7396Op(mergePatch any) UIPatchOp {
	raw, _ := json.Marshal(mergePatch)
	return UIPatchOp{
		Kind:       "rfc7396",
		MergePatch: raw,
	}
}

// AppendOp pushes `value` onto the array at `path`.
func AppendOp(path string, value any) UIPatchOp {
	raw, _ := json.Marshal(value)
	return UIPatchOp{
		Kind:  "append",
		Path:  path,
		Value: raw,
	}
}

// ReplaceOp sets `path` to `value` (idempotent shallow set).
func ReplaceOp(path string, value any) UIPatchOp {
	raw, _ := json.Marshal(value)
	return UIPatchOp{
		Kind:  "replace",
		Path:  path,
		Value: raw,
	}
}

// RemoveOp deletes the field at `path`.
func RemoveOp(path string) UIPatchOp {
	return UIPatchOp{
		Kind: "remove",
		Path: path,
	}
}

// ── Event constructors ─────────────────────────────────────────────────

// NewRendererRegistrationEvent registers a custom renderer at runtime.
// `source` should be "system" for trusted built-ins or "llm" for LLM-
// emitted registrations (sandboxed by the host).
func NewRendererRegistrationEvent(rendererID, source string, config any) UIRendererRegistrationEvent {
	var raw json.RawMessage
	if config != nil {
		raw, _ = json.Marshal(config)
	}
	return UIRendererRegistrationEvent{
		Type:       "ui.renderer_registration",
		RendererID: rendererID,
		Source:     source,
		Config:     raw,
		Timestamp:  nowTS(),
	}
}

// NewRendererDeregistrationEvent removes a previously-registered renderer.
func NewRendererDeregistrationEvent(rendererID string) UIRendererDeregistrationEvent {
	return UIRendererDeregistrationEvent{
		Type:       "ui.renderer_deregistration",
		RendererID: rendererID,
		Timestamp:  nowTS(),
	}
}
