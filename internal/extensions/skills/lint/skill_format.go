// SPDX-License-Identifier: Apache-2.0

package skilllint

import (
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
	skillsapi "github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
)

// skill-format is the SKILL file contract as a lint (skill-format.md). It
// lives with the kind it describes — the skills pack owns what a SKILL is —
// in a package of its own that only ktw-compose and test code import: a
// sidecar's census lints its skills at build without linking one extra
// package into the sidecar's executable, and ktw-compose lints any sidecar's
// skills on demand from its published manifest.
//
// The lint reads a skill through the SAME parser the skill provider uses
// (`skillsapi.ParseSkillMD`, strict): what the provider would drop, the lint
// refuses for the same reason, and the rules below judge the parsed
// frontmatter and body — never a second decode that could disagree.

// allowedSkillKeys is the strict frontmatter contract: the keys of the skill
// data-type (the shape of mbapi.SkillBody), read off the typed body rather than restated
// here — a second copy of the list is how a contract drifts.
var allowedSkillKeys = mbapi.SkillFrontmatterKeys()

// taskShapeKeys are the task-frontmatter fields that betray a runnable
// inputs→output artifact mis-authored as a skill (the artifact-TYPE defect).
var taskShapeKeys = map[string]bool{
	"input-schema": true, "output-schema": true, "config": true, "type": true,
}

var (
	reSkillKTSP        = regexp.MustCompile(`\bKTSP\b`)
	reSkillMetaComment = regexp.MustCompile(`(?i)there (are|is) no\b|\bno new\b`)
	reSkillWhenToUse   = regexp.MustCompile(`(?im)^#{1,6}\s+when to use\b`)
	reSkillSentence    = regexp.MustCompile(`[.!?](\s|$)`)
	// surface-specific loaders: a skill body must name the URI with the canonical
	// verb (get-instruction / read / call) and let the host bind it. These are
	// agent-tool / CLI-subcommand forms — false on the other surfaces.
	reSkillSurfaceLoader = regexp.MustCompile(`(?i)\bskills?[-_ ]get[-_ ]instructions?\b|\bget_skill_instructions?\b|\b(?:sc|llm|ktw-[a-z]+)\s+(?:resources|skills|tools)\s+(?:read|list|call|describe|get-instructions?)\b|\bresources[-/]read\b`)
	reUnknownField       = regexp.MustCompile(`field (\S+) not found`)
)

// descMaxChars: the L0 description is the only thing the LLM sees before
// loading; it must be a short what + when (one or two sentences, per
// skill-format's "one sentence where possible"). Beyond this it is over-long.
const descMaxChars = 200

// LintSkillFormat checks a skill file against the skill-format contract and
// returns the neutral verify.LintResult.
func LintSkillFormat(path string) verify.LintResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return verify.ErrorResult("unreadable", "skill-format: cannot read "+path)
	}
	return LintSkillFormatBytes(data)
}

// LintSkillFormatBytes is LintSkillFormat over content already in hand — an
// embedded file, a served body — so a sidecar lints itself without a path.
func LintSkillFormatBytes(data []byte) verify.LintResult {
	var findings []verify.LintFinding
	md, err := skillsapi.ParseSkillMD(data)
	if err != nil {
		// The provider would drop this file. Say why — and still judge the
		// body, so the author fixes everything in one pass.
		findings = append(findings, parseFindings(data, err)...)
		_, body, _ := verify.SplitFrontmatter(string(data))
		return verify.Result(append(findings, bodyFindings(body, "")...)...)
	}
	desc := md.Frontmatter.Description
	if len(desc) > descMaxChars {
		findings = append(findings, verify.LintFinding{Rule: "description-too-long", Message: "description is " + strconv.Itoa(len(desc)) + " chars; keep it to one short sentence (≤" + strconv.Itoa(descMaxChars) + ")", Severity: verify.SeverityWarn})
	}
	if skillSentences(desc) > 2 {
		findings = append(findings, verify.LintFinding{Rule: "description-multi-sentence", Message: "description is more than two sentences; keep it to a what + when pair (≤2) — the L0 selector", Severity: verify.SeverityWarn})
	}
	return verify.Result(append(findings, bodyFindings(md.Instructions, md.Frontmatter.Name)...)...)
}

// bodyFindings are the rules on the markdown body; `name` is the skill's
// name when the frontmatter parsed ("" otherwise).
func bodyFindings(body, name string) []verify.LintFinding {
	var findings []verify.LintFinding
	// A word, not a broken contract: surfaced, never blocking (was an error
	// until 2026-09-22, when a build gate over 328 skills would have failed
	// fifteen of them for a protocol name in a table cell).
	if reSkillKTSP.MatchString(body) {
		findings = append(findings, verify.LintFinding{Rule: "internal-jargon", Message: `body uses internal jargon "KTSP" — meaningless out of context; name the capability, not the protocol`, Severity: verify.SeverityWarn})
	}
	if reSkillMetaComment.MatchString(body) {
		findings = append(findings, verify.LintFinding{Rule: "meta-commentary", Message: `body states what is NOT there ("there are no…"); say only what must be done`, Severity: verify.SeverityWarn})
	}
	if reSkillWhenToUse.MatchString(body) {
		findings = append(findings, verify.LintFinding{Rule: "when-to-use-section", Message: "a `When to use` body section duplicates the description; the trigger belongs in the L0 description", Severity: verify.SeverityWarn})
	}
	// The per-binary `usage` skill is the one skill whose SUBJECT is a surface:
	// it tells an LLM driving the CLI which commands exist. Naming them there
	// is the point, not a defect.
	if m := reSkillSurfaceLoader.FindString(body); m != "" && name != "usage" {
		findings = append(findings, verify.LintFinding{Rule: "surface-specific-invocation", Message: `body names a surface-specific loader ("` + strings.TrimSpace(m) + `") — name the URI with the canonical verb (get-instruction a skill, read a resource, call a tool) and let the host bind it; the surface form is false on CLI/MCP`, Severity: verify.SeverityError})
	}
	return findings
}

// parseFindings turns the provider parser's refusal into the lint's rules,
// and — because a strict decoder stops at the first unknown key — lists every
// extra key from a relaxed read so the author fixes them in one pass.
func parseFindings(data []byte, err error) []verify.LintFinding {
	msg := err.Error()
	fm, _, ok := verify.SplitFrontmatter(string(data))
	if !ok {
		return []verify.LintFinding{{Rule: "frontmatter-missing", Message: "no opening `---` YAML frontmatter block", Severity: verify.SeverityError}}
	}
	var m map[string]any
	if yerr := yaml.Unmarshal([]byte(fm), &m); yerr != nil || m == nil {
		return []verify.LintFinding{{Rule: "frontmatter-invalid", Message: "frontmatter is not valid YAML: " + msg, Severity: verify.SeverityError}}
	}
	var out []verify.LintFinding
	typeShaped := false
	for k := range m {
		switch {
		case taskShapeKeys[k]:
			if !typeShaped {
				out = append(out, verify.LintFinding{Rule: "skill-is-task-shaped", Message: "frontmatter declares `" + k + "` — a runnable inputs→output artifact is a TASK, not a skill; author it as a task", Severity: verify.SeverityError})
				typeShaped = true
			}
		case !allowedSkillKeys[k]:
			out = append(out, verify.LintFinding{Rule: "frontmatter-extra-key", Message: "frontmatter key not in the skill contract: " + k, Severity: verify.SeverityError})
		}
	}
	switch {
	case strings.Contains(msg, "missing required field 'description'"):
		out = append(out, verify.LintFinding{Rule: "description-missing", Message: "no description (the L0 selector)", Severity: verify.SeverityError})
	case reUnknownField.MatchString(msg) && len(out) > 0:
		// already listed above, from the relaxed read
	default:
		out = append(out, verify.LintFinding{Rule: "frontmatter-invalid", Message: "the skill provider refuses this file: " + msg, Severity: verify.SeverityError})
	}
	return out
}

func skillSentences(s string) int {
	return len(reSkillSentence.FindAllString(strings.TrimSpace(s), -1))
}
