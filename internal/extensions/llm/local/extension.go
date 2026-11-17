// SPDX-License-Identifier: Apache-2.0

// Package local wires the LLM-facing cobra subtree (`<binary> llm …`)
// per the command-surface decision. Verbs follow the canonical
// `<noun> <verb>` ordering with subgroup aliases:
//
//	llm info                — usage skill + calling-conventions header
//	llm manifest            — full self-describe (Sidecar.Info + tool + resource summary)
//	llm tools list          — enumerate callable tools (alias: `llm tls list`)
//	llm tools describe NAME — full JSON schema for one tool
//	llm tools call NAME ARGS — dispatch a tool call
//	llm resources list      — list contributed resources (alias: `llm rsc list`)
//	llm resources get URI   — fetch one resource's metadata/manifest entry
//	llm resources read URI  — fetch one resource's RAW content (--into <file> to save)
//	llm resources types     — list distinct resource types
//
// The parent `llm` cobra group is created upstream by
// `appcore.SidecarGroupsExt()` and carries
// `console.SetGroupDefault(llmCmd, console.OutputJSON)` so every
// descendant defaults to JSON output (override with `--out rich`).
//
// Wiring order in bootstrap.go:
//
//	appcore.NewAppExt(cfg),
//	appcore.SidecarGroupsExt(),  // MUST be before SidecarExt + llmlocal
//	appcore.SidecarExt(efs),
//	llmlocal.Extension(),
//	...
package local

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	"github.com/spf13/cobra"
)

// Extension returns an application.Extension that registers the LLM
// subcommands under the `llm` cobra group from SidecarGroupsExt.
// One line per sidecar in buildExtensions.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddBootstrap("llm-local", ".cli", func(app *application.Application) error {
			grp, err := application.AppUseNamedService[appcore.CobraCommand](app, appcore.GroupNameLLM)
			if err != nil {
				return fmt.Errorf("llm-local: llm group unavailable: %w", err)
			}
			llm := grp.Cmd

			llm.AddCommand(infoCmd())
			llm.AddCommand(manifestCmd())
			llm.AddCommand(toolsCmd())
			llm.AddCommand(resourcesCmd())

			// Operator-facing `sc tools` — the callable analog of
			// `sc resources`, with a type column (tool / task / script).
			// Best-effort: skip on binaries without the `sc` group.
			if sc, err := application.AppUseNamedService[appcore.CobraCommand](app, appcore.GroupNameSidecar); err == nil {
				sc.Cmd.AddCommand(scToolsCommand())
			}
			return nil
		})
	}
}

// infoCmd implements `llm info` — entry point for any LLM driving the
// binary. Returns the sidecar's usage-skill body with mode-specific
// calling-conventions prepended.
func infoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Sidecar usage guide with calling conventions",
		Long: `'llm info' is the LLM-facing entry point — equivalent to '--help'
for humans. Returns the binary's usage skill body plus a calling-
conventions header (CLI invocation patterns, tool-name conventions,
resource-URI grammar).

LLMs should call this FIRST when encountering a sidecar for the first
time.`,
	}
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.LLMService](cmd.Context())
		if err != nil {
			return err
		}
		result, err := svc.Info(cmd.Context())
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "llm-info", result, `{{ .Content }}`)
	}
	return cmd
}

// manifestCmd implements `llm manifest` — full self-describe. Combines
// identity (Sidecar.Info), tool inventory (LLMService.ListTools), and
// resource-type list (LLMService.ListResourceTypes) into one document
// suitable for an LLM doing one-shot discovery.
func manifestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Full self-describe: identity + tools + resource types",
		Long: `'llm manifest' aggregates the sidecar's identity (Sidecar.Info),
its callable tool list (LLMService.ListTools), and the distinct
resource types it contributes (LLMService.ListResourceTypes) into a
single JSON document. Useful for one-shot LLM discovery — alternative
to multiple round-trips.`,
	}
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		var manifest manifestOutput

		if sc, err := application.UseService[api.Sidecar](ctx); err == nil {
			if info, err := sc.Info(ctx); err == nil {
				manifest.Sidecar = info
			}
		}

		llm, err := application.UseService[api.LLMService](ctx)
		if err != nil {
			return err
		}
		// Manifest aggregates the FULL tool list including schemas — an
		// LLM doing one-shot discovery wants payload-buildable info.
		if tools, err := llm.ListTools(ctx, api.LLMListToolsInput{WithSchema: true}); err == nil {
			manifest.Tools = tools.Tools
		}
		if types, err := llm.ListResourceTypes(ctx); err == nil {
			manifest.ResourceTypes = types.Types
		}

		return console.RenderConsole(*mode, "llm-manifest", manifest, "")
	}
	return cmd
}

// manifestOutput is the aggregated self-describe document.
type manifestOutput struct {
	Sidecar       api.InfoReport     `json:"sidecar"`
	Tools         []api.LLMToolEntry `json:"tools"`
	ResourceTypes []string           `json:"resource_types"`
}

// toolsCmd is the `tools` (alias `tls`) noun-group under `llm`.
func toolsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "tools",
		Aliases: []string{"tls"},
		Short:   "Callable tools (list / describe / call)",
		Long:    "Subcommands: list (enumerate all tools), describe NAME (full schema), call NAME ARGS (dispatch).",
	}
	cmd.AddCommand(toolsListCmd())
	cmd.AddCommand(toolsDescribeCmd())
	cmd.AddCommand(toolsCallCmd())
	return cmd
}

func toolsListCmd() *cobra.Command {
	var withSchema bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List callable tools (metadata-only by default; --with-schema for full JSON Schemas)",
		Long: `Lists every callable tool this binary exposes. Default response is
metadata-only — name + description + source (the underlying slot/method).
Use --with-schema to also include each tool's full JSON Schema (useful
when an LLM needs to construct a valid call payload). Mirrors the
parallel 'sc resources list [--with-body]' shape.`,
	}
	cmd.Flags().BoolVar(&withSchema, "with-schema", false, "Include each tool's JSON Schema (default: metadata only)")
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.LLMService](cmd.Context())
		if err != nil {
			return err
		}
		result, err := svc.ListTools(cmd.Context(), api.LLMListToolsInput{WithSchema: withSchema})
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "llm-tools-list", result, toolsListTemplate)
	}
	return cmd
}

// toolsListTemplate renders the tool inventory as a 3-column table
// in rich/raw mode (Name / Source / Description). JSON mode bypasses
// the template via the entries-collect path.
const toolsListTemplate = `{{ header "Tools" }}{{ if .Tools }}{{ table .Tools "Name" "~Source" "Description" }}{{ dim (printf "%d tool(s)" (count .Tools)) }}{{ else }}{{ dim "(none)" }}{{ end }}`

func toolsDescribeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe NAME",
		Short: "Get the JSON schema for one tool",
		Args:  cobra.ExactArgs(1),
	}
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.LLMService](cmd.Context())
		if err != nil {
			return err
		}
		result, err := svc.DescribeTool(cmd.Context(), api.LLMDescribeToolInput{ToolName: args[0]})
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "llm-tools-describe", result, toolsDescribeTemplate)
	}
	return cmd
}

// toolsDescribeTemplate renders name/source/description as label-value
// rows, then dumps the JSON schema verbatim (already pretty when the
// schema is multi-line JSON).
const toolsDescribeTemplate = `{{ header "Tool" }}{{ row "Name" .Name }}{{ row "Source" .Source }}{{ row "Description" .Description }}{{ if .Schema }}
{{ header "Schema" }}{{ printf "%s" .Schema }}
{{ end }}`

func toolsCallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "call NAME [JSON-PARAMS]",
		Short: "Call a tool by name with JSON params",
		Args:  cobra.RangeArgs(1, 2),
	}
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.LLMService](cmd.Context())
		if err != nil {
			return err
		}
		input := api.LLMCallToolInput{ToolName: args[0]}
		if len(args) > 1 {
			input.Params = []byte(args[1])
		}
		result, err := svc.CallTool(cmd.Context(), input)
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "llm-tools-call", result, toolsCallTemplate)
	}
	return cmd
}

// toolsCallTemplate renders the call result: tool name + error (if any)
// + the raw JSON result body.
const toolsCallTemplate = `{{ row "Tool" .ToolName }}{{ if .Error }}{{ error .Error }}{{ else }}
{{ header "Result" }}{{ printf "%s" .Result }}
{{ end }}`

// resourcesCmd is the `resources` (alias `rsc`) noun-group under `llm`.
func resourcesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resources",
		Aliases: []string{"rsc"},
		Short:   "Contributed resources (list / get / read / types)",
		Long:    "Subcommands: list (enumerate), get URI (metadata/manifest entry), read URI (raw content, --into <file> to save), types (distinct kinds).",
	}
	cmd.AddCommand(resourcesListCmd())
	cmd.AddCommand(resourcesGetCmd())
	cmd.AddCommand(resourcesReadCmd())
	cmd.AddCommand(resourcesTypesCmd())
	return cmd
}

func resourcesListCmd() *cobra.Command {
	var typeFilter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resources, optionally filtered by type (--type)",
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by resource type (e.g. skill, reference, config-file)")
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.LLMService](cmd.Context())
		if err != nil {
			return err
		}
		result, err := svc.ListResources(cmd.Context(), api.LLMListResourcesInput{Type: typeFilter})
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "llm-resources-list", result, resourcesListTemplate)
	}
	return cmd
}

// resourcesListTemplate renders the resource list as a 4-column table
// matching the sc resources list shape exactly (the two surfaces
// emit byte-identical JSON; rich rendering should too).
const resourcesListTemplate = `{{ header "Resources" }}{{ if .Resources }}{{ table .Resources "Type" "ID" "~Provider" "URI" }}{{ dim (printf "%d resource(s)" (count .Resources)) }}{{ else }}{{ dim "(none)" }}{{ end }}`

func resourcesGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get URI",
		Short: "Fetch one resource body by URI (<provider>:/<type>/<id>)",
		Args:  cobra.ExactArgs(1),
	}
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.LLMService](cmd.Context())
		if err != nil {
			return err
		}
		result, err := svc.GetResource(cmd.Context(), api.LLMGetResourceInput{URI: args[0]})
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "llm-resources-get", result, resourcesGetTemplate)
	}
	return cmd
}

// resourcesGetTemplate renders the wrapped Contribution: URI/Type/
// Provider/ID rows + the inline data layer. Matches `sc resources get`.
const resourcesGetTemplate = `{{ header "Resource" }}{{ row "URI" .Contribution.URI }}{{ row "Type" .Contribution.Type }}{{ row "Provider" .Contribution.Provider }}{{ row "ID" .Contribution.ID }}{{ if .Contribution.Data }}
{{ header "Body" }}{{ printf "%s" .Contribution.Data }}
{{ end }}`

func resourcesReadCmd() *cobra.Command {
	var into string
	cmd := &cobra.Command{
		Use:   "read URI",
		Short: "Fetch one resource's RAW content by URI (--into <file> to save instead of printing)",
		Args:  cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&into, "into", "", "Write the content to this file path instead of stdout")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.LLMService](cmd.Context())
		if err != nil {
			return err
		}
		result, err := svc.ReadResource(cmd.Context(), api.LLMReadResourceInput{URI: args[0]})
		if err != nil {
			return err
		}
		if into != "" {
			if dir := filepath.Dir(into); dir != "" && dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("create directory for --into: %w", err)
				}
			}
			if err := os.WriteFile(into, result.Content, 0o644); err != nil {
				return fmt.Errorf("write --into file: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %d bytes (%s) to %s\n", len(result.Content), result.MediaType, into)
			return nil
		}
		// Raw content to stdout — no envelope, so the output is usable
		// directly (pipe to a file, feed to another tool, read as text).
		_, err = cmd.OutOrStdout().Write(result.Content)
		return err
	}
	return cmd
}

func resourcesTypesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "types",
		Short: "List distinct resource types this sidecar contributes",
	}
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.LLMService](cmd.Context())
		if err != nil {
			return err
		}
		result, err := svc.ListResourceTypes(cmd.Context())
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "llm-resources-types", result, resourcesTypesTemplate)
	}
	return cmd
}

// resourcesTypesTemplate bullets the type list.
const resourcesTypesTemplate = `{{ header "Resource Types" }}{{ if .Types }}{{ range .Types }}{{ dim . }}{{ end }}{{ dim (printf "%d type(s)" (count .Types)) }}{{ else }}{{ dim "(none)" }}{{ end }}`
