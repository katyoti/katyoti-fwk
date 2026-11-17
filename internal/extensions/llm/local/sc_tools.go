// SPDX-License-Identifier: Apache-2.0

package local

import (
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	"github.com/spf13/cobra"
)

// scToolsCommand builds the operator-facing `sc tools` group — the callable
// analog of `sc resources`. It lists this binary's callable tools WITH a
// `type` discriminator (tool / task / script), so an operator can answer
// "what can I call here, and of what kind?". Data comes from the same
// LLMService.ListTools the LLM-facing `llm tools list` uses; this surface
// adds the type column + a `--type` filter + a `types` verb, mirroring
// `sc resources list --type` / `sc resources types`.
func scToolsCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "tools",
		Short: "List and inspect the callable tools this binary publishes",
		Long: `Tools are the callable items this binary publishes — addressed by URI
<provider>:<slot>/<method>. Each carries a type derived from its slot:

  tool      a published service method (the default)
  task      a task (slot ` + "`tasks`" + `) — call a short one directly; dispatch
            long-running work in the background and poll its status
  script    a script (slot ` + "`scripts`" + `)

Subcommands:
  list [--type tool|task|script]   List callable tools (with type column)
  get NAME                         Full JSON schema for one tool
  types                            Distinct tool types on this binary

This is the operator analog of the LLM-facing 'llm tools list'.`,
	}
	root.AddCommand(scToolsListCmd())
	root.AddCommand(scToolsGetCmd())
	root.AddCommand(scToolsTypesCmd())
	return root
}

// scToolEntry is one callable tool with its derived kind.
type scToolEntry struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

type scToolsListOutput struct {
	Tools []scToolEntry `json:"tools"`
}

// toolType derives the tool kind from its URI slot. Tool URIs are
// <provider>:<slot>[(<instance>)]/<method>; the slot names the kind.
// Self-contained string parse (no manifest-api dependency).
func toolType(name string) string {
	colon := strings.IndexByte(name, ':')
	if colon < 0 {
		return "tool"
	}
	rest := name[colon+1:]
	slot := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		slot = rest[:i]
	}
	if p := strings.IndexByte(slot, '('); p >= 0 {
		slot = slot[:p] // strip (instance)
	}
	switch slot {
	case "tasks":
		return "task"
	case "scripts", "script-service":
		return "script"
	default:
		return "tool"
	}
}

func scToolsListCmd() *cobra.Command {
	var typeFilter string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List callable tools with their type (tool / task / script)",
	}
	cmd.Flags().StringVar(&typeFilter, "type", "", "Filter by tool type (tool, task, script)")
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.LLMService](cmd.Context())
		if err != nil {
			return err
		}
		res, err := svc.ListTools(cmd.Context(), api.LLMListToolsInput{})
		if err != nil {
			return err
		}
		out := scToolsListOutput{Tools: make([]scToolEntry, 0, len(res.Tools))}
		for _, t := range res.Tools {
			kind := toolType(t.Name)
			if typeFilter != "" && kind != typeFilter {
				continue
			}
			out.Tools = append(out.Tools, scToolEntry{
				Name:        t.Name,
				Type:        kind,
				Description: t.Description,
				Source:      t.Source,
			})
		}
		return console.RenderConsole(*mode, "sc-tools-list", out, scToolsListTemplate)
	}
	return cmd
}

const scToolsListTemplate = `{{ header "Tools" }}{{ if .Tools }}{{ table .Tools "Type" "Name" "Description" }}{{ dim (printf "%d tool(s)" (count .Tools)) }}{{ else }}{{ dim "(none)" }}{{ end }}`

func scToolsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get NAME",
		Short: "Full JSON schema for one tool",
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
		return console.RenderConsole(*mode, "sc-tools-get", result, toolsDescribeTemplate)
	}
	return cmd
}

func scToolsTypesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "types",
		Short: "List distinct tool types on this binary",
	}
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.LLMService](cmd.Context())
		if err != nil {
			return err
		}
		res, err := svc.ListTools(cmd.Context(), api.LLMListToolsInput{})
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		types := []string{}
		for _, t := range res.Tools {
			k := toolType(t.Name)
			if !seen[k] {
				seen[k] = true
				types = append(types, k)
			}
		}
		sort.Strings(types)
		return console.RenderConsole(*mode, "sc-tools-types", map[string][]string{"types": types}, scToolsTypesTemplate)
	}
	return cmd
}

const scToolsTypesTemplate = `{{ header "Tool Types" }}{{ if .types }}{{ range .types }}{{ dim . }}{{ end }}{{ dim (printf "%d type(s)" (count .types)) }}{{ else }}{{ dim "(none)" }}{{ end }}`
