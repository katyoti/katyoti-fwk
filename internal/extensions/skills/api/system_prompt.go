// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"sort"
	"strings"
)

// PromptMode selects the verb vocabulary used in the <skills_system> block.
//
// The same skill catalog is reachable two ways:
//   - PromptModeAgent: in-process agent loop, where verbs are tool names invoked
//     by the LLM through the agent runtime (get_skill_instructions, ...).
//   - PromptModeCLI: external LLM driving the binary purely via the shell, where
//     verbs are CLI subcommands (`<binary> sc skills get-instructions <name>`, ...).
//
// Both modes emit the same structural block so a skill behaves identically
// whether driven by an in-binary agent or by an LLM piping the CLI.
//
// Updated 2026-05-23 (SN-13): CLI verbs moved under the `sc` group per the
// the command-surface decision (three command families: domain / sc / llm).
type PromptMode int

const (
	// PromptModeAgent emits agent-runtime tool names.
	PromptModeAgent PromptMode = iota
	// PromptModeCLI emits `<binary> skills <verb>` invocations.
	PromptModeCLI
)

// BuildSystemPrompt produces the <skills_system> block listing how to use
// skills and the available skills. It is the single source of truth shared
// between the in-process skill_access agent plugin and the `skills list` CLI
// subcommand.
//
// binaryName is only used in PromptModeCLI to render concrete invocation
// examples; it is ignored in PromptModeAgent.
func BuildSystemPrompt(skills []SkillInfo, mode PromptMode, binaryName string) string {
	var sb strings.Builder
	sb.WriteString("<skills_system>\n\n")
	sb.WriteString("## What are Skills?\n")
	sb.WriteString("Skills are packages of domain expertise — on-demand knowhow for a class of task. ")
	sb.WriteString("A skill body names things by URI; this block says how to load each kind.\n\n")

	sb.WriteString("## Skill check (MANDATORY, before you start)\n")
	sb.WriteString("Before doing any substantive work on a request, scan the Available Skills list below. ")
	sb.WriteString("If a skill's description matches the request, load that skill FIRST and follow its ")
	sb.WriteString("procedure — its body carries the steps and the format contracts your output must ")
	sb.WriteString("satisfy. Do not improvise from prior knowledge when a matching skill exists; ")
	sb.WriteString("output produced without the skill will likely violate contracts the skill enforces. ")
	sb.WriteString("Proceed without a skill only when nothing applies. The list shows ADVERTISED ")
	sb.WriteString("skills — it is NOT exhaustive: any skill or resource is loadable by its exact ")
	sb.WriteString("URI even when unlisted. So when a skill, task, or reference names another by URI, ")
	sb.WriteString("load it that way — never reach for the filesystem for platform content.\n\n")

	sb.WriteString("## Addressing\n")
	sb.WriteString("Everything is addressed by URI; the byte after `:` tells the kind:\n")
	sb.WriteString("- **resource** (read) — `<provider>:/<type>/<id>` — e.g. `ktw-docs:/skill/doc-create`, `ktw-docs:/reference/diagrams/architecture.md`\n")
	sb.WriteString("- **tool** (call) — `<provider>:<slot>/<method>` — e.g. `ktw-example:example/search`\n")
	sb.WriteString("- **event** (subscribe) — `<provider>:!<scope>/<path>` — e.g. `cwm:!workspace/object/*/created`\n\n")

	switch mode {
	case PromptModeAgent:
		sb.WriteString("Load on this surface:\n")
		sb.WriteString("- a skill — `skills-get-instruction` with `{\"skill_name\": \"<provider>:/skill/<name>\"}`\n")
		sb.WriteString("- any other resource's content (reference, config, data-type, …) — the **read-resource** tool with `{\"uri\": \"<provider>:/<type>/<id>\"}`\n")
		sb.WriteString("- a tool — call it by name with its JSON input\n")
		sb.WriteString("- a task is a tool (`<provider>:tasks/<name>`): call a short task directly; for long-running work dispatch with `delegate` (background mode) and poll `task-status`\n\n")
	case PromptModeCLI:
		bin := binaryName
		if bin == "" {
			bin = "<binary>"
		}
		sb.WriteString("Load on this surface:\n")
		sb.WriteString(fmt.Sprintf("- any resource's content (skill, reference, config, …) — `%s llm resources read <provider>:/<type>/<id>` (add `--into <file>` to save big content to disk instead of printing)\n", bin))
		sb.WriteString(fmt.Sprintf("- a tool — `%s llm tools call <provider>:<slot>/<method> '<json-params>'` (MANDATORY: `%s llm tools describe <tool-uri>` before the first call)\n", bin, bin))
		sb.WriteString(fmt.Sprintf("- list — `%s llm resources list` / `%s llm tools list`\n", bin, bin))
		sb.WriteString("- a task is a tool: call a short task directly; dispatch long-running work in background and poll its status\n\n")
	}

	sb.WriteString("## Available Skills\n")
	if len(skills) > skillListThreshold {
		writeCompactSkillIndex(&sb, skills)
	} else {
		for _, s := range skills {
			sb.WriteString("<skill>\n")
			sb.WriteString(fmt.Sprintf("  <name>%s</name>\n", s.URI()))
			sb.WriteString(fmt.Sprintf("  <description>%s</description>\n", s.Description))
			if len(s.Requires) > 0 {
				sb.WriteString(fmt.Sprintf("  <requires>%s</requires>\n", strings.Join(s.Requires, ", ")))
			}
			sb.WriteString("</skill>\n")
		}
	}
	sb.WriteString("\n</skills_system>")
	return sb.String()
}

// skillListThreshold is the count above which the Available Skills section
// switches from full per-skill <skill> blocks to a compact provider-grouped
// index. A scoped `skills:` task stays well under this (full detail); only a
// broad `skills: ["*"]` agent (chat, builders) crosses it, where the verbose
// XML scaffolding would flood the prompt. The index keeps every skill's name +
// description (the discovery signal) — it just drops the 4-lines-per-skill
// wrapper and groups by provider for scannability. Full instructions still load
// on demand via skills-get-instruction.
const skillListThreshold = 20

// writeCompactSkillIndex renders skills as `- <uri> — <description>` lines,
// grouped under a `### <provider>` header, providers + skills sorted for
// deterministic output.
func writeCompactSkillIndex(sb *strings.Builder, skills []SkillInfo) {
	byProvider := map[string][]SkillInfo{}
	for _, s := range skills {
		byProvider[s.Provider] = append(byProvider[s.Provider], s)
	}
	providers := make([]string, 0, len(byProvider))
	for p := range byProvider {
		providers = append(providers, p)
	}
	sort.Strings(providers)

	fmt.Fprintf(sb, "%d skills available, grouped by provider — load one with `skills-get-instruction` before use.\n", len(skills))
	for _, p := range providers {
		group := byProvider[p]
		sort.Slice(group, func(i, j int) bool { return group[i].Name < group[j].Name })
		fmt.Fprintf(sb, "### %s\n", p)
		for _, s := range group {
			line := fmt.Sprintf("- `%s` — %s", s.URI(), s.Description)
			if len(s.Requires) > 0 {
				line += fmt.Sprintf(" (requires: %s)", strings.Join(s.Requires, ", "))
			}
			sb.WriteString(line + "\n")
		}
	}
}
