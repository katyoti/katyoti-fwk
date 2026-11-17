// SPDX-License-Identifier: Apache-2.0

// Package patch_ui_component implements the `patch-ui-component` agent
// tool. The cross-turn updater for streamed UI artifacts: lets the LLM
// mutate a previously-mounted artifact's value without remounting (no
// flicker, no scroll loss, no re-render of unchanged subtrees).
//
// Ops are addressed by either:
//   - `target`: a node-id reference (LLM-friendly; the chat panel bridge
//     resolves it to a JSON Pointer in the artifact's tree)
//   - `path`:   a raw JSON Pointer (escape hatch for deep array indexes
//     with no natural id)
//
// 5 op kinds (applied by the chat panel bridge, terminal/chatpanel):
//   - "replace"  : idempotent set at path (wire kind="replace")
//   - "add"      : RFC 6902 add (wire kind="rfc6902", op="add")
//   - "remove"   : delete at path (wire kind="remove")
//   - "append"   : push onto array at path (wire kind="append")
//   - "merge"    : RFC 7396 deep merge (wire kind="rfc7396")
//
// Per dsl-id-based-patching spec (ms06).
package patch_ui_component

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// PatchOp is the LLM-facing patch op. Either `Target` (node id) or
// `Path` (JSON Pointer) addresses the mutation site. When both are
// set, `Path` wins (frontend bypass of id-resolution).
type PatchOp struct {
	Op     string `json:"op" jsonschema:"One of: replace | add | remove | append | merge." required:"true"`
	Target string `json:"target,omitempty" jsonschema:"Node-id reference (preferred). The chat resolves it to a JSON Pointer in the artifact's tree."`
	Path   string `json:"path,omitempty" jsonschema:"Raw JSON Pointer (RFC 6901). Escape hatch when no natural id exists, e.g. '/children/2/data/3'."`
	Value  any    `json:"value,omitempty" jsonschema:"Patched value. Required for replace/add/append/merge; omit for remove."`
}

// PatchUIComponentInput is the input schema. The handler resolves each
// op into a wire-format `terminal.UIPatchOp` and publishes one
// `UIArtifactPatchEvent` carrying all ops in order.
type PatchUIComponentInput struct {
	ArtifactID string    `json:"artifactId" jsonschema:"The id of a previously-mounted artifact (set persistent:true on display-ui-component to allow cross-turn patches)." required:"true"`
	Ops        []PatchOp `json:"ops" jsonschema:"Ordered list of patch ops. Ops apply sequentially; if op[0] adds a node and op[1] targets it by id, the lookup map rebuilds between ops." required:"true"`
}

// PatchUIComponentOutput is the tool's result.
type PatchUIComponentOutput struct {
	Success    bool   `json:"success"`
	ArtifactID string `json:"artifactId"`
	OpCount    int    `json:"opCount"`
	Message    string `json:"message"`
}

// NewPatchUIComponentTool creates the patch-ui-component tool.
func NewPatchUIComponentTool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"patch-ui-component",
		toolDescription,
		executePatch,
		toolsapi.WithReadOnly(),
		toolsapi.WithDescriber(describePatch),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{
			Icon: "🩹", Label: "Patch UI", Color: "#F59E0B",
			Template: "{{.artifactId}} ({{len .ops}} ops)",
		}),
	)
}

func executePatch(ctx context.Context, in PatchUIComponentInput) (PatchUIComponentOutput, error) {
	if strings.TrimSpace(in.ArtifactID) == "" {
		return PatchUIComponentOutput{}, fmt.Errorf("patch-ui-component: artifactId is required")
	}
	if len(in.Ops) == 0 {
		return PatchUIComponentOutput{}, fmt.Errorf("patch-ui-component: ops is required (at least one op)")
	}

	wireOps := make([]any, 0, len(in.Ops))
	for i, op := range in.Ops {
		wo, err := translateOp(op)
		if err != nil {
			return PatchUIComponentOutput{}, fmt.Errorf("patch-ui-component: op[%d]: %w", i, err)
		}
		wireOps = append(wireOps, wo)
	}

	bus, err := application.UseService[engine.EventBus](ctx)
	if err != nil || bus == nil {
		return PatchUIComponentOutput{
			Success:    true,
			ArtifactID: in.ArtifactID,
			OpCount:    len(in.Ops),
			Message:    fmt.Sprintf("UI artifact %q: %d ops queued (no event bus)", in.ArtifactID, len(in.Ops)),
		}, nil
	}

	evt := agentapi.UIArtifactPatchEvent{
		ArtifactID: in.ArtifactID,
		Ops:        wireOps,
	}
	evt.StampEvent(agentapi.EventUIArtifactPatch, agentapi.RunIDFromContext(ctx))
	bus.Publish(agentapi.EventUIArtifactPatch, evt)

	return PatchUIComponentOutput{
		Success:    true,
		ArtifactID: in.ArtifactID,
		OpCount:    len(in.Ops),
		Message:    fmt.Sprintf("UI artifact %q patched (%d ops)", in.ArtifactID, len(in.Ops)),
	}, nil
}

// translateOp converts an LLM-facing PatchOp into the wire-format
// shape consumed by the frontend's applyPatchOps. The 5 LLM op names
// map to {Kind, Op} pairs as documented in the package doc.
func translateOp(op PatchOp) (map[string]any, error) {
	if op.Target == "" && op.Path == "" && op.Op != "merge" {
		return nil, fmt.Errorf("missing target or path (op=%q)", op.Op)
	}

	wire := map[string]any{}
	if op.Target != "" {
		wire["target"] = op.Target
	}
	if op.Path != "" {
		wire["path"] = op.Path
	}

	switch op.Op {
	case "replace":
		if op.Value == nil {
			return nil, fmt.Errorf("replace requires a value")
		}
		wire["kind"] = "replace"
		wire["value"] = op.Value
	case "add":
		if op.Value == nil {
			return nil, fmt.Errorf("add requires a value")
		}
		wire["kind"] = "rfc6902"
		wire["op"] = "add"
		wire["value"] = op.Value
	case "remove":
		wire["kind"] = "remove"
	case "append":
		if op.Value == nil {
			return nil, fmt.Errorf("append requires a value")
		}
		wire["kind"] = "append"
		wire["value"] = op.Value
	case "merge":
		if op.Value == nil {
			return nil, fmt.Errorf("merge requires a value (the merge patch object)")
		}
		wire["kind"] = "rfc7396"
		wire["mergePatch"] = op.Value
	default:
		return nil, fmt.Errorf("unknown op %q (use one of: replace, add, remove, append, merge)", op.Op)
	}

	return wire, nil
}

func describePatch(raw json.RawMessage) string {
	var in PatchUIComponentInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "patch-ui-component <invalid>"
	}
	return fmt.Sprintf("patch-ui-component %q (%d ops)", in.ArtifactID, len(in.Ops))
}

//go:embed prompt.md
var toolDescription string
