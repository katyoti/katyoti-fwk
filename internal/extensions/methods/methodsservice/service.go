// SPDX-License-Identifier: Apache-2.0

// Package methodsservice publishes MethodsService — the neutral methods-pack
// dispatch surface that holds BOTH the generic file linters AND the pure
// acceptance verifier. Every method is artifact-AGNOSTIC and STATELESS (it
// reads files / takes data as input and returns a verdict) — it touches no CWM
// and holds no lock, so it lives cleanly in the ktw-methods sidecar. The
// orchestration that walks the graph and records verdicts is CWM's job;
// MethodsService only computes verdicts.
//
// Verbs auto-derive kebab (FileExists → file-exists, VerifyAcceptance →
// verify-acceptance). Node/item types list them by URI: `ktw-methods:methods/<verb>`.
package methodsservice

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

// MethodsService is the neutral methods verb surface: linters + verifiers.
type MethodsService interface {
	// Linters — generic file quality checks (wrap methods/verify funcs).
	FileExists(ctx context.Context, in PathInput) (verify.LintResult, error)
	MarkdownFrontmatter(ctx context.Context, in PathInput) (verify.LintResult, error)
	FrontmatterKeys(ctx context.Context, in KeysInput) (verify.LintResult, error)
	NonEmpty(ctx context.Context, in PathInput) (verify.LintResult, error)
	// ValueIn — a value is one of the allowed ones: how a type fixes a field
	// to a set through its lint hook (a goal's reachability is asymptotic).
	ValueIn(ctx context.Context, in ValueInInput) (verify.LintResult, error)

	// VerifyAcceptance — the pure acceptance verifier. Given a list of
	// acceptance criteria (passed AS INPUT — the caller interpolates them from
	// the use-case body), it runs each criterion's files-only probe and returns
	// a single ProbeResult. No CWM, no lock — the data comes in, the verdict
	// goes out; the cwm host records it.
	VerifyAcceptance(ctx context.Context, in AcceptanceInput) (verify.ProbeResult, error)

	// Rollup — the METHOD's grading vocabulary. Takes raw per-check outcomes as
	// INPUT and returns the grade (green | unmet | ungraded + progress + the unmet
	// frontier). Stateless: it touches no CWM. Walking the graph and running the
	// checks is CWM's job; INTERPRETING the results is the method's.
	Rollup(ctx context.Context, in RollupInput) (RollupOutput, error)
}

// PathInput is the input for the single-path linters.
type PathInput struct {
	Path string `json:"path" cli:"arg" desc:"File to lint (workspace-relative or absolute)" required:"true"`
}

// KeysInput adds the required-keys set for the frontmatter-keys linter.
type KeysInput struct {
	Path string   `json:"path" cli:"arg"  desc:"File to lint (workspace-relative or absolute)" required:"true"`
	Keys []string `json:"keys" cli:"keys" desc:"Required frontmatter keys"`
}

// ValueInInput is the input of the value-in linter: the value (a `${...}` over
// the object, empty when unset), the default an empty value reads as, the
// allowed values, and the rule and message of the finding.
type ValueInInput struct {
	Value   string   `json:"value,omitempty"   cli:"value"   desc:"The value checked; empty reads as default"`
	Default string   `json:"default,omitempty" cli:"default" desc:"What an empty value reads as (the field's schema default)"`
	Allowed []string `json:"allowed"           cli:"allowed" desc:"The allowed values" required:"true"`
	Rule    string   `json:"rule,omitempty"    cli:"rule"    desc:"The finding's rule code; empty = value-not-allowed"`
	Message string   `json:"message,omitempty" cli:"message" desc:"What the finding says"`
}

// AcceptanceInput carries the acceptance criteria to verify.
type AcceptanceInput struct {
	Criteria []AcceptanceCriterion `json:"criteria"`
}

// AcceptanceCriterion mirrors one inline acceptance_criteria record. Only
// test_ref drives the probe; id is for the evidence string.
type AcceptanceCriterion struct {
	ID      string `json:"id"`
	Claim   string `json:"claim,omitempty"`
	Kind    string `json:"kind,omitempty"`
	TestRef string `json:"test_ref"`
	Status  string `json:"status,omitempty"`
}

// MethodsServiceDef declares the service for publication.
var MethodsServiceDef = meta.ServiceDef{
	Name:    "methods",
	Short:   "Neutral methods linters + verifiers",
	Long:    "Artifact-agnostic, stateless verb surface: file linters (file-exists, markdown-frontmatter, frontmatter-keys, non-empty), the value-in linter returning LintResult, and the acceptance verifier (verify-acceptance) returning a ProbeResult over a criteria list. CWM node/item types list these by URI as their linter:/verifier: checks.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"FileExists":          meta.Method("Lint: the target file exists"),
		"MarkdownFrontmatter": meta.Method("Lint: file is well-structured markdown with a YAML frontmatter block + body"),
		"FrontmatterKeys":     meta.Method("Lint: required frontmatter keys are present"),
		"NonEmpty":            meta.Method("Lint: file has non-whitespace content"),
		"ValueIn":             meta.Method("Lint: a value is one of the allowed ones (a type fixing a field through its lint hook)"),
		"VerifyAcceptance":    meta.Method("Verify: run each acceptance criterion's files-only probe; pass only if every criterion passes"),
		"Rollup":              meta.Method("Grade raw check outcomes: green only when every check passed; unproven blocks green"),
	},
}

// Config is the (empty) service config.
type Config struct{}

// New constructs the MethodsService.
func New() MethodsService { return &service{} }

type service struct {
	repoRoot string // test override; empty = resolve against the workspace root
}

// envRoot is the workspace dir, guarded so a bare process (no Application set,
// e.g. a unit test) yields "" instead of panicking — callers then treat
// targets as absolute / cwd-relative.
func envRoot() (root string) {
	defer func() { _ = recover() }()
	return application.Env().WorkspaceDir()
}

// resolve resolves a relative target against the workspace root, so a node's
// workspace-relative path resolves the same way object-verify's files probe
// does. Absolute paths pass through unchanged.
func (s *service) resolve(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	root := s.repoRoot
	if root == "" {
		root = envRoot()
	}
	if root == "" {
		return p
	}
	return filepath.Join(root, p)
}

func (s *service) FileExists(_ context.Context, in PathInput) (verify.LintResult, error) {
	return verify.LintFileExists(s.resolve(in.Path)), nil
}

func (s *service) MarkdownFrontmatter(_ context.Context, in PathInput) (verify.LintResult, error) {
	return verify.LintMarkdownFrontmatter(s.resolve(in.Path)), nil
}

func (s *service) FrontmatterKeys(_ context.Context, in KeysInput) (verify.LintResult, error) {
	return verify.LintFrontmatterKeys(s.resolve(in.Path), in.Keys), nil
}

func (s *service) NonEmpty(_ context.Context, in PathInput) (verify.LintResult, error) {
	return verify.LintNonEmpty(s.resolve(in.Path)), nil
}

func (s *service) ValueIn(_ context.Context, in ValueInInput) (verify.LintResult, error) {
	return verify.LintValueIn(in.Value, in.Default, in.Allowed, in.Rule, in.Message), nil
}

func (s *service) VerifyAcceptance(ctx context.Context, in AcceptanceInput) (verify.ProbeResult, error) {
	if len(in.Criteria) == 0 {
		// No ACs written yet is a real, common state — and it is NOT acceptance.
		return verify.ProbeResult{Status: "fail", Evidence: "no acceptance criteria defined"}, nil
	}
	runner := verify.FilesProbeRunner{Root: s.workspaceRoot()}
	var failed []string
	for _, ac := range in.Criteria {
		if strings.TrimSpace(ac.TestRef) == "" {
			failed = append(failed, ac.ID+" (no test_ref)")
			continue
		}
		if r := runner.RunProbe(ctx, ac.TestRef); r.Status != "pass" {
			failed = append(failed, fmt.Sprintf("%s (%s: %s)", ac.ID, r.Status, ac.TestRef))
		}
	}
	if len(failed) > 0 {
		return verify.ProbeResult{Status: "fail", Evidence: "failing criteria: " + strings.Join(failed, "; ")}, nil
	}
	return verify.ProbeResult{Status: "pass", Evidence: fmt.Sprintf("%d criteria passed", len(in.Criteria))}, nil
}

// workspaceRoot is the FilesProbeRunner root: the workspace dir (empty in a
// bare process — probes then resolve relative test_refs against the cwd).
func (s *service) workspaceRoot() string {
	if s.repoRoot != "" {
		return s.repoRoot
	}
	return envRoot()
}
