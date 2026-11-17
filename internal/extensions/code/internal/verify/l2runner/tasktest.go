// SPDX-License-Identifier: Apache-2.0

// Package l2runner runs `.tasktest` scenarios for the `module verify --layer L2`
// surface. Each scenario invokes a real binary (typically `ktw-agents run`)
// against a fixture home dir, captures stdout/stderr/exit, and matches
// assertions declared in YAML.
//
// Phase C.1 scope (this iteration): YAML parser + variable substitution +
// fixture-copy + invoke + expect_exit + stdout/stderr contains/regex matchers.
// Journal capture + dimension assertions deferred to Phase C.2 (see
// `tier-1-verify-l2-journal-and-dimensions`).
package l2runner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Scenario is the parsed `.tasktest` file. Fields not consumed by Phase C.1
// (e.g. `setup.llm`) are tolerated (yaml.v3 ignores unknown keys by default
// when using `yaml:"-"` shapes — but here we accept them via untyped maps
// where useful for forward-compat).
type Scenario struct {
	Path string `yaml:"-"` // resolved at parse time

	UC           string `yaml:"uc"`
	Title        string `yaml:"title"`
	Description  string `yaml:"description,omitempty"`
	Severity     string `yaml:"severity,omitempty"`
	AuditFinding string `yaml:"audit_finding,omitempty"`

	// Tags are free-form labels for discovery/filtering (e.g. "no-llm",
	// "delegation-mode"). They carry no behavior — a suite runner may use them
	// to select a subset. A simulator-only scenario is tagged "no-llm".
	Tags []string `yaml:"tags,omitempty"`

	Setup      Setup      `yaml:"setup"`
	Invoke     Invoke     `yaml:"invoke"`
	Assertions Assertions `yaml:"assertions"`
}

// Setup describes the per-run environment.
type Setup struct {
	// Home is a directory (path relative to the .tasktest file, or absolute)
	// whose contents are copied into $TMP_HOME before invoke. Optional.
	Home string `yaml:"home,omitempty"`

	// LLM selects the LLM mode. "simulator" for token-free runs (default).
	// Phase C.1 ignores this field; the user-supplied `cmd` controls
	// LLM-vs-simulator (typically via --dry-run on ktw-agents).
	LLM string `yaml:"llm,omitempty"`

	// Env is environment variables set on the invoke process. Values are
	// variable-substituted (e.g. KTW_WORKSPACE: $TMP_HOME).
	Env map[string]string `yaml:"env,omitempty"`
}

// Invoke describes the command to run.
type Invoke struct {
	Cmd        string        `yaml:"cmd"`
	Timeout    time.Duration `yaml:"timeout,omitempty"`
	ExpectExit *int          `yaml:"expect_exit,omitempty"` // nil = ignore
}

// Assertions describes the per-scenario success conditions.
type Assertions struct {
	// ExpectExit is the expected process exit code. nil = ignore (pass on
	// any exit).
	ExpectExit *int `yaml:"expect_exit,omitempty"`

	StdoutContains []string `yaml:"stdout_contains,omitempty"`
	StdoutRegex    []string `yaml:"stdout_regex,omitempty"`
	StderrContains []string `yaml:"stderr_contains,omitempty"`
	StderrRegex    []string `yaml:"stderr_regex,omitempty"`

	// Phase C.2 journal-based assertions. Each entry is a "shape map" matched
	// against captured JournalEvents. The `event:` key (or `type:`) names the
	// expected event type; remaining keys path-match against `event.Data`.
	// Matcher syntax: literal value | "any" | "$VAR" | {regex: "..."} |
	// {contains: "..."} | {exists: true|false}.
	Journal       []map[string]any `yaml:"journal,omitempty"`
	JournalAbsent []map[string]any `yaml:"journal_absent,omitempty"`

	// ExpectDimensions checks the simulator's `__spawn_dimensions` snapshot
	// against expected per-category values. Each entry SHOULD include a
	// `child:` key naming the spawned child whose dimensions to inspect (omit
	// for the top-level run's own dimensions). Remaining keys correspond to
	// the snapshot's 6 dimension categories (scheduling, context, permissions,
	// agent_comms, user_comms, isolation) — each a nested map with the values
	// to match. Strings undergo $VAR expansion: `$SAFE_TOOL_SUBSET` resolves
	// to the presets constant.
	ExpectDimensions []map[string]any `yaml:"expect_dimensions,omitempty"`
}

// Load parses a `.tasktest` file from disk. Variable substitution is NOT
// applied here; Run does it after computing $TMP_HOME (deferred substitution).
func Load(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	var s Scenario
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	s.Path = abs
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &s, nil
}

// validate enforces required fields and surface-level invariants.
func (s *Scenario) validate() error {
	if s.UC == "" {
		return fmt.Errorf("missing required field: uc")
	}
	if !ucIDRe.MatchString(s.UC) {
		return fmt.Errorf("invalid uc %q (expected UC-NNN)", s.UC)
	}
	if s.Invoke.Cmd == "" {
		return fmt.Errorf("missing required field: invoke.cmd")
	}
	for _, pat := range s.Assertions.StdoutRegex {
		if _, err := regexp.Compile(pat); err != nil {
			return fmt.Errorf("invalid stdout_regex %q: %w", pat, err)
		}
	}
	for _, pat := range s.Assertions.StderrRegex {
		if _, err := regexp.Compile(pat); err != nil {
			return fmt.Errorf("invalid stderr_regex %q: %w", pat, err)
		}
	}
	return nil
}

var ucIDRe = regexp.MustCompile(`^UC-\d+$`)

// substVars replaces the $TMP_HOME / $SCENARIO_DIR / $NOW placeholders in s
// with their resolved values. Idempotent on inputs without placeholders.
func substVars(s string, vars map[string]string) string {
	out := s
	for name, value := range vars {
		out = strings.ReplaceAll(out, "$"+name, value)
	}
	return out
}

// applySubstitutions walks the Scenario in place, replacing every variable
// reference with its resolved value. Called by Run after $TMP_HOME exists.
func (s *Scenario) applySubstitutions(vars map[string]string) {
	s.Setup.Home = substVars(s.Setup.Home, vars)
	for k, v := range s.Setup.Env {
		s.Setup.Env[k] = substVars(v, vars)
	}
	s.Invoke.Cmd = substVars(s.Invoke.Cmd, vars)
	for i := range s.Assertions.StdoutContains {
		s.Assertions.StdoutContains[i] = substVars(s.Assertions.StdoutContains[i], vars)
	}
	for i := range s.Assertions.StderrContains {
		s.Assertions.StderrContains[i] = substVars(s.Assertions.StderrContains[i], vars)
	}
}
