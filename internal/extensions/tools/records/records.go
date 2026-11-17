// SPDX-License-Identifier: Apache-2.0

// Package records ships the record-persistence agent tools behind the
// schema-driven CRUD flows (load-record / list-records / save-record /
// delete-record).
//
// The tools are THIN WRAPPERS over the generic CRUDService contract: every record lives
// on a named CONNECTION (an instance of a CRUD strategy — `memory`,
// future SQL/REST), and is addressed by a URI in the
// external form
//
//	<connection>:/<relative-uri>
//
// e.g. `memory:/book/123` (record on the memory connection). The tool
// strips the connection prefix and dispatches the relative URI to
// `<provider>:crud-service(<connection>)/<Method>` over the message
// bus — the exact wire surface the GUI and other processes use.
//
// save-record optionally validates the value against a registered
// data-type BEFORE persisting (one bus round-trip to the hub's
// SchemaService). Validation failures come back as structured
// per-field violations in the tool output (not a tool error) so the
// LLM can patch the offending fields in the form.
package records

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	coreschemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	crudapi "github.com/katyoti/katyoti-fwk/internal/extensions/crud/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

const validateSlotPath = "ktw-hub:schema/ValidateAgainst"

// Violation is one structured validation finding from the substrate
// validator, flattened for LLM consumption (path → form field).
type Violation struct {
	Severity string `json:"severity"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

// parseConnURI splits the external record-URI form
// `<connection>:/<relative-uri>` into its connection name and the
// connection-relative URI (per cwm-v3 §2.4's round-trip rule).
func parseConnURI(uri string) (connection string, relative string, err error) {
	i := strings.Index(uri, ":")
	if i <= 0 {
		return "", "", fmt.Errorf("record URI %q must be <connection>:/<path> (e.g. memory:/book/123)", uri)
	}
	connection = uri[:i]
	relative = strings.TrimPrefix(uri[i+1:], "/")
	if relative == "" {
		return "", "", fmt.Errorf("record URI %q has an empty path after the connection prefix", uri)
	}
	return connection, relative, nil
}

// busSender builds a scoped messagebus sender from the registered
// bus bridge — the canonical in-process path for slot calls (there
// is NO resolvable ConnectionManager service in a universe core).
func busSender(ctx context.Context) (ktsp.Sender, error) {
	bus, err := application.UseService[mbapi.MessageBus](ctx)
	if err != nil {
		return nil, fmt.Errorf("message bus unavailable: %w", err)
	}
	tenantID := messagebus.TenantFromContext(ctx)
	universeID := messagebus.UniverseFromContext(ctx)
	if tenantID == "" || universeID == "" {
		fallbackTenant, fallbackUniverse := cenv.ResolveTenantUniverse(application.Env().UniverseDir())
		if tenantID == "" {
			tenantID = fallbackTenant
		}
		if universeID == "" {
			universeID = fallbackUniverse
		}
	}
	return messagebus.NewSender(bus, tenantID, universeID), nil
}

// crudCall dispatches one CRUDService method on a named connection
// over the bus: `<provider>:crud-service(<connection>)/<method>`.
// provider is the binary hosting the connection instances (captured
// at Extension contribute time — the agent's own binary).
func crudCall[O any](ctx context.Context, provider, connection, method string, in any) (O, error) {
	var zero O
	sender, err := busSender(ctx)
	if err != nil {
		return zero, err
	}
	path := provider + ":crud-service(" + connection + ")/" + method
	raw, err := sender.Call(ctx, path, in)
	if err != nil {
		return zero, fmt.Errorf("%s: %w", path, err)
	}
	var out O
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, fmt.Errorf("%s: decode response: %w", path, err)
	}
	return out, nil
}

// validateAgainst is the schema-validation hook — one bus round-trip
// to the hub's SchemaService (the universe-wide data-type authority).
// Var so tests can stub it.
var validateAgainst = func(ctx context.Context, schemaURI string, value map[string]any) (bool, []Violation, error) {
	sender, err := busSender(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("schema validation unavailable: %w", err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return false, nil, fmt.Errorf("encode record value: %w", err)
	}
	raw, err := sender.Call(ctx, validateSlotPath, schemaapi.ValidateAgainstInput{URI: schemaURI, Body: body})
	if err != nil {
		return false, nil, fmt.Errorf("ValidateAgainst(%s): %w", schemaURI, err)
	}
	var out coreschemaapi.ValidateOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return false, nil, fmt.Errorf("decode ValidateAgainst response: %w", err)
	}
	violations := make([]Violation, 0, len(out.Diagnostics))
	for _, d := range out.Diagnostics {
		violations = append(violations, Violation{
			Severity: string(d.Severity),
			Code:     d.Code,
			Message:  d.Message,
			Path:     d.Path,
		})
	}
	return out.Valid, violations, nil
}

// flattenRecord decodes a crud Record's body into a map and injects
// the server-side identity (`id`) so consumers (table rows, form
// values) always see it.
func flattenRecord(rec crudapi.Record) map[string]any {
	value := map[string]any{}
	if len(rec.Body) > 0 {
		// Best-effort decode; a non-object body surfaces under `_value`.
		if err := json.Unmarshal(rec.Body, &value); err != nil {
			var anyBody any
			if json.Unmarshal(rec.Body, &anyBody) == nil {
				value = map[string]any{"_value": anyBody}
			}
		}
	}
	value["id"] = rec.ID
	return value
}

// --- tool inputs / outputs -------------------------------------------------

type LoadRecordInput struct {
	URI string `json:"uri" jsonschema:"Record URI: <connection>:/<type>/<id> (e.g. memory:/book/123); required"`
}

type LoadRecordOutput struct {
	ID    string         `json:"id"`
	Value map[string]any `json:"value"`
}

type ListRecordsInput struct {
	URI string `json:"uri" jsonschema:"Collection URI: <connection>:/<type> (e.g. memory:/book); required"`
}

type ListRecordsOutput struct {
	Records []map[string]any `json:"records"`
	Count   int              `json:"count"`
}

type SaveRecordInput struct {
	URI       string         `json:"uri"                  jsonschema:"Target URI. Collection form (<connection>:/<type>) creates with a server-assigned id; record form (<connection>:/<type>/<id>) upserts that id; required"`
	Value     map[string]any `json:"value"                jsonschema:"The record fields; required"`
	SchemaURI string         `json:"schema_uri,omitempty" jsonschema:"Optional data-type URI (e.g. ktw-demo:/data-type/book). When set, the value is validated against it BEFORE persisting; failures return per-field violations"`
}

type SaveRecordOutput struct {
	ID         string      `json:"id,omitempty"`
	URI        string      `json:"uri,omitempty"        jsonschema:"The record's canonical URI after the save (<connection>:/<type>/<id>)"`
	Success    bool        `json:"success"`
	Violations []Violation `json:"violations,omitempty" jsonschema:"Per-field validation findings when success=false; patch the named fields and retry"`
}

type DeleteRecordInput struct {
	URI string `json:"uri" jsonschema:"Record URI: <connection>:/<type>/<id>; required"`
}

type DeleteRecordOutput struct {
	Success bool `json:"success"`
}

// --- tools -------------------------------------------------------------------

// RecordTools returns the record-persistence tool set. provider is
// the binary hosting the crud-service connection instances (the
// agent's own binary — captured at Extension contribute time).
func RecordTools(provider string) []toolsapi.Tool {
	return []toolsapi.Tool{
		toolsapi.NewFunctionTool[LoadRecordInput, LoadRecordOutput](
			"load-record",
			"Load one record by URI (<connection>:/<type>/<id>). Returns the record's fields with `id` injected.",
			func(ctx context.Context, in LoadRecordInput) (LoadRecordOutput, error) {
				conn, rel, err := parseConnURI(in.URI)
				if err != nil {
					return LoadRecordOutput{}, err
				}
				out, err := crudCall[crudapi.LoadOutput](ctx, provider, conn, "Load",
					crudapi.LoadInput{URI: crudapi.URI(rel)})
				if err != nil {
					return LoadRecordOutput{}, err
				}
				return LoadRecordOutput{ID: out.Record.ID, Value: flattenRecord(out.Record)}, nil
			},
			toolsapi.WithReadOnly(),
		),

		toolsapi.NewFunctionTool[ListRecordsInput, ListRecordsOutput](
			"list-records",
			"List records of a collection URI (<connection>:/<type>) — e.g. to render a table. Each record carries its `id`.",
			func(ctx context.Context, in ListRecordsInput) (ListRecordsOutput, error) {
				conn, rel, err := parseConnURI(in.URI)
				if err != nil {
					return ListRecordsOutput{}, err
				}
				out, err := crudCall[crudapi.ListOutput](ctx, provider, conn, "List",
					crudapi.ListInput{URI: crudapi.URI(rel)})
				if err != nil {
					return ListRecordsOutput{}, err
				}
				rows := make([]map[string]any, 0, len(out.Records))
				for _, rec := range out.Records {
					rows = append(rows, flattenRecord(rec))
				}
				return ListRecordsOutput{Records: rows, Count: len(rows)}, nil
			},
			toolsapi.WithReadOnly(),
		),

		toolsapi.NewFunctionTool[SaveRecordInput, SaveRecordOutput](
			"save-record",
			"Create (collection URI) or upsert (record URI) a record. With schema_uri set, the value is validated BEFORE persisting; on failure the output carries per-field violations — fix those fields and retry.",
			func(ctx context.Context, in SaveRecordInput) (SaveRecordOutput, error) {
				conn, rel, err := parseConnURI(in.URI)
				if err != nil {
					return SaveRecordOutput{}, err
				}
				if in.Value == nil {
					return SaveRecordOutput{}, fmt.Errorf("value is required")
				}
				// `id` mirrors the server-side identity — strip it from the
				// stored body so validation doesn't trip on a server field.
				value := make(map[string]any, len(in.Value))
				for k, v := range in.Value {
					if k == "id" {
						continue
					}
					value[k] = v
				}
				if in.SchemaURI != "" {
					valid, violations, err := validateAgainst(ctx, in.SchemaURI, value)
					if err != nil {
						return SaveRecordOutput{}, err
					}
					if !valid {
						return SaveRecordOutput{Success: false, Violations: violations}, nil
					}
				}
				body, err := json.Marshal(value)
				if err != nil {
					return SaveRecordOutput{}, fmt.Errorf("encode record value: %w", err)
				}
				out, err := crudCall[crudapi.SaveOutput](ctx, provider, conn, "Save",
					crudapi.SaveInput{URI: crudapi.URI(rel), Body: body})
				if err != nil {
					return SaveRecordOutput{}, err
				}
				canonical := conn + ":/" + out.Record.Type + "/" + out.Record.ID
				return SaveRecordOutput{ID: out.Record.ID, URI: canonical, Success: true}, nil
			},
			toolsapi.WithMutating(),
		),

		toolsapi.NewFunctionTool[DeleteRecordInput, DeleteRecordOutput](
			"delete-record",
			"Delete a record by URI (<connection>:/<type>/<id>). Irreversible — confirm with the user first (ask-user) unless the user already approved.",
			func(ctx context.Context, in DeleteRecordInput) (DeleteRecordOutput, error) {
				conn, rel, err := parseConnURI(in.URI)
				if err != nil {
					return DeleteRecordOutput{}, err
				}
				out, err := crudCall[crudapi.DeleteOutput](ctx, provider, conn, "Delete",
					crudapi.DeleteInput{URI: crudapi.URI(rel)})
				if err != nil {
					return DeleteRecordOutput{}, err
				}
				return DeleteRecordOutput{Success: out.OK}, nil
			},
			toolsapi.WithDestructive(),
		),
	}
}

const recordsPluginSystemMessage = `Record tools persist typed data records over named CRUD
connections (cwm-v3 §2.4). A record URI is <connection>:/<type>/<id>;
a collection URI is <connection>:/<type>. The "memory" connection is
in-process and cleared on restart (demos, scratch); durable data is CWM
objects (the cwm-object-* tools). Use
list-records to fetch table rows, load-record before showing or
editing one record, save-record to create/update (pass schema_uri to
validate first; it returns per-field violations on failure),
delete-record only after user confirmation.`

// Extension contributes the `records` plugin strategy (bind via
// `plugins: [records]` in TASK.md). Add to binaries that run the
// agent loop AND host the crud-service connections (ktw-agents).
func Extension(init *application.ApplicationInit) {
	init.AddContribute("records-tools", "", func(app *application.Application) error {
		provider := app.Provider()
		strategy := strategies.FuncStrategy[agentapi.PluginFactory](
			"records", "Typed record persistence tools (schema-driven CRUD)",
			func() (agentapi.PluginFactory, error) {
				return &agentapi.FuncFactory{
					FactoryName: "records",
					CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
						return []agentapi.AgentPlugin{
							toolsapi.NewToolGroupPlugin("records", recordsPluginSystemMessage, RecordTools(provider)),
						}, nil
					},
				}, nil
			},
		)
		return agentapi.ContributePluginStrategy(app, strategy)
	})
}
