// SPDX-License-Identifier: Apache-2.0

// Package api is the use-cases service contract: add, split and retire a
// use-case, attach its acceptance criteria, find the unchecked ones and the
// goals a use-case serves. Every verb is implemented over the CWM Object API —
// the service keeps no storage and no index of its own.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Object types and the edge this service works with.
const (
	UseCaseType   = "use-case"
	CriterionType = "acceptance-criterion"
	EdgeServes    = "serves"
)

// States of the `manual` workflow a use-case's lifecycle moves through that the
// service names: a new use-case starts in the initial state; a retired or split
// one is dropped, the reason in its lifecycle history.
const (
	StateInitial = "idea"
	StateDropped = "dropped"
)

// UseCasesServiceDef describes the service for KTSP/CLI auto-generation.
var UseCasesServiceDef = meta.ServiceDef{
	Name:    "use-cases",
	Short:   "A module's use-cases: add, split, retire, attach criteria, find the unchecked ones",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Add":             meta.Method("Add a use-case to a module, serving the goals it names"),
		"Split":           meta.Method("Replace a use-case with several, each keeping the goals it served"),
		"Retire":          meta.Method("Retire a use-case that is no longer wanted"),
		"AttachCriterion": meta.Method("Attach an acceptance criterion to a use-case"),
		"Uncovered":       meta.Method("List the live use-cases no criterion checks"),
		"Goals":           meta.Method("List the goals a use-case serves"),
	},
}

// UseCasesService manages use-case and acceptance-criterion objects.
type UseCasesService interface {
	Add(ctx context.Context, in AddInput) (AddOutput, error)
	Split(ctx context.Context, in SplitInput) (SplitOutput, error)
	Retire(ctx context.Context, in RetireInput) (RetireOutput, error)
	AttachCriterion(ctx context.Context, in AttachCriterionInput) (AttachCriterionOutput, error)
	Uncovered(ctx context.Context, in UncoveredInput) (UncoveredOutput, error)
	Goals(ctx context.Context, in GoalsInput) (GoalsOutput, error)
}

// UseCase is the service's view of one use-case object.
type UseCase struct {
	ID     string   `json:"id"`
	Module string   `json:"module"`
	Title  string   `json:"title"`
	Parent string   `json:"parent"`
	Status string   `json:"status,omitempty"`
	Serves []string `json:"serves,omitempty" jsonschema:"Ids the use-case has a serves edge to"`
}

// Criterion is one acceptance criterion.
type Criterion struct {
	ID       string `json:"id"`
	UseCase  string `json:"use_case"`
	Title    string `json:"title"`
	Probe    string `json:"probe,omitempty"`
	Severity string `json:"severity,omitempty"`
}

// AddInput adds a use-case.
type AddInput struct {
	Module string   `json:"module"           jsonschema:"Module the use-case belongs to; required"`
	Title  string   `json:"title"            jsonschema:"The need in one line; required"`
	Parent string   `json:"parent,omitempty" jsonschema:"Object it sits under; empty = the module itself"`
	Serves []string `json:"serves,omitempty" jsonschema:"Goal ids it serves"`
}

// AddOutput returns the created use-case.
type AddOutput struct {
	UseCase UseCase `json:"use_case"`
	Path    string  `json:"path" jsonschema:"Repo-relative path of the use-case's file"`
}

// SplitInput replaces a use-case with several.
type SplitInput struct {
	ID string `json:"id" jsonschema:"The use-case to split; required"`
	// Actor is who splits — recorded in the source's lifecycle history.
	Actor      cwmapi.Actor     `json:"actor"      jsonschema:"Who splits; required"`
	Successors []SplitSuccessor `json:"successors" jsonschema:"At least two"`
	Reason     string           `json:"reason"     jsonschema:"Why; required — the journal records it with the split"`
}

// SplitSuccessor is one use-case replacing the split one.
type SplitSuccessor struct {
	Title    string   `json:"title"              jsonschema:"Its need in one line; required"`
	Criteria []string `json:"criteria,omitempty" jsonschema:"Ids of the source's criteria it adopts"`
}

// SplitOutput reports the successors (the journal records the split).
type SplitOutput struct {
	Successors []UseCase `json:"successors"`
	Orphans    []string  `json:"orphans,omitempty" jsonschema:"Criteria no successor adopted; they stay on the source"`
}

// RetireInput retires a use-case.
type RetireInput struct {
	ID string `json:"id" jsonschema:"Use-case id; required"`
	// Actor is who retires — recorded in the lifecycle history.
	Actor  cwmapi.Actor `json:"actor"            jsonschema:"Who retires; required"`
	Reason string       `json:"reason,omitempty" jsonschema:"Why; appended to its prose under a Retired heading"`
}

// RetireOutput returns the retired use-case.
type RetireOutput struct {
	UseCase UseCase `json:"use_case"`
}

// AttachCriterionInput attaches a criterion to a use-case.
type AttachCriterionInput struct {
	UseCase  string `json:"use_case"           jsonschema:"Use-case id; required"`
	Title    string `json:"title"              jsonschema:"The claim in one line; required"`
	Probe    string `json:"probe,omitempty"    jsonschema:"A shell command that exits 0 when the claim holds; empty = not yet measurable"`
	Severity string `json:"severity,omitempty" jsonschema:"core (default) or the severity the method grades by"`
}

// AttachCriterionOutput returns the criterion.
type AttachCriterionOutput struct {
	Criterion Criterion `json:"criterion"`
	Path      string    `json:"path" jsonschema:"Repo-relative path of the criterion's file"`
}

// UncoveredInput selects the use-cases to check.
type UncoveredInput struct {
	Module string `json:"module,omitempty" jsonschema:"Only this module's use-cases; empty = every one"`
}

// UncoveredOutput lists the live use-cases with no criterion.
type UncoveredOutput struct {
	UseCases []UseCase `json:"use_cases"`
}

// GoalsInput names a use-case.
type GoalsInput struct {
	ID string `json:"id" jsonschema:"Use-case id; required"`
}

// Served is one object a use-case serves.
type Served struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

// GoalsOutput lists what the use-case serves.
type GoalsOutput struct {
	Goals []Served `json:"goals"`
}
