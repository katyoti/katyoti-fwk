// SPDX-License-Identifier: Apache-2.0

// Package api is the verification service contract: READING the `verify` and
// `lint` hook points over CWM objects. CWM owns the points and runs the hooks
// (ObjectRunHooks, raw answers); this service interprets them — a verify
// answer as PASS / FAIL / UNPROVEN, a lint answer as findings — and adds two
// functions to expressions: `verify()` and `lint()`, which a guard or a gate
// check reads (`verify().green && lint().blocking == 0`).
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// VerificationServiceDef describes the service for KTSP/CLI auto-generation.
// lint() is read-only. verify() is NOT: it runs every verify hook under the
// object, and an acceptance criterion's hook records its verdict — so it is a
// costly function (CWM runs a costly function's action without asking it to
// be read-only, and never in a preview).
//
// Verify and verify() declare VerifyTimeoutS: a verify hook may run a probe (a
// Go test, a build) and a subtree runs its hooks one after another, so a caller
// over the bus waits that long instead of giving up at the transport default.
// Verify is also declared long: run from the action
// surface it is a method run with its panel, each check reported into it as
// it answers. verify() is not: a gate asks it and waits for its answer.
var VerificationServiceDef = meta.ServiceDef{
	Name:    "verification",
	Short:   "Read the verify and lint hook points over CWM objects",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Verify":         {Short: "Verify an object and everything under it: run every verify hook in force and count pass / fail / unproven", TimeoutS: VerifyTimeoutS, LongRunning: true},
		"Lint":           meta.Method("Lint one object (with subtree: and everything under it) or a module: run every lint hook in force and return the findings"),
		"VerifyFunction": {Short: "verify() in an expression: the verification of the object and everything under it, counted; the criteria's hooks record their verdicts", TimeoutS: VerifyTimeoutS},
		"LintFunction":   {Short: "lint() in an expression: the lint of the object and everything under it, with the findings at or above the type's verification.gate_blocks_on", ReadOnly: true},
	},
}

// VerifyTimeoutS is how long a caller waits for a verification: two hours,
// room for a probe at the longest bound a check may declare (an hour) and
// others run in turn.
const VerifyTimeoutS = 7200

// VerificationService reads the verify and lint points.
type VerificationService interface {
	Verify(ctx context.Context, in VerifyInput) (VerifyOutput, error)
	Lint(ctx context.Context, in LintInput) (LintOutput, error)
	VerifyFunction(ctx context.Context, in cwmapi.FunctionCall) (VerifySummary, error)
	LintFunction(ctx context.Context, in cwmapi.FunctionCall) (LintSummary, error)
}

// VerifyInput names the root of the walk.
type VerifyInput struct {
	ID string `json:"id" jsonschema:"The object to verify, with everything under it; required"`
}

// VerifyOutcome is what one verify hook answered for one object.
type VerifyOutcome struct {
	ObjectID string `json:"object_id"`
	URI      string `json:"uri"`
	Source   string `json:"source" jsonschema:"Who declared the hook: capability:<id> | type:<name> | object"`
	Status   string `json:"status" jsonschema:"pass | fail | needs-harness | no-probe | harness-error | hook-error"`
	Evidence string `json:"evidence,omitempty"`
	// Verdict is the object that recorded it, when the hook records one.
	Verdict string `json:"verdict,omitempty"`
	// Timeout is the extended bound the check ran on, when it declared one
	// longer than the default (the hook's answer says so); empty otherwise.
	Timeout string `json:"timeout,omitempty" jsonschema:"The extended bound the check ran on (30m); empty on the default"`
}

// VerifyOutput counts the answers. Green is Failed and Unproven empty with at
// least one pass — no hook, no verdict.
//
// A verify action answers `{status, evidence?, verdict?}`. Only `pass` is a
// pass and only `fail` a fail: anything else — no probe, a command nobody
// runs here, a check that could not run, an answer that is not a verify
// result — is UNPROVEN, never folded into either column.
type VerifyOutput struct {
	// Objects is how many objects under the root had a verify hook.
	Objects  int             `json:"objects"`
	Runs     int             `json:"runs"`
	Passed   int             `json:"passed"`
	Failed   []VerifyOutcome `json:"failed,omitempty"`
	Unproven []VerifyOutcome `json:"unproven,omitempty"`
	// Outcomes is every answer, in object then hook order.
	Outcomes []VerifyOutcome `json:"outcomes"`
}

// Green reports a verification that proved everything it ran, and ran
// something.
func (o VerifyOutput) Green() bool {
	return o.Runs > 0 && len(o.Failed) == 0 && len(o.Unproven) == 0
}

// LintInput lints one object (with Subtree, the object and every
// descendant), or every object of one module. Exactly one of ID and Module;
// Subtree goes with ID.
type LintInput struct {
	ID      string `json:"id,omitempty"      jsonschema:"One object to lint"`
	Module  string `json:"module,omitempty"  jsonschema:"Every object of this module"`
	Subtree bool   `json:"subtree,omitempty" jsonschema:"With id: the object and every descendant"`
}

// LintFinding is one finding a lint hook reported, located at the object and
// the hook that found it. A finding is never a verdict.
type LintFinding struct {
	ObjectID string `json:"object_id"`
	URI      string `json:"uri"      jsonschema:"The lint hook that reported it"`
	Source   string `json:"source"   jsonschema:"Who declared the hook: capability:<id> | type:<name> | object"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Severity string `json:"severity" jsonschema:"error | warn | info"`
}

// LintOutput is every finding, plus what was run. A hook that could not run,
// or answered something that is not a lint result, is a finding of rule
// `lint-hook-error` — never a silent pass.
type LintOutput struct {
	Findings []LintFinding `json:"findings"`
	Objects  int           `json:"objects"  jsonschema:"Objects linted"`
	Runs     int           `json:"runs"     jsonschema:"Lint hooks run"`
}

// VerifySummary is what `verify()` answers in an expression.
type VerifySummary struct {
	Runs     int  `json:"runs"`
	Passed   int  `json:"passed"`
	Failed   int  `json:"failed"`
	Unproven int  `json:"unproven"`
	Green    bool `json:"green"`
}

// LintSummary is what `lint()` answers in an expression: Blocking counts the
// findings at or above the object type's gate_blocks_on (error when unset).
type LintSummary struct {
	Findings int `json:"findings"`
	Blocking int `json:"blocking"`
}
