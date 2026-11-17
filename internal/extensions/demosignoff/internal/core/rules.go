// SPDX-License-Identifier: Apache-2.0

// Package core is the demo sign-off service: the rules of a sign-off, read from
// an object's `signoff` group and its type's `signoff:` section, and the
// service that answers the capability's hooks, function, panels and acts over
// the CWMService in its context.
package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/demosignoff/api"
)

// GroupOf is the object's sign-off group, empty when it carries none.
func GroupOf(obj cwmapi.Object) api.SignoffGroup {
	var g api.SignoffGroup
	raw, ok := obj.Capability(api.Capability)
	if !ok {
		return g
	}
	b, err := json.Marshal(raw)
	if err == nil {
		_ = json.Unmarshal(b, &g)
	}
	return g
}

// SetGroup writes g as the object's sign-off group.
func SetGroup(obj *cwmapi.Object, g api.SignoffGroup) {
	b, _ := json.Marshal(g)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if obj.Capabilities == nil {
		obj.Capabilities = map[string]map[string]any{}
	}
	obj.Capabilities[api.Capability] = m
}

// SettingsOf is a type's sign-off section, the defaults when it writes none.
func SettingsOf(t cwmapi.ObjectType) api.SignoffSettings {
	s, _, err := cwmapi.TypeSection[api.SignoffSettings](t, api.Capability)
	if err != nil {
		return api.SignoffSettings{}
	}
	return s
}

// Approvals are the distinct approvals given by one of the reviewers, sorted.
func Approvals(g api.SignoffGroup) []string {
	reviewer := map[string]bool{}
	for _, r := range g.Reviewers {
		reviewer[strings.TrimSpace(r)] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, a := range g.Approvals {
		a = strings.TrimSpace(a)
		if a != "" && reviewer[a] && !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

// SignedOff reports whether the sign-off has the approvals its type requires.
func SignedOff(g api.SignoffGroup, s api.SignoffSettings) bool {
	return len(Approvals(g)) >= s.Required()
}

// CheckWrite is the before_update rule: an approved verdict needs the
// approvals the type requires.
func CheckWrite(g api.SignoffGroup, s api.SignoffSettings) cwmapi.HookAccept {
	if g.Verdict == api.VerdictApproved && !SignedOff(g, s) {
		return cwmapi.HookAccept{Reason: fmt.Sprintf(
			"an approved verdict needs %d approval(s) from its reviewers; it has %d", s.Required(), len(Approvals(g)))}
	}
	return cwmapi.HookAccept{Accept: true}
}

// Lint is what a sign-off still lacks, as findings; never a verdict.
func Lint(g api.SignoffGroup) api.LintOutput {
	out := api.LintOutput{Status: "ok", Findings: []api.LintFinding{}}
	if len(g.Reviewers) == 0 {
		out.Findings = append(out.Findings, api.LintFinding{Rule: api.RuleNoReviewer, Severity: "warn",
			Message: "nobody reviews this object: name its reviewers"})
	}
	if g.Verdict == api.VerdictChanges && strings.TrimSpace(g.Notes) == "" {
		out.Findings = append(out.Findings, api.LintFinding{Rule: api.RuleChangesWithoutNotes, Severity: "error",
			Message: "changes are asked without notes saying which"})
	}
	given := map[string]bool{}
	for _, a := range Approvals(g) {
		given[a] = true
	}
	for _, a := range g.Approvals {
		if a = strings.TrimSpace(a); a != "" && !given[a] {
			out.Findings = append(out.Findings, api.LintFinding{Rule: api.RuleApprovalNotReviewer, Severity: "warn",
				Message: fmt.Sprintf("%s approved but is not among the reviewers", a)})
		}
	}
	if len(out.Findings) > 0 {
		out.Status = "findings"
	}
	return out
}

// Verify is the verdict of a rule: approvals (the capability's) or notes (the
// signoff-has-notes policy's).
func Verify(g api.SignoffGroup, s api.SignoffSettings, rule string) api.VerifyOutput {
	switch rule {
	case "notes":
		if strings.TrimSpace(g.Notes) == "" {
			return api.VerifyOutput{Status: api.StatusFail, Evidence: "the sign-off has no notes"}
		}
		return api.VerifyOutput{Status: api.StatusPass, Evidence: "the sign-off has its notes"}
	case "", "approvals":
		switch {
		case g.Verdict == api.VerdictChanges:
			return api.VerifyOutput{Status: api.StatusFail, Evidence: "the reviewers asked for changes"}
		case g.Verdict == api.VerdictApproved && SignedOff(g, s):
			return api.VerifyOutput{Status: api.StatusPass, Evidence: fmt.Sprintf("approved by %s", strings.Join(Approvals(g), ", "))}
		default:
			return api.VerifyOutput{Status: api.StatusUnproven, Evidence: fmt.Sprintf(
				"%d of %d approval(s), verdict %q", len(Approvals(g)), s.Required(), verdictOrPending(g.Verdict))}
		}
	}
	return api.VerifyOutput{Status: api.StatusUnproven, Evidence: fmt.Sprintf("no rule %q", rule)}
}

// State is where the sign-off stands: its approvals against the type's
// requirement, and its verdict.
func State(g api.SignoffGroup, s api.SignoffSettings) string {
	return fmt.Sprintf("%d/%d approval(s), verdict %s", len(Approvals(g)), s.Required(), verdictOrPending(g.Verdict))
}

// Digest is the sign-off in one line: its state and how many contest it.
func Digest(g api.SignoffGroup, s api.SignoffSettings, contests int) string {
	return fmt.Sprintf("%s, %d contest(s)", State(g, s), contests)
}

func verdictOrPending(v string) string {
	if v == "" {
		return api.VerdictPending
	}
	return v
}

// VerdictColor is the colour a verdict is drawn in.
func VerdictColor(v string) string {
	switch v {
	case api.VerdictApproved:
		return "green"
	case api.VerdictChanges:
		return "orange"
	}
	return "gray"
}
