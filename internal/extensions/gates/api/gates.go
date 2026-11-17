// SPDX-License-Identifier: Apache-2.0

// Package api is the gates service contract. A GATE is a named definition of
// done — ready-to-plan, ready-to-build, ready-to-test, ready-to-release — made
// of REQUIREMENTS, each checked on its own so a refusal says which are unmet:
// a `check:` (an expression CWM evaluates over the object, ObjectEvaluate), a
// `uri:` (an action answering {accept, reason}) or `person: true` (met by a
// person's confirmation, kept in the object's `gates` group). A gate is
// declared once (`gates/`); a type or a pack adds requirements to a gate it
// did not declare (`gate-requirements/`).
//
// The service adds `gate(name)` to expressions: a workflow guard reads
// `gate('ready-to-build').passes`, and a refused move carries the checklist
// in its function answers (ChecklistOf) — what a planner plans from.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// FunctionName is the expression function the gates capability declares.
const FunctionName = "gate"

// GatesServiceDef describes the service for KTSP/CLI auto-generation. Confirm
// and Revoke are a person's acts: kept off the LLM tool surface (NoLLM), as the
// gates agent plugin omits them; the GUI and the CLI still call them.
var GatesServiceDef = meta.ServiceDef{
	Name:    "gates",
	Short:   "Named gates: an object's checklist for a definition of done, and a person's confirmations",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Check":        {Short: "An object's checklist for a named gate: each requirement met, unmet or unproven, why, and how it is resolved", ReadOnly: true},
		"Confirm":      {Short: "A person confirms one requirement of a named gate on an object; recorded with who and when", NoLLM: true},
		"GateFunction": {Short: "gate(name) in an expression: the object's checklist for the gate", ReadOnly: true},
		"List":         {Short: "The named gates that apply to an object (by its type), each with its requirements' statuses", ReadOnly: true},
		"Revoke":       {Short: "A person withdraws a confirmation of one requirement of a named gate on an object; the withdrawal is kept with who and when", NoLLM: true},
		"Steps":        {Short: "The steps a named gate's requirements add for an object to the request that plans the phase the gate closes, in priority order; no check is run", ReadOnly: true},
	},
}

// GatesService checks gates and records confirmations.
type GatesService interface {
	Check(ctx context.Context, in CheckInput) (CheckOutput, error)
	Confirm(ctx context.Context, in ConfirmInput) (ConfirmOutput, error)
	GateFunction(ctx context.Context, in cwmapi.FunctionCall) (GateChecklist, error)
	List(ctx context.Context, in ListInput) (ListOutput, error)
	Revoke(ctx context.Context, in RevokeInput) (RevokeOutput, error)
	Steps(ctx context.Context, in StepsInput) (StepsOutput, error)
}

// CheckInput evaluates a gate on an object; By / From / To are the move it
// would allow, when there is one.
type CheckInput struct {
	ID   string        `json:"id"             jsonschema:"The object; required"`
	Gate string        `json:"gate"           jsonschema:"The gate; required"`
	By   *cwmapi.Actor `json:"by,omitempty"   jsonschema:"Who would move the object"`
	From string        `json:"from,omitempty" jsonschema:"The state a move would leave"`
	To   string        `json:"to,omitempty"   jsonschema:"The state a move would reach"`
	// NoCostly is a preview's check: a requirement calling a costly function
	// is not run — it reads unproven, decided at the move.
	NoCostly bool `json:"no_costly,omitempty" jsonschema:"A preview: a check calling a costly function is not run and reads unproven - decided at the move"`
}

// CheckOutput is the checklist.
type CheckOutput struct {
	Checklist GateChecklist `json:"checklist"`
}

// ConfirmInput records a person's confirmation of a requirement.
type ConfirmInput struct {
	ID          string       `json:"id"             jsonschema:"The object; required"`
	Gate        string       `json:"gate"           jsonschema:"The gate; required"`
	Requirement string       `json:"requirement"    jsonschema:"The person requirement confirmed; required"`
	By          cwmapi.Actor `json:"by"             jsonschema:"Who confirms - a person; required"`
	Note        string       `json:"note,omitempty"`
}

// ConfirmOutput is the checklist after the confirmation.
type ConfirmOutput struct {
	Checklist GateChecklist `json:"checklist"`
}

// ListInput names the object whose gates are listed.
type ListInput struct {
	ID string `json:"id" jsonschema:"The object; required"`
}

// ListOutput is every named gate that applies to the object — a gate applies
// when at least one of its requirements applies to the object's type — in
// gate-name order.
type ListOutput struct {
	Gates []GateSummary `json:"gates"`
}

// GateSummary is one gate's checklist on the object, counted.
type GateSummary struct {
	Gate     string     `json:"gate"`
	Label    string     `json:"label,omitempty"`
	Passes   bool       `json:"passes"`
	Met      int        `json:"met"`
	Unmet    int        `json:"unmet"`
	Unproven int        `json:"unproven"`
	Items    []GateItem `json:"items"`
}

// RevokeInput withdraws a person's confirmation of a requirement.
type RevokeInput struct {
	ID          string       `json:"id"             jsonschema:"The object; required"`
	Gate        string       `json:"gate"           jsonschema:"The gate; required"`
	Requirement string       `json:"requirement"    jsonschema:"The person requirement whose confirmation is withdrawn; required"`
	By          cwmapi.Actor `json:"by"             jsonschema:"Who withdraws it - a person; required"`
	Note        string       `json:"note,omitempty"`
}

// RevokeOutput is the withdrawal as kept, and the checklist after it.
type RevokeOutput struct {
	Revoked   GateRevocation `json:"revoked"`
	Checklist GateChecklist  `json:"checklist"`
}

// StepsInput names the object and the gate whose added steps are read.
type StepsInput struct {
	ID   string `json:"id"   jsonschema:"The object; required"`
	Gate string `json:"gate" jsonschema:"The gate; required"`
}

// StepsOutput is the steps the gate's requirements that apply to the object
// add (resolve.step), by priority then requirement name. A gate with no
// requirement for the object's type adds none.
type StepsOutput struct {
	Gate  string      `json:"gate"`
	Steps []AddedStep `json:"steps"`
}

// AddedStep is one requirement's step, with the requirement and the pack
// that declares it.
type AddedStep struct {
	Requirement string          `json:"requirement"`
	Provider    string          `json:"provider,omitempty"`
	Step        RequirementStep `json:"step"`
}

// ChecklistOf reads, among an evaluation's function answers, what
// gate(name) answered.
func ChecklistOf(calls []cwmapi.FunctionAnswer, gate string) (GateChecklist, bool) {
	a, ok := cwmapi.AnswerOf(calls, FunctionName, gate)
	if !ok || a.Error != "" || a.Answer == nil {
		return GateChecklist{}, false
	}
	var cl GateChecklist
	if err := remarshal(a.Answer, &cl); err != nil {
		return GateChecklist{}, false
	}
	return cl, true
}

// RefusedChecklist is the checklist of the first gate a refused move's guard
// called that does not pass.
func RefusedChecklist(err error) (GateChecklist, bool) {
	var refused *cwmapi.TransitionRefusedError
	if !errors.As(err, &refused) {
		return GateChecklist{}, false
	}
	return FailingChecklist(refused.Calls)
}

// FailingChecklist is the first gate checklist among calls that does not pass.
func FailingChecklist(calls []cwmapi.FunctionAnswer) (GateChecklist, bool) {
	for _, c := range calls {
		if c.Function != FunctionName || len(c.Args) != 1 {
			continue
		}
		if cl, ok := ChecklistOf(calls, fmt.Sprint(c.Args[0])); ok && !cl.Passes {
			return cl, true
		}
	}
	return GateChecklist{}, false
}

// remarshal decodes a function's JSON answer (already read as any) into v.
func remarshal(from any, v any) error {
	b, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
