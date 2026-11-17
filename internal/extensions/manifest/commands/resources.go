// SPDX-License-Identifier: Apache-2.0

// Package commands provides the `resources` Cobra command group:
//
//	<binary> sc resources list                  # list (metadata-only by default; --with-body to include bodies)
//	<binary> sc resources list --type skill     # filter by Contribution type
//	<binary> sc resources get <uri>             # fetch one Contribution (metadata/manifest entry)
//	<binary> sc resources read <uri> [--into f] # fetch one resource's RAW content
//	<binary> sc resources types                 # distinct types this binary contributes
//	<binary> sc resources manifest              # full descriptor (slotTypes + hash + contributions)
//
// All sub-verbs are thin shells over `ResourceService` (the
// published per-sidecar slot), so the CLI surface and the KTSP slot
// emit JSON-identical responses. `<binary> llm tools call
// <binary>:resources/list` == `<binary> sc resources list`
// (per the command-surface decision — consistency unification 2026-05-23).
//
// Wired automatically by manifest/local/Extension() under the `sc`
// group; any binary that imports the manifest extension AND
// `appcore.SidecarGroupsExt()` gains these commands.
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/spf13/cobra"
)

// NewResourcesCommand builds the `resources` command group — the operator
// surface for the readable resources this binary publishes. The internal
// envelope/Go type is `Contribution`; the external word is "resource".
// Binary identity is resolved from the cobra root at run-time.
func NewResourcesCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "resources",
		Short: "List, fetch, and inspect the resources this binary publishes",
		Long: `Resources are the polymorphic readable items this binary publishes:
skills, references, tasks, data-types, configs, and any custom kinds. Each
entry has a URI of the form <provider>:/<type>/<id>.

Subcommands:
  list                 List resources (metadata-only by default; --with-body)
  get URI              Fetch one resource's metadata/manifest entry by URI
  read URI             Fetch one resource's RAW content (--into <file> to save)
  types                Distinct resource types on this binary
  manifest             Full descriptor (slot types + hash + resources)

All verbs delegate to the resource service (the published per-sidecar
slot) — invoking them via the CLI produces the SAME JSON the LLM-facing
'llm resources list/get/read/types' returns.`,
	}

	root.AddCommand(newListCommand())
	root.AddCommand(newGetCommand())
	root.AddCommand(newReadCommand())
	root.AddCommand(newTypesCommand())
	root.AddCommand(newManifestCommand())

	return root
}

func newListCommand() *cobra.Command {
	var typeFilter string
	var withBody bool
	var includeSystem bool
	var includeLocal bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resources (metadata-only by default; --with-body)",
		Long: `Delegates to ResourceService.List — the JSON shape is identical
to '<binary> llm tools call <binary>:resources/list'.

Use --type to filter by Contribution type (skill / reference / task / etc.).
Use --with-body to include each Contribution's full body (default: metadata
only — the response is much smaller without bodies).
Use --include-system=false to hide the embedded 'system' meta-model
(contribution-type + data-type schemas); shown by default.`,
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by Contribution type (skill, reference, task, ...)")
	cmd.Flags().BoolVar(&withBody, "with-body", false, "Include each Contribution's body (default: metadata only)")
	cmd.Flags().BoolVar(&includeSystem, "include-system", true, "Include Provider:\"system\" meta-model entries (default true)")
	cmd.Flags().BoolVar(&includeLocal, "include-local", true, "Include Provider:\"local\" binary-private entries (default true for local CLI)")
	outMode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[mbapi.ResourceService](cmd.Context())
		if err != nil {
			return fmt.Errorf("ResourceService not published: %w", err)
		}
		// Local CLI sees everything by default — operators inspecting
		// the binary want full visibility, including binary-private
		// (`Provider == "local"`) artifacts (--include-local defaults
		// true here). Remote bus callers implicitly get the bus-safe
		// filtered view because they leave IncludeLocal at its false zero.
		in := mbapi.ContributionListInput{WithBody: withBody, IncludeLocal: includeLocal, IncludeSystem: &includeSystem}
		if typeFilter != "" {
			in.Filter = mbapi.ContributionListFilter{Types: []string{typeFilter}}
		}
		out, err := svc.List(cmd.Context(), in)
		if err != nil {
			return err
		}
		for i := range out.Contributions {
			if out.Contributions[i].URI == "" {
				out.Contributions[i].URI = out.Contributions[i].ResolvedURI()
			}
		}
		return console.RenderConsole(*outMode, "contributions-list", out, contribListTemplate)
	}
	return cmd
}

// contribListTemplate renders the list as a 3-column table for
// rich/raw mode (Type, ID, URI). JSON mode bypasses the template via
// the entries-collect path.
const contribListTemplate = `{{ header "Contributions" }}{{ if .Contributions }}{{ table .Contributions "Type" "ID" "~Provider" "URI" }}{{ dim (printf "%d contribution(s)" (count .Contributions)) }}{{ else }}{{ dim "(none)" }}{{ end }}`

func newGetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get URI",
		Short: "Fetch one resource by URI",
		Long: `URI form: <provider>:/<type>/<item_path>

Examples:
  <binary> sc resources get ktw-docs:/skill/doc-create
  <binary> sc resources get ktw-docs:/reference/templates/report.typ

Output JSON matches '<binary> llm resources get <uri>'
exactly — wrapping the resource in a {"resource":{...}} envelope.`,
		Args: cobra.ExactArgs(1),
	}
	outMode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[mbapi.ResourceService](cmd.Context())
		if err != nil {
			return fmt.Errorf("ResourceService not published: %w", err)
		}
		out, err := svc.Get(cmd.Context(), mbapi.ContributionGetInput{URI: args[0]})
		if err != nil {
			return err
		}
		if out.Contribution.URI == "" {
			out.Contribution.URI = out.Contribution.ResolvedURI()
		}
		return console.RenderConsole(*outMode, "contributions-get", out, contribGetTemplate)
	}
	return cmd
}

// contribGetTemplate renders the resolved Contribution as label-value
// rows in rich/raw mode (URI, Type, Provider, ID) followed by
// the data layer when it is carried inline (the index lane keeps only the
// inline band; larger data is fetched by hash). JSON mode bypasses the template.
const contribGetTemplate = `{{ header "Contribution" }}{{ row "URI" .Contribution.URI }}{{ row "Type" .Contribution.Type }}{{ row "Provider" .Contribution.Provider }}{{ row "ID" .Contribution.ID }}{{ if .Contribution.Data }}
{{ header "Body" }}{{ printf "%s" .Contribution.Data }}
{{ end }}`

func newReadCommand() *cobra.Command {
	var into string
	cmd := &cobra.Command{
		Use:   "read URI",
		Short: "Fetch one resource's RAW content by URI (--into <file> to save instead of printing)",
		Long: `URI form: <provider>:/<type>/<item_path>

Returns the raw bytes of the resource's backing file — the content lane
(get returns the metadata/manifest entry; references enumerate with a
pointer body, so their content is only reachable here).

Examples:
  <binary> sc resources read ktw-compose:/reference/prompt-engineering.md
  <binary> sc resources read ktw-docs:/reference/diagrams/mermaid/flowchart.md --into flowchart.md`,
		Args: cobra.ExactArgs(1),
	}
	cmd.Flags().StringVar(&into, "into", "", "Write the content to this file path instead of stdout")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[mbapi.ResourceService](cmd.Context())
		if err != nil {
			return fmt.Errorf("ResourceService not published: %w", err)
		}
		out, err := svc.Read(cmd.Context(), mbapi.ContributionReadInput{URI: args[0]})
		if err != nil {
			return err
		}
		if into != "" {
			if dir := filepath.Dir(into); dir != "" && dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return fmt.Errorf("create directory for --into: %w", err)
				}
			}
			if err := os.WriteFile(into, out.Content, 0o644); err != nil {
				return fmt.Errorf("write --into file: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %d bytes (%s) to %s\n", len(out.Content), out.MediaType, into)
			return nil
		}
		// Raw content to stdout — no envelope, directly usable.
		_, err = cmd.OutOrStdout().Write(out.Content)
		return err
	}
	return cmd
}

func newTypesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "types",
		Short: "List distinct resource type discriminators on this binary",
	}
	outMode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[mbapi.ResourceService](cmd.Context())
		if err != nil {
			return fmt.Errorf("ResourceService not published: %w", err)
		}
		types, err := svc.Types(cmd.Context())
		if err != nil {
			return err
		}
		if types == nil {
			types = []string{}
		}
		// Same wrapper shape as `llm resources types` — `{types: [...]}`.
		return console.RenderConsole(*outMode, "contributions-types", map[string][]string{"types": types}, contribTypesTemplate)
	}
	return cmd
}

// contribTypesTemplate renders the distinct-types list as bullets.
const contribTypesTemplate = `{{ header "Contribution Types" }}{{ if .types }}{{ range .types }}{{ dim . }}{{ end }}{{ dim (printf "%d type(s)" (count .types)) }}{{ else }}{{ dim "(none)" }}{{ end }}`

func newManifestCommand() *cobra.Command {
	var includeSystem bool
	var includeLocal bool
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Full binary descriptor (slot types + hash + all contributions with bodies)",
		Long: `Returns the complete Manifest for this binary — slot types, hash,
and every Contribution including bodies. Used by ktw-hub's catalog
hydration path (` + "`appcore.RunResourcesList`" + `) to cache the full
descriptor in one round-trip without spawning the runtime services.

Use --include-system=false to strip the embedded 'system' meta-model
(identical in every binary) — the hub passes this for every sidecar
except its one canonical system carrier, so the universe catalog holds
the system entries exactly once.

Use --include-local=false to strip the binary-private 'local:' entries —
the hub always passes this: a local contribution never crosses a process
boundary, and the probe is the boundary. The default (true) is the
binary's own complete self view.

For just the contributions list (typically what you want as an
operator), use 'sc resources list [--with-body]' instead.`,
	}
	cmd.Flags().BoolVar(&includeSystem, "include-system", true, "Include Provider:\"system\" meta-model entries (default true)")
	cmd.Flags().BoolVar(&includeLocal, "include-local", true, "Include Provider:\"local\" binary-private entries (default true)")
	_ = console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[mbapi.ResourceService](cmd.Context())
		if err != nil {
			return fmt.Errorf("ResourceService not published: %w", err)
		}
		out, err := svc.GetManifest(cmd.Context(), mbapi.ContributionGetManifestInput{IncludeSystem: &includeSystem, IncludeLocal: &includeLocal})
		if err != nil {
			return err
		}
		// Emit the Manifest itself (not the wrapper) for backward
		// compatibility with the hub catalog hydration consumer in
		// `appcore.RunResourcesList`, which unmarshals directly into
		// *mbapi.Manifest. Always JSON — the full descriptor is too
		// big for a rich table; consumers parse it.
		return emitJSON(cmd, out.Manifest)
	}
	return cmd
}

// emitJSON serializes v as indented JSON to the cobra command's
// stdout. Keeps the surface JSON-only — every contributions verb is a
// machine-consumed query, no rich/raw template variants needed today.
func emitJSON(cmd *cobra.Command, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}
