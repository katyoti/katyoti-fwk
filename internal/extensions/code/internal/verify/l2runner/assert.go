// SPDX-License-Identifier: Apache-2.0

package l2runner

import (
	"fmt"
	"regexp"
	"strings"
)

// checkAssertions evaluates every Phase C.1 assertion against the captured
// process result. Returns a list of human-readable failure descriptions —
// empty list = all assertions held.
func checkAssertions(scenario *Scenario, res *Result) []string {
	if res.Verdict == VerdictError {
		return nil // process error — assertion check is moot
	}
	a := scenario.Assertions
	var failures []string

	// expect_exit (top-level on assertions; also accepted on invoke for legacy)
	expected := a.ExpectExit
	if expected == nil {
		expected = scenario.Invoke.ExpectExit
	}
	if expected != nil && *expected != res.ExitCode {
		failures = append(failures, fmt.Sprintf(
			"expect_exit %d but process exited with %d", *expected, res.ExitCode))
	}

	for _, sub := range a.StdoutContains {
		if !strings.Contains(res.Stdout, sub) {
			failures = append(failures, fmt.Sprintf(
				"stdout_contains %q not found in stdout", sub))
		}
	}
	for _, pat := range a.StdoutRegex {
		re, err := regexp.Compile(pat)
		if err != nil {
			failures = append(failures, fmt.Sprintf(
				"stdout_regex %q invalid: %v", pat, err))
			continue
		}
		if !re.MatchString(res.Stdout) {
			failures = append(failures, fmt.Sprintf(
				"stdout_regex %q did not match stdout", pat))
		}
	}

	for _, sub := range a.StderrContains {
		if !strings.Contains(res.Stderr, sub) {
			failures = append(failures, fmt.Sprintf(
				"stderr_contains %q not found in stderr", sub))
		}
	}
	for _, pat := range a.StderrRegex {
		re, err := regexp.Compile(pat)
		if err != nil {
			failures = append(failures, fmt.Sprintf(
				"stderr_regex %q invalid: %v", pat, err))
			continue
		}
		if !re.MatchString(res.Stderr) {
			failures = append(failures, fmt.Sprintf(
				"stderr_regex %q did not match stderr", pat))
		}
	}

	// Phase C.2: journal / journal_absent / expect_dimensions matchers.
	// If the runner failed to capture the session journal AND any of these
	// matchers are declared, surface the capture error.
	if (len(a.Journal) > 0 || len(a.JournalAbsent) > 0 || len(a.ExpectDimensions) > 0) && res.JournalErr != nil {
		failures = append(failures, fmt.Sprintf(
			"journal-based assertions declared but journal capture failed: %v", res.JournalErr))
		return failures
	}

	vars := newMatcherVars()

	for i, shape := range a.Journal {
		matched := false
		for _, evt := range res.Journal {
			if matchEvent(evt, shape, vars) {
				matched = true
				break
			}
		}
		if !matched {
			failures = append(failures, fmt.Sprintf(
				"journal[%d] %v: no event in journal matched the shape", i, shape))
		}
	}

	for i, shape := range a.JournalAbsent {
		for _, evt := range res.Journal {
			if matchEvent(evt, shape, vars) {
				failures = append(failures, fmt.Sprintf(
					"journal_absent[%d] %v: event matched (must not appear): %s seq=%d",
					i, shape, evt.Type, evt.Seq))
				break
			}
		}
	}

	for i, shape := range a.ExpectDimensions {
		child, _ := shape["child"].(string)
		snapshot, err := findSpawnDimensions(res.Journal, child)
		if err != nil {
			failures = append(failures, fmt.Sprintf(
				"expect_dimensions[%d]: %v", i, err))
			continue
		}
		if msg := matchDimensions(snapshot, shape, vars); msg != "" {
			failures = append(failures, fmt.Sprintf(
				"expect_dimensions[%d] (child=%q): %s", i, child, msg))
		}
	}

	return failures
}
