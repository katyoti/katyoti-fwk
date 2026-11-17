// SPDX-License-Identifier: Apache-2.0

package tasklint

import (
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	taskcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib"
)

// task-format is the TASK file contract as a lint (task-format.md) — the
// sibling of the skills pack's skill-format. It lives with the kind it
// describes, in a package only ktw-compose and test code import — nothing
// extra links into a sidecar's executable; the census lints at build, compose
// on demand.
//
// The lint reads a task through the SAME parser the task provider uses
// (`taskcontrib.Parse`, strict): what the provider would drop, the lint
// refuses for the same reason; the rules below judge the parsed task.

// runtimeConfigKeys belong under `config:`, never at root frontmatter.
var runtimeConfigKeys = map[string]bool{"plugins": true, "skills": true, "tools": true, "mode": true}

// ValidTaskTypes is the closed set of `type:` values a TASK file may declare:
// the machine strategies a task can be run by, under their canonical names.
// `simulator` is the scripted, token-free strategy
// (`internal/extensions/simulator`); it was missing here and the lint refused
// seven live tasks (2026-09-22). `agent-loop` is the strategy's internal
// name; the file says `agent`.
var ValidTaskTypes = map[string]bool{"agent": true, "task": true, "workflow": true, "simulator": true}

var (
	reTaskKTSP       = regexp.MustCompile(`\bKTSP\b`)
	reTaskUnknownKey = regexp.MustCompile(`unknown frontmatter key "([^"]+)"`)
)

// LintTaskFormat checks a TASK file against the task-format contract.
func LintTaskFormat(path string) verify.LintResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return verify.ErrorResult("unreadable", "task-format: cannot read "+path)
	}
	return LintTaskFormatBytes(data)
}

// LintTaskFormatBytes is LintTaskFormat over content already in hand.
func LintTaskFormatBytes(data []byte) verify.LintResult {
	fm, body, ok := verify.SplitFrontmatter(string(data))
	if !ok {
		return verify.Result(verify.LintFinding{Rule: "frontmatter-missing", Message: "no opening `---` YAML frontmatter block", Severity: verify.SeverityError})
	}
	// The raw map carries the static rules whether or not the provider
	// accepts the file, so the author sees every finding in one pass.
	var raw map[string]any
	if yerr := yaml.Unmarshal([]byte(fm), &raw); yerr != nil || raw == nil {
		return verify.Result(verify.LintFinding{Rule: "frontmatter-invalid", Message: "frontmatter is not valid YAML", Severity: verify.SeverityError})
	}
	var findings []verify.LintFinding
	if _, err := taskcontrib.Parse(data); err != nil {
		findings = append(findings, parseFindings(fm, err)...)
	}
	typ, _ := raw["type"].(string)
	if typ != "" && !ValidTaskTypes[typ] {
		findings = append(findings, verify.LintFinding{Rule: "type-invalid", Message: "type must be agent | task | workflow | simulator, got: " + typ, Severity: verify.SeverityError})
	}
	if kind, _ := raw["kind"].(string); kind == taskapi.KindObject || kind == taskapi.KindCollection {
		if _, has := raw["types"]; !has {
			findings = append(findings, verify.LintFinding{Rule: "object-kind-without-types", Message: "`kind: " + kind + "` requires `types:` (the CWM object types this task targets)", Severity: verify.SeverityError})
		}
	}
	// `mode: chat` filters out the `complete` tool (the chat preset — the user
	// owns session lifetime), so a chat agent must NOT be told to call it.
	mode := ""
	if cfg, ok := raw["config"].(map[string]any); ok {
		mode, _ = cfg["mode"].(string)
	}
	if typ == "agent" && mode != "chat" && !strings.Contains(body, "complete") {
		findings = append(findings, verify.LintFinding{Rule: "agent-no-complete", Message: `an agent prompt must instruct "Call complete when done" — every non-chat agent must complete`, Severity: verify.SeverityWarn})
	}
	// The old form, still read by the engine for task folders outside the
	// repository: the contracts belong in files beside the task, the keys
	// take the names the catalog publishes.
	for _, o := range []struct{ key, instead string }{
		{"input-schema", "a file beside the task: <name>.input.dsl.yaml (TASK.input.dsl.yaml in a folder task)"},
		{"output-schema", "a file beside the task: <name>.output.dsl.yaml (TASK.output.dsl.yaml in a folder task)"},
		{"permission_profile", "permissionProfile"},
		{"session", "sessionMode: <mode>"},
	} {
		key, instead := o.key, o.instead
		if _, old := raw[key]; old {
			findings = append(findings, verify.LintFinding{Rule: "task-old-form", Message: "`" + key + ":` is the old form — write " + instead, Severity: verify.SeverityWarn})
		}
	}
	if reTaskKTSP.MatchString(body) {
		findings = append(findings, verify.LintFinding{Rule: "internal-jargon", Message: `body uses internal jargon "KTSP" — meaningless out of context`, Severity: verify.SeverityWarn})
	}
	return verify.Result(findings...)
}

// parseFindings turns the provider parser's refusal into the lint's rules:
// a runtime key at root gets its own diagnosis, an unknown key is listed, a
// missing required key named; anything else is the parser's own words.
func parseFindings(fm string, err error) []verify.LintFinding {
	msg := err.Error()
	var m map[string]any
	if yerr := yaml.Unmarshal([]byte(fm), &m); yerr != nil || m == nil {
		return []verify.LintFinding{{Rule: "frontmatter-invalid", Message: "frontmatter is not valid YAML: " + msg, Severity: verify.SeverityError}}
	}
	var out []verify.LintFinding
	for k := range m {
		if runtimeConfigKeys[k] {
			out = append(out, verify.LintFinding{Rule: "runtime-config-at-root", Message: "`" + k + "` is runtime config — move it under `config:`, not root frontmatter", Severity: verify.SeverityError})
		}
	}
	switch {
	case strings.Contains(msg, "missing required field"):
		key := "name"
		if strings.Contains(msg, "'type'") {
			key = "type"
		}
		out = append(out, verify.LintFinding{Rule: "required-key-missing", Message: "required frontmatter key missing: " + key, Severity: verify.SeverityError})
	case strings.Contains(msg, "requires non-empty node_types"):
		// reported by the static rule above
	default:
		if km := reTaskUnknownKey.FindStringSubmatch(msg); km != nil {
			if !runtimeConfigKeys[km[1]] {
				out = append(out, verify.LintFinding{Rule: "frontmatter-extra-key", Message: "frontmatter key not in the task contract: " + km[1], Severity: verify.SeverityError})
			}
		} else {
			out = append(out, verify.LintFinding{Rule: "frontmatter-invalid", Message: "the task provider refuses this file: " + msg, Severity: verify.SeverityError})
		}
	}
	if _, has := m["description"]; !has {
		out = append(out, verify.LintFinding{Rule: "required-key-missing", Message: "required frontmatter key missing: description", Severity: verify.SeverityError})
	}
	return out
}
