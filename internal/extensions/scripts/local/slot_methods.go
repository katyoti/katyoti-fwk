// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scripts/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// PerScriptSlotMethodsExtension surfaces every discovered script as its
// own KTSP slot method on the sidecar's ScriptService slot. Per 2E:
// the LLM sees scripts as first-class callable URIs alongside service
// methods — `<provider>:scripts/<script-name>` per the URI scheme in
// the command-surface decision (revised 6U).
//
// Lifecycle:
//   - AppInit phase: AutoKTSPContainer builds the `scripts` slot with
//     the Go-method-derived methods (List, Run).
//   - Bootstrap phase (THIS extension): resolve the *ktsp.Endpoint,
//     find the `scripts` slot, resolve the ScriptService instance,
//     enumerate discovered scripts via List(ctx), add one RawMethod
//     per script that closures the script name and delegates to Run.
//
// Per-script methods carry the `cli=false` tag so they don't pollute
// the CLI verb tree — the legacy `<binary> scripts run <name>` verb
// stays as the user-facing CLI surface. The new per-script slot URIs
// are visible in `<binary> ktsp describe` and in the catalog.
//
// Wire it once per binary, immediately after scriptslocal.Extension():
//
//	scriptslocal.Extension(),
//	scriptslocal.PerScriptSlotMethodsExtension(),
func PerScriptSlotMethodsExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddBootstrap("scripts", ".per-script-slot-methods", func(app *application.Application) error {
			endpoint, err := application.AppUseService[*ktsp.Endpoint](app)
			if err != nil || endpoint == nil {
				// No KTSP container in this binary — silently skip
				// (some binaries don't expose KTSP).
				return nil
			}

			slot := endpoint.Slot(api.ScriptServiceDef.Name, ktsp.DefaultInstance)
			if slot == nil {
				// No `scripts` slot in this binary — skip.
				return nil
			}

			svc, err := application.AppUseService[api.ScriptService](app)
			if err != nil || svc == nil {
				return fmt.Errorf("per-script slot methods: resolve ScriptService: %w", err)
			}

			out, err := svc.List(context.Background())
			if err != nil {
				return fmt.Errorf("per-script slot methods: list scripts: %w", err)
			}

			inputSchema := mustInputSchema()
			outputSchema := mustOutputSchema()

			for _, info := range out.Scripts {
				methodName := scriptMethodName(info)
				if methodName == "" {
					continue
				}
				addPerScriptMethod(slot, svc, info, methodName, inputSchema, outputSchema)
			}
			return nil
		})
	}
}

// scriptMethodName produces the kebab-case slot-method name for a
// script. The method lives ON the ScriptService slot of the sidecar
// that ships the script, so the URI is naturally
// `<info.Provider>:scripts/<methodName>`. Per the command-surface decision
// the method token must match `[a-z0-9-]{1,63}`; script names
// already follow that convention. Empty/invalid names are skipped.
func scriptMethodName(info api.ScriptInfo) string {
	name := info.Name
	if name == "" {
		return ""
	}
	// Sanity bound — slot tokens are 63 chars max per 6U.
	if len(name) > 63 {
		return ""
	}
	return name
}

// PerScriptInput is the input schema for a per-script slot method.
// Mirrors ScriptRunInput minus the Name field (encoded in the method).
type PerScriptInput struct {
	Params map[string]string `json:"params,omitempty" desc:"Named parameters (passed as SCRIPT_PARAM_* env vars)"`
	Args   []string          `json:"args,omitempty"   desc:"Positional arguments"`
}

// addPerScriptMethod registers one RawMethod on the slot for a single
// script. The handler closures over the script URI and dispatches to
// ScriptService.Run.
func addPerScriptMethod(
	slot *ktsp.Slot,
	svc api.ScriptService,
	info api.ScriptInfo,
	methodName string,
	inputSchema, outputSchema any,
) {
	scriptURI := info.URI()
	desc := info.Description
	if desc == "" {
		desc = fmt.Sprintf("Run script %s", scriptURI)
	}

	handler := func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in PerScriptInput
		if len(params) > 0 {
			if err := json.Unmarshal(params, &in); err != nil {
				return nil, fmt.Errorf("unmarshal per-script input: %w", err)
			}
		}
		out, err := svc.Run(ctx, api.ScriptRunInput{
			Name:   scriptURI,
			Params: in.Params,
			Args:   in.Args,
		})
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}

	slot.Add(ktsp.RawMethod(methodName, ktsp.MethodOpts{
		Desc:         desc,
		Tags:         ktsp.MethodTags{ktsp.TagMethodCLI: "false"},
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
		// The script's bound, advertised: a caller (ActionService, the GUI's
		// action dispatch) waits as long as the engine lets the run last.
		TimeoutS: info.TimeoutS,
	}, handler))
}

// mustInputSchema generates the JSON Schema for PerScriptInput once.
// Failure is logged + nil-returned by the underlying helper; we
// pass through whatever the helper produces (nil = no schema).
func mustInputSchema() any {
	raw, err := corereflect.JSONSchema[PerScriptInput]()
	if err != nil || len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

// mustOutputSchema generates the JSON Schema for ScriptRunOutput once.
func mustOutputSchema() any {
	raw, err := corereflect.JSONSchema[api.ScriptRunOutput]()
	if err != nil || len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}
