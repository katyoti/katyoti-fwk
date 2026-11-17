// SPDX-License-Identifier: Apache-2.0

// Package commands provides the `skills` Cobra command for any binary that
// imports the skills extension. The command lets an LLM drive a binary purely
// from the shell — discover skills, read instructions, fetch reference files,
// run scripts — without ever loading the binary as an agent plugin.
package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
	"github.com/spf13/cobra"
)

// NewSkillsCommand returns the `skills` Cobra root with its 4 subcommands.
// It is contributed once by skillslocal.Extension(), so any binary that
// imports the skills extension automatically gains this CLI surface.
func NewSkillsCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "skills",
		Short: "Discover and use skills from the command line",
		Long: `Discover and use skills purely from the command line.

Skills are packages of domain expertise (instructions, references, scripts).
This command lets an LLM (or a human) discover and invoke them via shell calls,
mirroring the in-process agent loop's skill_access plugin.

Run "skills list" first — it emits a self-describing block listing the
available skills and how to invoke them.`,
		SilenceUsage: true,
	}

	root.AddCommand(newListCommand())
	root.AddCommand(newGetInstructionsCommand())
	// `get-reference` and `run-script` were dropped 2026-05-24 —
	// references are first-class Contributions addressed via
	// `<binary> sc resources get <provider>:/reference/<path>`
	// (or `llm resources get <uri>` from the LLM surface). Standalone
	// scripts live as `<provider>:scripts/<name>` slots.
	return root
}

// newListCommand emits skills in two shapes:
//   - rich/raw (default): the `<skills_system>` prompt block the agent loop
//     injects, with verbs rewritten as CLI subcommands so an external LLM
//     piping the binary can self-bootstrap.
//   - json (--out json): a structured `{skills: [{name, description, hashtags, category, advertised}]}`
//     payload for scripts and tooling that need a machine-readable list.
func newListCommand() *cobra.Command {
	var filter []string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List skills (LLM system block by default; --out json for structured)",
		Long: `Default output (rich/raw): emits a <skills_system> block describing how
to use skills via this binary's CLI and listing all available skills with
their descriptions. Same prompt block the in-process agent loop injects.

With --out json: emits a structured {skills: [...]} payload — name,
description, hashtags, category, advertised flag per entry. Use this
mode for scripts and operator tooling that need to consume the skill
catalog without parsing the prompt block.`,
		SilenceUsage: true,
	}
	cmd.Flags().StringSliceVar(&filter, "filter", nil, "Optional skill name filter (repeatable)")
	outMode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// Catalog-backed enumeration — works in any binary that
		// imports `manifestlocal.Extension()` (every sidecar does:
		// each publishes a self-only catalog with its own manifest;
		// ktw-agents overwrites with an aggregated catalog at
		// Bootstrap). Per `bus-split-aggregator-to-catalog` step 12.
		cat, err := application.UseService[mbapi.Catalog](cmd.Context())
		if err != nil || cat == nil {
			return fmt.Errorf("resolve Catalog: %w", err)
		}
		skills, err := api.SkillsFromCatalog(cmd.Context(), cat)
		if err != nil {
			return fmt.Errorf("list skills: %w", err)
		}
		// Apply filter (legacy SkillManager.List supported a name
		// allow-list; preserve the same semantics).
		if len(filter) > 0 {
			want := make(map[string]bool, len(filter))
			for _, f := range filter {
				want[f] = true
			}
			kept := skills[:0]
			for _, s := range skills {
				if want[s.Name] {
					kept = append(kept, s)
				}
			}
			skills = kept
		}
		if *outMode == console.OutputJSON {
			payload := map[string]any{"skills": skills}
			data, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		}
		binary := binaryName(cmd)
		fmt.Fprintln(cmd.OutOrStdout(), api.BuildSystemPrompt(skills, api.PromptModeCLI, binary))
		return nil
	}
	return cmd
}

func newGetInstructionsCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "get-instructions <skill>",
		Short:        "Print the full instructions (SKILL.md body) for a skill",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Catalog-served — instructions live in SkillBody.Instructions
			// after step 13 of bus-split-aggregator-to-catalog.
			cat, err := application.UseService[mbapi.Catalog](cmd.Context())
			if err != nil || cat == nil {
				return fmt.Errorf("resolve Catalog: %w", err)
			}
			instructions, err := api.InstructionsFromCatalog(cmd.Context(), cat, args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), instructions)
			return nil
		},
	}
}

func parseParams(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(raw))
	for _, kv := range raw {
		idx := strings.IndexByte(kv, '=')
		if idx <= 0 {
			return nil, fmt.Errorf("invalid --param %q (expected key=value)", kv)
		}
		out[kv[:idx]] = kv[idx+1:]
	}
	return out, nil
}

// binaryName returns the root command's name, used to render concrete CLI
// invocations in the LLM-mode prompt block.
func binaryName(cmd *cobra.Command) string {
	root := cmd
	for root.Parent() != nil {
		root = root.Parent()
	}
	return root.Name()
}
