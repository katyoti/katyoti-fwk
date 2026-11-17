// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// LLMService provides the unified LLM-facing interface.
// Every sidecar publishes this service via SidecarExt(efs).
// CLI verbs are grouped under `<sidecar> llm <verb>`.
//
// The interface mirrors MCP's tools+resources duality: tool methods
// (CallTool/ListTools/DescribeTool) cover the "callable operations"
// surface; resource methods (ListResources/GetResource/ListResourceTypes)
// cover the "queryable content" surface. An LLM client driving a sidecar
// through `<binary> llm` sees the same shape as an MCP server.
type LLMService interface {
	// Info loads the sidecar's `usage` skill with the mode-specific calling
	// conventions header prepended. This is the "getting started" guide for
	// any LLM encountering the sidecar for the first time.
	Info(ctx context.Context) (LLMInfoOutput, error)

	// CallTool dispatches a tool call by tool name with JSON params.
	// Resolves the tool name via the internal registry, routes to the
	// appropriate handler (Go method or external script), returns the result.
	CallTool(ctx context.Context, in LLMCallToolInput) (LLMCallToolOutput, error)

	// ListTools returns all callable tools. Defaults to metadata-only
	// (name + description + source); set LLMListToolsInput.WithSchema=true
	// to include each tool's full JSON Schema (matches the parallel
	// ResourceService.List(WithBody=...) semantics).
	ListTools(ctx context.Context, in LLMListToolsInput) (LLMListToolsOutput, error)

	// DescribeTool returns the full JSON schema for one tool by name.
	DescribeTool(ctx context.Context, in LLMDescribeToolInput) (LLMDescribeToolOutput, error)

	// ListResources returns every resource this sidecar contributes,
	// optionally filtered by type. Mirrors MCP's resources/list. Delegates
	// to the binary's ResourceService.
	ListResources(ctx context.Context, in LLMListResourcesInput) (LLMListResourcesOutput, error)

	// GetResource fetches one resource's metadata/manifest entry by URI
	// (`<provider>:/<type>/<itemPath>`). For file-backed kinds (references)
	// the body is the manifest pointer — fetch raw content with ReadResource.
	GetResource(ctx context.Context, in LLMGetResourceInput) (LLMGetResourceOutput, error)

	// ReadResource fetches one resource's RAW content by URI — the bytes
	// of the backing file plus its media type. Mirrors MCP's
	// resources/read. Delegates to the binary's ResourceService.Read.
	ReadResource(ctx context.Context, in LLMReadResourceInput) (LLMReadResourceOutput, error)

	// ListResourceTypes returns the distinct resource types this sidecar
	// contributes (skill, reference, config-file, …). Cheap discovery —
	// LLM clients can list types before listing each type's resources.
	ListResourceTypes(ctx context.Context) (LLMListResourceTypesOutput, error)
}

// LLMServiceDef is the service definition for LLMService.
var LLMServiceDef = meta.ServiceDef{
	Name:    "llm",
	Short:   "LLM-facing interface",
	Long:    "Provides tool dispatch, tool discovery, resource enumeration, and usage guide for LLMs driving the sidecar.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Info":              meta.Method("Load the sidecar usage guide with calling conventions"),
		"CallTool":          meta.Method("Call any tool by name with JSON params"),
		"ListTools":         meta.Method("List all callable tools with schemas"),
		"DescribeTool":      meta.Method("Get the JSON schema for one tool"),
		"ListResources":     meta.Method("List resources, optionally filtered by type"),
		"GetResource":       meta.Method("Fetch one resource's metadata/manifest entry by URI"),
		"ReadResource":      meta.Method("Fetch one resource's RAW content (bytes + media type) by URI"),
		"ListResourceTypes": meta.Method("List distinct resource types"),
	},
}

// --- I/O types ---

// LLMInfoOutput contains the usage guide + calling conventions header.
type LLMInfoOutput struct {
	Content string `json:"content" description:"Usage skill body with mode header prepended"`
	Mode    string `json:"mode"    description:"Consumption mode used for header generation (agent, cli, ktsp)"`
}

// LLMCallToolInput specifies a tool call.
type LLMCallToolInput struct {
	ToolName string          `json:"tool_name" cli:"arg"    description:"Tool name (sidecar-unique)"`
	Params   json.RawMessage `json:"params"    cli:"params" description:"JSON parameters for the tool"`
}

// LLMCallToolOutput is the tool call result.
type LLMCallToolOutput struct {
	ToolName string          `json:"tool_name"`
	Result   json.RawMessage `json:"result"`
	Error    string          `json:"error,omitempty"`
}

// LLMListToolsInput parameterizes ListTools. Defaults to metadata-only
// (Schema is stripped); set WithSchema=true to include each tool's
// full JSON Schema. Parallels ContributionListInput.WithBody.
type LLMListToolsInput struct {
	WithSchema bool `json:"withSchema,omitempty" cli:"with-schema" desc:"Include each tool's JSON Schema (default false — metadata only)"`
}

// LLMListToolsOutput contains all callable tools. Schema is populated
// only when LLMListToolsInput.WithSchema=true; otherwise the field is
// nil on every entry.
type LLMListToolsOutput struct {
	Tools []LLMToolEntry `json:"tools"`
}

// LLMToolEntry describes one callable tool.
type LLMToolEntry struct {
	Name        string          `json:"name"         description:"Sidecar-unique tool name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"       description:"JSON Schema for the tool's input parameters"`
	Source      string          `json:"source"       description:"Origin: service method name or script path"`
}

// LLMDescribeToolInput specifies which tool to describe.
type LLMDescribeToolInput struct {
	ToolName string `json:"tool_name" cli:"arg" description:"Tool name"`
}

// LLMDescribeToolOutput contains the full schema for one tool.
type LLMDescribeToolOutput struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
	Source      string          `json:"source"`
}

// --- Resource methods (MCP-shaped surface) ---

// LLMListResourcesInput parameterizes ListResources. Type optional.
type LLMListResourcesInput struct {
	Type string `json:"type,omitempty" cli:"type" description:"Filter by resource type (skill, reference, config-file, ...). Empty = all."`
}

// LLMListResourcesOutput contains the resource list. Each entry is a
// full `contribapi.Contribution` — same shape `ResourceService.List`
// emits, so `llm resources list` and `llm tools call
// <binary>:resources/list` are JSON-identical (2026-05-23
// unification per the command-surface decision).
//
// Bodies are stripped on the wire by default (matches
// ResourceService.List's WithBody=false semantics). Consumers that
// need the body call `llm resources get <uri>` (or
// `<binary>:resources/get`).
type LLMListResourcesOutput struct {
	Resources []contribapi.Contribution `json:"resources"`
}

// LLMGetResourceInput specifies the resource URI to fetch.
type LLMGetResourceInput struct {
	URI string `json:"uri" cli:"arg" description:"Resource URI: <provider>:/<type>/<itemPath>"`
}

// LLMGetResourceOutput contains the resolved Contribution. Wraps the
// SAME `contribapi.Contribution` shape `ResourceService.Get` returns —
// `llm resources get` and `<binary>:resources/get` are JSON-
// identical (2026-05-23 unification per the command-surface decision).
type LLMGetResourceOutput struct {
	Contribution contribapi.Contribution `json:"resource"`
}

// LLMReadResourceInput specifies the resource URI whose raw content to read.
type LLMReadResourceInput struct {
	URI string `json:"uri" cli:"arg" description:"Resource URI: <provider>:/<type>/<itemPath>"`
}

// LLMReadResourceOutput contains the resource's raw content. Same shape
// `ResourceService.Read` returns — `llm resources read` and
// `<binary>:resources/read` are JSON-identical. Content is base64 on
// the JSON wire ([]byte marshaling); CLI surfaces decode and print or
// write it raw.
type LLMReadResourceOutput struct {
	URI       string `json:"uri"`
	MediaType string `json:"mediaType,omitempty"`
	Content   []byte `json:"content"`
}

// LLMListResourceTypesOutput lists the distinct types this sidecar
// contributes. Cheap discovery: an LLM client can list types before
// drilling into each type's resources.
type LLMListResourceTypesOutput struct {
	Types []string `json:"types"`
}
