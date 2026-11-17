// SPDX-License-Identifier: Apache-2.0

package ucfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Finding is one lint result. Severity follows the gocode lint convention
// (error | warn): error-severity findings gate CI and verify pre-flight;
// warn-severity findings are the phased authoring rules (anti-criterion,
// curated description) that flip to error per module once its UCs are
// refined.
type Finding struct {
	Path     string
	UCID     string
	ACID     string
	Severity string // "error" | "warn"
	Rule     string
	Msg      string
}

func (f Finding) String() string {
	loc := f.Path
	if f.ACID != "" {
		loc += "#" + f.ACID
	}
	return fmt.Sprintf("%s [%s/%s] %s", loc, f.Severity, f.Rule, f.Msg)
}

const (
	SeverityError = "error"
	SeverityWarn  = "warn"
)

// Options configures path-dependent checks.
type Options struct {
	// ModuleDir is the module root (modules/<m>) used to resolve
	// path-shaped test_refs (tests/L2/x.tasktest). Empty = skip
	// file-existence checks (pure in-memory lint).
	ModuleDir string

	// AntiRuleError promotes the ≥1-anti-criterion and curated-description
	// rules from warn to error (per-module flip once its UCs are refined).
	AntiRuleError bool
}

// Lint runs every check over the use cases (rebuilt from the use-case and
// criterion objects; Path is the use case's object file). Returns all
// findings; callers split on Severity.
func Lint(files []*UCFile, opts Options) []Finding {
	var out []Finding
	seenUC := map[string]string{} // UC id -> path
	for _, f := range files {
		out = append(out, lintFile(f, opts)...)

		if prev, dup := seenUC[f.Meta.ID]; dup && f.Meta.ID != "" {
			out = append(out, Finding{
				Path: f.Path, UCID: f.Meta.ID, Severity: SeverityError, Rule: "duplicate-uc-id",
				Msg: fmt.Sprintf("UC id %s already declared in %s", f.Meta.ID, prev),
			})
		} else {
			seenUC[f.Meta.ID] = f.Path
		}
	}
	return out
}

func lintFile(f *UCFile, opts Options) []Finding {
	var out []Finding
	m := f.Meta
	err := func(acID, rule, msg string, args ...any) {
		out = append(out, Finding{Path: f.Path, UCID: m.ID, ACID: acID,
			Severity: SeverityError, Rule: rule, Msg: fmt.Sprintf(msg, args...)})
	}
	softSeverity := SeverityWarn
	if opts.AntiRuleError {
		softSeverity = SeverityError
	}
	soft := func(rule, msg string, args ...any) {
		out = append(out, Finding{Path: f.Path, UCID: m.ID,
			Severity: softSeverity, Rule: rule, Msg: fmt.Sprintf(msg, args...)})
	}

	// ── identity ────────────────────────────────────────────────────────
	if !UCIDRe.MatchString(m.ID) {
		err("", "uc-id", "id %q must match UC-NNN", m.ID)
	}
	if m.Kind != KindUseCase {
		err("", "kind", "kind must be %q, got %q", KindUseCase, m.Kind)
	}
	if m.Title == "" {
		err("", "title", "missing title")
	}
	if m.Module == "" {
		err("", "module", "missing module")
	}
	if !ValidSeverities[m.Severity] {
		err("", "severity", "severity %q not in {core,security,qol,cosmetic}", m.Severity)
	}
	if m.StatusCache != "" && !ValidStatuses[m.StatusCache] {
		err("", "status-cache", "status_cache %q not in the 5-state set", m.StatusCache)
	}
	if m.StatusCache != "" && len(m.AcceptanceCriteria) > 0 {
		if want := WorstOf(m.AcceptanceCriteria); m.StatusCache != want {
			out = append(out, Finding{Path: f.Path, UCID: m.ID, Severity: SeverityWarn,
				Rule: "status-cache-drift",
				Msg:  fmt.Sprintf("status_cache %q != worst-of ACs %q (verify is the only writer)", m.StatusCache, want)})
		}
	}

	// ── acceptance criteria ─────────────────────────────────────────────
	if len(m.AcceptanceCriteria) == 0 {
		soft("no-acceptance-criteria", "UC declares no acceptance criteria")
	}
	hasAnti := false
	hasDeferred := false
	seenAC := map[string]bool{}
	for _, ac := range m.AcceptanceCriteria {
		if ac.IsAnti() {
			hasAnti = true
		}
		if ac.Status == StatusFailDeferred {
			hasDeferred = true
		}
		if !ACIDRe.MatchString(ac.ID) {
			err(ac.ID, "ac-id", "AC id %q must match UC-NNN-ACn(.m)", ac.ID)
		} else if m.ID != "" && !strings.HasPrefix(ac.ID, m.ID+"-") {
			err(ac.ID, "ac-id-scope", "AC id %q not scoped to %s", ac.ID, m.ID)
		}
		if seenAC[ac.ID] {
			err(ac.ID, "duplicate-ac-id", "duplicate AC id %q", ac.ID)
		}
		seenAC[ac.ID] = true

		if ac.Claim == "" {
			err(ac.ID, "claim", "missing claim")
		}
		if ac.Kind != "" && ac.Kind != "normal" && ac.Kind != "anti" {
			err(ac.ID, "ac-kind", "kind %q not in {normal,anti}", ac.Kind)
		}
		if !ValidLayers[ac.Layer] {
			err(ac.ID, "layer", "layer %q not in {L1,L2,L3}", ac.Layer)
		}
		if !ValidStatuses[ac.EffectiveStatus()] {
			err(ac.ID, "status", "status %q not in the 5-state set", ac.Status)
		}

		// One binary probe: a test_ref is required unless the criterion is
		// explicitly an open gap (no-test) or an OD-anchored deferral
		// (fail-deferred = intentionally not implemented — no probe exists
		// by definition; the OD link is enforced above).
		if ac.TestRef == "" {
			if st := ac.EffectiveStatus(); st != StatusNoTest && st != StatusFailDeferred {
				err(ac.ID, "probe", "no test_ref — a criterion without a nameable probe is fluff (set status: no-test for an open gap)")
			}
			continue
		}
		out = append(out, lintTestRef(f, m.ID, ac, opts)...)
	}
	if !hasAnti {
		soft("anti-criterion", "UC has no anti-criterion (≥1 kind: anti required — a behavior with no named failure mode is under-specified)")
	}
	if hasDeferred && m.FailDeferredBy == "" {
		err("", "deferred-without-od", "fail-deferred AC present but fail_deferred_by carries no decision id")
	}

	// ── description (phased) ────────────────────────────────────────────
	if d := strings.TrimSpace(m.Description); d == "" || strings.Contains(strings.ToLower(d), "(generated") {
		soft("description", "description missing or still the generated placeholder — state what the UC covers and HOW it is tested")
	}

	return out
}

// lintTestRef statically validates a probe reference per its layer.
// L1 refs are bare Go test names (verify resolves + runs them); L2/L3
// path-shaped refs must exist under the module dir; L3 task-name refs
// (no slash) are accepted verbatim.
func lintTestRef(f *UCFile, ucID string, ac AC, opts Options) []Finding {
	var out []Finding
	err := func(rule, msg string, args ...any) {
		out = append(out, Finding{Path: f.Path, UCID: ucID, ACID: ac.ID,
			Severity: SeverityError, Rule: rule, Msg: fmt.Sprintf(msg, args...)})
	}
	switch ac.Layer {
	case "L1":
		if !L1TestNameRe.MatchString(ac.TestRef) {
			err("test-ref-l1", "L1 test_ref %q is not a Go test name (Test[A-Z]...)", ac.TestRef)
		}
	case "L2":
		if !strings.HasSuffix(ac.TestRef, ".tasktest") {
			err("test-ref-l2", "L2 test_ref %q is not a .tasktest path", ac.TestRef)
		} else if opts.ModuleDir != "" {
			if _, statErr := os.Stat(filepath.Join(opts.ModuleDir, filepath.FromSlash(ac.TestRef))); statErr != nil {
				err("test-ref-l2", "L2 scenario %q not found under %s", ac.TestRef, opts.ModuleDir)
			}
		}
	case "L3":
		if strings.Contains(ac.TestRef, "/") && opts.ModuleDir != "" {
			if _, statErr := os.Stat(filepath.Join(opts.ModuleDir, filepath.FromSlash(ac.TestRef))); statErr != nil {
				err("test-ref-l3", "L3 scenario %q not found under %s", ac.TestRef, opts.ModuleDir)
			}
		}
	}
	return out
}

// Errors filters findings to error severity.
func Errors(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Severity == SeverityError {
			out = append(out, f)
		}
	}
	return out
}

// OrphanTests reports module test names mapped by no AC. allTests is the
// caller-supplied inventory (e.g. extracted from the module's _test.go
// files); the check is a warn — shared/indirect coverage is legitimate.
func OrphanTests(files []*UCFile, allTests []string) []Finding {
	mapped := map[string]bool{}
	for _, f := range files {
		for _, ac := range f.Meta.AcceptanceCriteria {
			if ac.Layer == "L1" && ac.TestRef != "" {
				mapped[ac.TestRef] = true
			}
		}
	}
	var out []Finding
	for _, t := range allTests {
		if !mapped[t] {
			out = append(out, Finding{Severity: SeverityWarn, Rule: "orphan-test",
				Msg: fmt.Sprintf("test %s is mapped by no acceptance criterion", t)})
		}
	}
	return out
}
