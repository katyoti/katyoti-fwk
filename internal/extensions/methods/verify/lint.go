// SPDX-License-Identifier: Apache-2.0

package verify

// LintResult is the OUTPUT contract every linter conforms to — the one thing
// that makes an action "a linter". A linter is any runnable (a KTSP method, a
// script, or a task — dispatched by URI through ActionService) whose result
// decodes to this shape. Its INPUT is the runnable's own schema, populated by
// the declaring node type through el (`input: { skill_file: "${node.skill_file}" }`).
//
// This is artifact-AGNOSTIC: methods owns the contract + neutral basic linters;
// a pack (ktw-compose) contributes its own (skill-format, task-format) without
// methods ever learning what a "skill" is.
type LintResult struct {
	// Status is the linter's overall verdict: "pass" (no blocking findings),
	// "fail" (at least one blocking finding), or "error" (the linter could not
	// run — a missing target, a malformed input).
	Status string `json:"status"`
	// Findings are the individual issues, each with a severity so the GATE can
	// decide which block `done`/`verified` (an error blocks; a warn/info informs).
	Findings []LintFinding `json:"findings,omitempty"`
}

// LintSeverity ranks a finding. The gate policy maps severities to "blocks".
type LintSeverity string

const (
	SeverityError LintSeverity = "error" // a contract violation — blocks by default
	SeverityWarn  LintSeverity = "warn"  // a quality smell — surfaced, does not block by default
	SeverityInfo  LintSeverity = "info"  // advisory
)

// LintFinding is one issue a linter reports.
type LintFinding struct {
	Rule     string       `json:"rule"`     // stable code, e.g. "frontmatter-missing"
	Message  string       `json:"message"`  // human-readable detail
	Severity LintSeverity `json:"severity"` // error | warn | info
}

// ResourceFinding is a LintFinding located at a published resource: the
// URI of the entry it concerns. It is the one shape every content lint
// (manifest, skills, tasks) reports and every consumer (ktw-compose verbs,
// the census) reads; it lives here, in the neutral pack, so an api package
// never has to import a lint package to name it.
type ResourceFinding struct {
	URI string `json:"uri"`
	LintFinding
}

func (f ResourceFinding) String() string {
	return f.URI + ": [" + string(f.Severity) + "/" + f.Rule + "] " + f.Message
}

// Result builds a LintResult from findings, deriving status: "fail" if any
// error-severity finding is present, else "pass". A linter that cannot run
// returns ErrorResult instead.
func Result(findings ...LintFinding) LintResult {
	status := "pass"
	for _, f := range findings {
		if f.Severity == SeverityError {
			status = "fail"
			break
		}
	}
	return LintResult{Status: status, Findings: findings}
}

// ErrorResult is the "could not run" verdict (a missing target etc.).
func ErrorResult(rule, message string) LintResult {
	return LintResult{Status: "error", Findings: []LintFinding{{Rule: rule, Message: message, Severity: SeverityError}}}
}
