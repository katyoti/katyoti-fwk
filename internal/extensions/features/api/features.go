// SPDX-License-Identifier: Apache-2.0

// Package api is the features service contract: what a person sees of a
// module's features — the method's want node, ranked by its cost of delay —
// and the verbs that manage them: add, split and retire a feature with its
// trail, attach its acceptance criteria, find the ones nothing checks and the
// goals a feature serves. Every verb is implemented over the CWM Object API;
// the service keeps nothing of its own.
package api

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/assistant"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// FeatureType is the object type a feature is stored as.
const FeatureType = "feature"

// CapFeature is the capability holding a feature's declared cost of delay.
const CapFeature = "feature"

// EdgeServes is the edge kind from a feature to the goal it serves.
const EdgeServes = "serves"

// CriterionType is the object type of an acceptance criterion, hung under the
// feature it checks.
const CriterionType = "acceptance-criterion"

// States of the `feature` workflow the service names: a new feature starts in
// the initial state; a retired or split one is dropped, the reason in its
// lifecycle history.
const (
	StateInitial = "idea"
	StateDropped = "dropped"
)

// FeaturesServiceDef describes the service for KTSP/CLI auto-generation.
var FeaturesServiceDef = meta.ServiceDef{
	Name:    "features",
	Short:   "A module's features: every one of its subtree with its state, its cost of delay and the goals it serves",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Board":           {Short: "Every feature of an object's subtree as the page the module's Features tab shows: highest cost of delay first, each with its state, the object it sits under, its cost of delay and the goals it serves", ReadOnly: true},
		"Add":             meta.Method("Add a feature to a module, or a sub-feature under a feature, serving the goals it names"),
		"Split":           meta.Method("Replace a feature with several, each keeping the goals it served, the trail recorded"),
		"Retire":          meta.Method("Retire a feature that is no longer wanted, the reason recorded"),
		"AttachCriterion": meta.Method("Attach an acceptance criterion, with its probe, to a feature"),
		"Uncovered":       {Short: "List the live features nothing checks: no criterion under them nor under a live sub-feature", ReadOnly: true},
		"Goals":           {Short: "List the goals a feature serves", ReadOnly: true},
	},
}

// FeaturesService answers and manages the features of a module.
type FeaturesService interface {
	// Board is the Features panel's source.
	Board(ctx context.Context, in BoardInput) (render.Def, error)
	// Add creates a feature in the feature workflow's initial state.
	Add(ctx context.Context, in AddInput) (AddOutput, error)
	// Split replaces a feature with several, the source dropped.
	Split(ctx context.Context, in SplitInput) (SplitOutput, error)
	// Retire drops a feature, the reason in its body.
	Retire(ctx context.Context, in RetireInput) (RetireOutput, error)
	// AttachCriterion hangs an acceptance criterion under a feature.
	AttachCriterion(ctx context.Context, in AttachCriterionInput) (AttachCriterionOutput, error)
	// Uncovered lists the live features nothing checks.
	Uncovered(ctx context.Context, in UncoveredInput) (UncoveredOutput, error)
	// Goals lists what a feature serves.
	Goals(ctx context.Context, in GoalsInput) (GoalsOutput, error)
}

// Feature is the service's view of one feature object.
type Feature struct {
	ID     string   `json:"id"`
	Module string   `json:"module"`
	Title  string   `json:"title"`
	Parent string   `json:"parent"`
	Status string   `json:"status,omitempty" jsonschema:"Its state in the feature workflow"`
	Serves []string `json:"serves,omitempty" jsonschema:"Ids the feature has a serves edge to"`
}

// Criterion is one acceptance criterion of a feature.
type Criterion struct {
	ID       string `json:"id"`
	Feature  string `json:"feature"`
	Title    string `json:"title"`
	Probe    string `json:"probe,omitempty"`
	Severity string `json:"severity,omitempty"`
}

// AddInput adds a feature.
type AddInput struct {
	Module string   `json:"module"           jsonschema:"Module the feature belongs to; required"`
	Title  string   `json:"title"            jsonschema:"What someone can do once it exists in one line; required"`
	Parent string   `json:"parent,omitempty" jsonschema:"Object it sits under (a feature for a sub-feature); empty = the module itself"`
	Serves []string `json:"serves,omitempty" jsonschema:"Goal ids it serves"`
}

// AddOutput returns the created feature.
type AddOutput struct {
	Feature Feature `json:"feature"`
	Path    string  `json:"path" jsonschema:"Repo-relative path of the feature's file"`
}

// SplitInput replaces a feature with several.
type SplitInput struct {
	ID string `json:"id" jsonschema:"The feature to split; required"`
	// Actor is who splits — recorded in the source's lifecycle history.
	Actor      cwmapi.Actor     `json:"actor"      jsonschema:"Who splits; required"`
	Successors []SplitSuccessor `json:"successors" jsonschema:"At least two"`
	Reason     string           `json:"reason"     jsonschema:"Why; required — the journal records it with the split"`
}

// SplitSuccessor is one feature replacing the split one.
type SplitSuccessor struct {
	Title    string   `json:"title"              jsonschema:"What someone can do once it exists in one line; required"`
	Children []string `json:"children,omitempty" jsonschema:"Ids of the source's children (criteria or sub-features) it adopts"`
}

// SplitOutput reports the successors (the journal records the split).
type SplitOutput struct {
	Successors []Feature `json:"successors"`
	Orphans    []string  `json:"orphans,omitempty" jsonschema:"Children no successor adopted; they stay on the source"`
}

// RetireInput retires a feature.
type RetireInput struct {
	ID string `json:"id" jsonschema:"Feature id; required"`
	// Actor is who retires — recorded in the lifecycle history.
	Actor  cwmapi.Actor `json:"actor"            jsonschema:"Who retires; required"`
	Reason string       `json:"reason,omitempty" jsonschema:"Why; appended to its body under a Retired heading"`
}

// RetireOutput returns the retired feature.
type RetireOutput struct {
	Feature Feature `json:"feature"`
}

// AttachCriterionInput attaches a criterion to a feature.
type AttachCriterionInput struct {
	Feature  string `json:"feature"            jsonschema:"Feature id; required"`
	Title    string `json:"title"              jsonschema:"The claim in one line; required"`
	Probe    string `json:"probe,omitempty"    jsonschema:"A shell command that exits 0 when the claim holds; empty = not yet measurable"`
	Severity string `json:"severity,omitempty" jsonschema:"core (default) or the severity the method grades by"`
}

// AttachCriterionOutput returns the criterion.
type AttachCriterionOutput struct {
	Criterion Criterion `json:"criterion"`
	Path      string    `json:"path" jsonschema:"Repo-relative path of the criterion's file"`
}

// UncoveredInput selects the features to check.
type UncoveredInput struct {
	Module string `json:"module,omitempty" jsonschema:"Only this module's features; empty = every one"`
}

// UncoveredOutput lists the live features nothing checks.
type UncoveredOutput struct {
	Features []Feature `json:"features"`
}

// GoalsInput names a feature.
type GoalsInput struct {
	ID string `json:"id" jsonschema:"Feature id; required"`
}

// Served is one object a feature serves.
type Served struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

// GoalsOutput lists what the feature serves.
type GoalsOutput struct {
	Goals []Served `json:"goals"`
}

// BoardInput names the object whose features to show (a module: its whole
// subtree; a project: every module) and the page's state filter.
type BoardInput struct {
	ID       string `json:"id"                 jsonschema:"Object id; required"`
	Language string `json:"language,omitempty" jsonschema:"The person's language (en or fr); empty = English"`
	State    string `json:"state,omitempty"    jsonschema:"Only the features in this lifecycle state"`
}

// CoD is a feature's cost of delay: the `cod` its feature capability
// declares, plus the primitives its fields state (FieldsCoD). o is read whole
// for its fields to count.
func CoD(o cwmapi.Object) assistant.CoD {
	c := FieldsCoD(o.FieldsJSON())
	switch v := o.Capabilities[CapFeature]["cod"].(type) {
	case float64:
		c.Declared = int(v)
	case int64:
		c.Declared = int(v)
	case int:
		c.Declared = v
	}
	return c
}

// FieldsCoD is the cost-of-delay primitives an object's fields state:
// business_value, time_criticality, risk_opportunity.
func FieldsCoD(fields json.RawMessage) assistant.CoD {
	var m map[string]any
	if len(fields) > 0 {
		_ = json.Unmarshal(fields, &m)
	}
	num := func(v any) int {
		if f, ok := v.(float64); ok {
			return int(f)
		}
		return 0
	}
	return assistant.CoD{
		BusinessValue:   num(m["business_value"]),
		TimeCriticality: num(m["time_criticality"]),
		RiskOpportunity: num(m["risk_opportunity"]),
	}
}
