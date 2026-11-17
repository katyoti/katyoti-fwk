// SPDX-License-Identifier: Apache-2.0

// Package api is the acceptance service contract: verify ONE criterion — an
// object carrying `checkable` — by running its probe and recording the result
// as its `verdict` object; and answer a probe that records nothing (Probe — a
// policy's verify entry).
//
// ONLY A RUN PRODUCES A VERDICT. An `expression` probe is evaluated over the
// object by CWM (ObjectEvaluate, as a guard would be); a criterion's `command`
// probe runs in the project the universe serves (exit 0 passes) — a policy's
// (Probe) is the method harness's to run: needs-harness; `manual` waits for a
// person; `none` (or no probe) is no-probe. It is the `verify` hook the method pack declares on
// the `checkable` capability; CWM's verify point (read by the verification
// service) walks a subtree and calls it once per criterion. The service keeps no storage: it reads and writes through
// the CWM Object API.
package api

import (
	"context"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/projectcmd"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The verdict type and its field-group (declared by ktw-methods).
const (
	VerdictType = "verdict"
	CapVerdict  = "verdict"
)

// HOW LONG A VERIFY MAY TAKE. A command probe is a Go test, a build: it runs up
// to ProbeTimeout — or to the bound its checkable declares (`timeout`, at most
// projectcmd.MaxTimeout) — then is stopped and its verdict records that it
// timed out. Verify declares VerifyTimeoutS — the longest bound a probe may
// declare plus room to record the verdict — so a caller over the bus (the
// ActionService running the verify hook) waits for the verdict instead of
// giving up at the transport default.
const (
	ProbeTimeout   = 10 * time.Minute
	VerifyTimeoutS = int((projectcmd.MaxTimeout + 5*time.Minute) / time.Second)
)

// AcceptanceServiceDef describes the service for KTSP/CLI auto-generation.
var AcceptanceServiceDef = meta.ServiceDef{
	Name:    "acceptance",
	Short:   "Verify an acceptance criterion: run its check and record its verdict",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Verify": {Short: "Run one criterion's probe and upsert its verdict object; the verify hook of `checkable`", TimeoutS: VerifyTimeoutS},
		"Probe":  meta.Method("Run a probe over an object and answer it as a verify answer, recording nothing; a policy's verify hook"),
	},
}

// AcceptanceService verifies criteria.
type AcceptanceService interface {
	Verify(ctx context.Context, in VerifyInput) (VerifyResult, error)
	Probe(ctx context.Context, in ProbeInput) (VerifyResult, error)
}

// ProbeInput is a probe to run over an object: the hook entry's declared input.
type ProbeInput struct {
	ID    string `json:"id"              jsonschema:"The object the probe reads; required"`
	Kind  string `json:"kind,omitempty"  jsonschema:"command | expression | manual | none; empty = expression"`
	Probe string `json:"probe,omitempty" jsonschema:"The probe; empty = no-probe"`
}

// VerifyInput names the criterion: the hook call's `object`, or an `id`.
type VerifyInput struct {
	ID     string         `json:"id,omitempty"     jsonschema:"The criterion's id; or send the hook call's object"`
	Object *cwmapi.Object `json:"object,omitempty" jsonschema:"The object the verify hook fires on"`
}

// VerifyResult is a verify answer: what the check said, why, and the verdict
// object that records it.
type VerifyResult struct {
	Status   string `json:"status" jsonschema:"pass | fail | needs-harness | no-probe | harness-error"`
	Evidence string `json:"evidence,omitempty"`
	Verdict  string `json:"verdict,omitempty"`
	// Timeout is the bound the probe ran on when its checkable declared one
	// longer than the default — what a grade says of a check run on an
	// extended bound; empty on the default.
	Timeout string `json:"timeout,omitempty" jsonschema:"The bound the probe ran on when its checkable declared one (30m); empty on the default"`
}
