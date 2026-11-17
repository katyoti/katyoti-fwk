// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// TestOutcome captures the result of one named test extracted from a batched
// `go test` invocation.
type TestOutcome struct {
	Name     string
	Status   TestStatus
	Duration time.Duration
	// Output is the slice of `go test` output relevant to this test
	// (failure body, error messages). Empty for passes.
	Output string
}

// TestStatus enumerates the per-test outcomes the runner observes.
type TestStatus int

const (
	StatusUnknown TestStatus = iota
	StatusPass
	StatusFail
	StatusSkip
	StatusNoMatch // `go test` ran but found no test by that name
	StatusError   // process error (couldn't build, harness crashed, etc.)
)

func (s TestStatus) String() string {
	switch s {
	case StatusPass:
		return "pass"
	case StatusFail:
		return "fail"
	case StatusSkip:
		return "skip"
	case StatusNoMatch:
		return "no-match"
	case StatusError:
		return "error"
	default:
		return "unknown"
	}
}

// L1Runner shells out `go test` to verify a list of named tests across the
// module's source tree. Names are batched into a single invocation per Run
// call — the per-process startup cost dominates per-test wall-clock, so
// batching delivers a 5-10× speedup over per-name invocations.
type L1Runner struct {
	// SourceRoots scopes the `./<path>/...` package set the runner walks.
	// Caller resolves these from manifest.json's source_paths.
	SourceRoots []string

	// WorkingDir is the directory `go test` runs from. Typically the
	// repository root so relative source roots resolve correctly.
	WorkingDir string

	// Timeout caps the batched invocation. Defaults to 5min — enough for the
	// full tasks-charter suite under wildcard package walks.
	Timeout time.Duration
}

// Run executes one `go test` with the OR-pattern of all input names against
// the configured source roots. Returns one TestOutcome per input name in
// input order. Names not found in the scoped run get StatusNoMatch — that
// signals charter↔code drift (a test name in the Test plan doesn't resolve)
// which is intentional: the verify command's job is to surface drift.
//
// Empty input returns an empty slice without invoking `go test`.
func (r *L1Runner) Run(ctx context.Context, names []string) []TestOutcome {
	if len(names) == 0 {
		return nil
	}

	parsed, runErr := r.exec(ctx, names, r.SourceRoots)

	result := make([]TestOutcome, 0, len(names))
	for _, name := range names {
		if outcome, ok := parsed[name]; ok {
			result = append(result, outcome)
			continue
		}
		o := TestOutcome{Name: name}
		if runErr != nil && len(parsed) == 0 {
			o.Status = StatusError
		} else {
			o.Status = StatusNoMatch
		}
		result = append(result, o)
	}
	return result
}

// exec runs a single batched `go test` against the given roots and returns
// the parsed per-test outcomes (keyed by test name).
func (r *L1Runner) exec(ctx context.Context, names, roots []string) (map[string]TestOutcome, error) {
	if len(names) == 0 || len(roots) == 0 {
		return map[string]TestOutcome{}, nil
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"test", "-run", buildOrPattern(names), "-count=1", "-v"}
	args = append(args, roots...)

	cmd := exec.CommandContext(runCtx, "go", args...)
	if r.WorkingDir != "" {
		cmd.Dir = r.WorkingDir
	}
	combined, err := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return map[string]TestOutcome{}, fmt.Errorf("timed out after %s", timeout)
	}
	return parseTestOutput(string(combined)), err
}

// buildOrPattern produces an anchored alternation regex matching exactly the
// listed test names. Names are quoted so `+`, `_`, etc. don't break the regex.
func buildOrPattern(names []string) string {
	if len(names) == 1 {
		return "^" + regexp.QuoteMeta(names[0]) + "$"
	}
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = regexp.QuoteMeta(n)
	}
	return "^(" + strings.Join(quoted, "|") + ")$"
}

// goTestResultRe matches `--- PASS: TestName (0.01s)` etc. Captures status,
// name, duration. Anchored to start-of-line.
var goTestResultRe = regexp.MustCompile(`(?m)^\s*--- (PASS|FAIL|SKIP): (\S+) \(([\d.]+)s\)`)

// parseTestOutput extracts per-test outcomes from a batched `go test -v`
// run. Lines like `--- PASS: TestX (0.01s)` are the canonical signal. For
// failures, the body lines after the marker (until the next `=== RUN` or
// `--- ...`) are captured as the failure output.
func parseTestOutput(output string) map[string]TestOutcome {
	out := map[string]TestOutcome{}
	matches := goTestResultRe.FindAllStringSubmatchIndex(output, -1)
	for i, m := range matches {
		// m = [start, end, statusStart, statusEnd, nameStart, nameEnd, durStart, durEnd]
		status := output[m[2]:m[3]]
		name := output[m[4]:m[5]]
		dur := output[m[6]:m[7]]

		o := TestOutcome{Name: name}
		switch status {
		case "PASS":
			o.Status = StatusPass
		case "FAIL":
			o.Status = StatusFail
		case "SKIP":
			o.Status = StatusSkip
		}
		if d, err := time.ParseDuration(dur + "s"); err == nil {
			o.Duration = d
		}

		// Failure body: go test prints the t.Errorf / t.Fatalf lines BEFORE
		// the `--- FAIL: ...` marker (between the previous result line and
		// this one). Capture that span.
		if o.Status == StatusFail {
			bodyStart := 0
			if i > 0 {
				bodyStart = matches[i-1][1]
			}
			bodyEnd := m[0]
			body := strings.TrimSpace(output[bodyStart:bodyEnd])
			// Strip the leading `=== RUN`/`=== PAUSE`/`=== CONT` lines so
			// the captured body is just the assertion output.
			var kept []string
			for _, line := range strings.Split(body, "\n") {
				t := strings.TrimSpace(line)
				if strings.HasPrefix(t, "=== ") {
					continue
				}
				kept = append(kept, line)
			}
			o.Output = truncate(strings.TrimSpace(strings.Join(kept, "\n")), 4000)
		}
		out[name] = o
	}
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n... [truncated %d bytes]", len(s)-max)
}

// ── Per-UC rollup ────────────────────────────────────────────────────

// CombinedOutcome reduces per-test outcomes for one UC into a single rollup.
// Reports the worst observed status (Error > Fail > NoMatch > Skip > Pass).
type CombinedOutcome struct {
	UCID       string
	Tests      []TestOutcome
	Status     TestStatus
	TotalTime  time.Duration
	FailReason string
}

// Combine merges per-test outcomes for one UC into a single rollup.
func Combine(ucID string, outcomes []TestOutcome) CombinedOutcome {
	c := CombinedOutcome{UCID: ucID, Tests: outcomes}
	if len(outcomes) == 0 {
		c.Status = StatusUnknown
		return c
	}
	severity := func(s TestStatus) int {
		switch s {
		case StatusError:
			return 5
		case StatusFail:
			return 4
		case StatusNoMatch:
			return 3
		case StatusSkip:
			return 2
		case StatusPass:
			return 1
		}
		return 0
	}
	worst := outcomes[0]
	for _, o := range outcomes {
		c.TotalTime += o.Duration
		if severity(o.Status) > severity(worst.Status) {
			worst = o
		}
	}
	c.Status = worst.Status
	if c.Status == StatusFail || c.Status == StatusError {
		c.FailReason = worst.Name + ": " + firstNonEmptyLine(worst.Output)
	}
	return c
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t != "" {
			return t
		}
	}
	return ""
}
