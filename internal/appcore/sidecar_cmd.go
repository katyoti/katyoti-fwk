// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/spf13/cobra"
)

// NewKTSPDescribeCommand creates the `ktsp describe` command group.
//
// Phase E §4 reframe (2026-06-05): `ktsp describe` is LIVE-STATE
// introspection — it reports the slots currently bound on the running
// KTSP container plus the capability flags this binary advertises. It
// is NOT the primary discovery surface.
//
// Operators / LLM agents looking to discover what a sidecar publishes
// should use `sc resources list --type service` instead — that
// returns the static contract (a `meta.ServiceDef`-shaped Contribution
// per published service) without depending on the runtime KTSP
// container being instantiated. The contributions path also surfaces
// resources (skills, tasks, data-types) that `ktsp describe` does NOT
// cover.
//
// Use `ktsp describe` to answer "what is alive in this process right
// now?" (debugging, health checks, post-bootstrap verification). Use
// `sc resources list` to answer "what does this binary publish on
// the bus?" (discovery, catalog refresh, LLM tool browsing).
func NewKTSPDescribeCommand(cfg AppConfig, container *ktsp.Endpoint) *cobra.Command {
	ktspCmd := &cobra.Command{
		Use:   "ktsp",
		Short: "KTSP protocol commands",
	}

	descCmd := &cobra.Command{
		Use:   "describe",
		Short: "Inspect live KTSP container state (NOT the discovery surface)",
		Long: `Outputs a live snapshot of the slots currently bound on this
process's KTSP container — debugging / health-check view.

This is LIVE-STATE INTROSPECTION, not the primary discovery surface.
For static contract discovery (the surface ktw-hub catalog refresh and
LLM agents consume), use:

  ` + cfg.AppName + ` sc resources list --type service     # service interfaces
  ` + cfg.AppName + ` sc resources list --type data-type   # I/O types
  ` + cfg.AppName + ` sc resources list --type skill       # skills
  ` + cfg.AppName + ` sc resources list --type task        # tasks (if any)

Those return content-addressable Contribution envelopes that exist
without depending on the runtime container being live. ` + "`ktsp describe`" + `
only sees what's CURRENTLY MOUNTED.`,
	}

	var legacyJSON bool
	mode := console.AddOutputFlag(descCmd)
	descCmd.Flags().BoolVar(&legacyJSON, "json", false, "Output as JSON (shorthand for --out json)")

	descCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if legacyJSON {
			*mode = console.OutputJSON
		}
		manifest := BuildEndpointManifest(cfg.AppName, cfg.Version, cfg.Short, cfg.Transports, container)
		enrichManifestFromContext(cmd.Context(), &manifest)
		return console.RenderConsole(*mode, "manifest", manifest, renderExtensionsFn)
	}

	ktspCmd.AddCommand(descCmd)
	return ktspCmd
}

// NewExtensionsCommand creates the `extensions` command (backward compat alias for `ktsp describe`).
func NewExtensionsCommand(cfg AppConfig, container *ktsp.Endpoint) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "extensions",
		Short:  "Print sidecar capability manifest (alias for 'ktsp describe')",
		Hidden: true,
	}

	// Keep --json for backward compat (maps to --out json)
	var legacyJSON bool
	mode := console.AddOutputFlag(cmd)
	cmd.Flags().BoolVar(&legacyJSON, "json", false, "Output as JSON (shorthand for --out json)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if legacyJSON {
			*mode = console.OutputJSON
		}

		manifest := BuildEndpointManifest(
			cfg.AppName,
			cfg.Version,
			cfg.Short,
			cfg.Transports,
			container,
		)

		return console.RenderConsole(*mode, "manifest", manifest, renderExtensionsFn)
	}

	return cmd
}

// renderExtensionsFn renders the manifest in rich/raw mode.
//
// Slot method listings resolve through the SlotTypes table (the canonical
// source post-dedup) and fall back to inline slot.Methods for legacy
// untyped slots. Each method shows its params signature extracted from the
// JSON Schema's `properties` / `required` fields.
var renderExtensionsFn = console.RenderFunc(func(p *console.Printer, v any) error {
	m := v.(EndpointManifest)
	raw := p.Mode == console.OutputRaw

	// Phase E §4 reframe: this is the LIVE container view, not the
	// discovery surface. Surface the redirect at the top so an operator
	// who lands here for "what does this sidecar publish?" gets pointed
	// at `sc resources list` immediately.
	fmt.Println(styledDim(raw,
		"Live KTSP container state. For static contract discovery, run:"))
	fmt.Println(styledDim(raw,
		"  "+m.Name+" sc resources list --type service|data-type|skill|task"))
	fmt.Println()

	p.Header("Sidecar Manifest (live)")
	printRow(p, "Name", m.Name)
	printRow(p, "Version", m.Version)
	printRow(p, "Description", m.Description)
	printRow(p, "Transports", strings.Join(m.Transports, ", "))

	// Capabilities
	p.Header("Capabilities")
	printRow(p, "CLI", fmt.Sprintf("%v", m.Capabilities.CLI))
	printRow(p, "KTSP", fmt.Sprintf("%v", m.Capabilities.KTSP))
	printRow(p, "HTTP", fmt.Sprintf("%v", m.Capabilities.HTTP))

	if m.KTSP.Name != "" {
		p.Header("KTSP Container")
		printRow(p, "Container", m.KTSP.Name)

		if len(m.KTSP.SlotTypes) > 0 {
			p.Header("Slot Types")
			for _, st := range m.KTSP.SlotTypes {
				fmt.Printf("  %s  v%s  %s\n",
					styledBold(raw, string(st.Type)),
					styledDim(raw, st.Version),
					styledDim(raw, fmt.Sprintf("(%d methods)", len(st.Methods))),
				)
			}
		}

		for _, slot := range m.KTSP.Slots {
			slotType := ""
			if slot.SlotType != "" {
				slotType = fmt.Sprintf(" [%s]", slot.SlotType)
			}
			fmt.Printf("\n  %s%s  %s\n",
				styledBold(raw, slot.Name),
				styledDim(raw, slotType),
				styledDim(raw, slot.Description),
			)

			methods := resolveSlotMethods(m.KTSP, slot)
			if len(methods) == 0 {
				fmt.Printf("    %s\n", styledDim(raw, "(no methods)"))
				continue
			}
			t := p.Table("Method", "Params", "Description")
			for _, meth := range methods {
				params := formatSchemaParams(meth.InputSchema)
				t.AddRow(meth.Name, params, meth.Description)
			}
			t.Print()
		}
	}

	fmt.Println()
	return nil
})

// resolveSlotMethods returns the method contract for a slot. Typed slots
// reference the SlotTypes table; untyped slots keep their methods inline
// (legacy shape). Mirrors sidecar_plugin.methods().
func resolveSlotMethods(container ktsp.ContainerInfo, slot ktsp.SlotInfo) []ktsp.MethodInfo {
	if slot.SlotType != "" {
		for _, st := range container.SlotTypes {
			if st.Type == slot.SlotType {
				return st.Methods
			}
		}
	}
	return slot.Methods
}

// formatSchemaParams renders a compact parameter signature from a JSON
// Schema tree: "field1: type, field2?: type". Required fields come first
// and use "field: type"; optional fields use "field?: type". Returns "—"
// when the schema has no properties or cannot be parsed.
func formatSchemaParams(schema any) string {
	obj, ok := schema.(map[string]any)
	if !ok {
		return "—"
	}
	props, _ := obj["properties"].(map[string]any)
	if len(props) == 0 {
		return "—"
	}
	required := map[string]bool{}
	if reqList, ok := obj["required"].([]any); ok {
		for _, r := range reqList {
			if s, ok := r.(string); ok {
				required[s] = true
			}
		}
	}
	// Sort so required come first, then alphabetical within each group.
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		ri, rj := required[names[i]], required[names[j]]
		if ri != rj {
			return ri
		}
		return names[i] < names[j]
	})
	parts := make([]string, 0, len(names))
	for _, name := range names {
		field, _ := props[name].(map[string]any)
		typeStr, _ := field["type"].(string)
		if typeStr == "" {
			typeStr = "any"
		}
		marker := ""
		if !required[name] {
			marker = "?"
		}
		parts = append(parts, fmt.Sprintf("%s%s:%s", name, marker, typeStr))
	}
	return strings.Join(parts, ", ")
}

func printRow(p *console.Printer, label, value string) {
	if p.Mode == console.OutputRaw {
		if value == "" {
			value = "(not set)"
		}
		fmt.Printf("  %-22s%s\n", label, value)
	} else {
		console.PrintRow(label, value)
	}
}

func styledBold(raw bool, s string) string {
	if raw {
		return s
	}
	return console.TitleStyle.Render(s)
}

func styledDim(raw bool, s string) string {
	if raw {
		return s
	}
	return console.DimStyle.Render(s)
}

// enrichManifestFromContext populates capabilities from the application context.
func enrichManifestFromContext(ctx context.Context, m *EndpointManifest) {
	m.Capabilities.CLI = true // always
	published := application.UseExtensions[publish.PublishedService](ctx)
	for _, ps := range published {
		if ps.HasCapability("ktsp-container") {
			m.Capabilities.KTSP = true
		}
		if ps.HasCapability("http-server") {
			m.Capabilities.HTTP = true
		}
	}
}
