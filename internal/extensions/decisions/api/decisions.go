// SPDX-License-Identifier: Apache-2.0

// Package api is the decisions service contract: open a question, weigh
// options, resolve it, supersede it. Every verb is implemented over the CWM
// Object API — the service keeps no storage and no index of its own.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// DecisionType is the object type a decision is stored as.
const DecisionType = "decision"

// CapDecision is the field-group holding a decision's state.
const CapDecision = "decision"

// EdgeSupersedes runs from the newer decision to the one it replaces.
const EdgeSupersedes = "supersedes"

// Whose a decision is: the person's -
// product direction, cost, a customer's commitment, an irreversible choice -
// or an agent's - technical, reversible, inside an approved study. An agent
// takes an agent's with its reasons, traced; the person may reopen it. Only
// the person takes the person's. Unset reads the person's.
const (
	OwnerPerson = "person"
	OwnerAgent  = "agent"
)

// States of the `decision` workflow a decision's lifecycle moves through: open
// → applies (decided, in force) → superseded (replaced by a later decision) or
// retired (withdrawn, nothing replaces it); open → dropped (closed without a
// choice). A decision with no lifecycle state is open.
const (
	StatusOpen       = "open"
	StatusApplies    = "applies"
	StatusSuperseded = "superseded"
	StatusRetired    = "retired"
	StatusDropped    = "dropped"
)

// DecisionsServiceDef describes the service for KTSP/CLI auto-generation.
var DecisionsServiceDef = meta.ServiceDef{
	Name:    "decisions",
	Short:   "A module's decisions: open a question, propose options, resolve, supersede",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Open":              meta.Method("Open a decision: the question a module cannot answer yet"),
		"ProposeOption":     meta.Method("Add an option to an open decision"),
		"Prepare":           meta.Method("Prepare an open decision: its options as short labels, what each means with its pros, cons, cost and consequence, and the option proposed"),
		"Challenge":         meta.Method("Challenge the proposal of an open decision: the points against it"),
		"Resolve":           meta.Method("Decide an open decision with the option(s) chosen; it then applies"),
		"Supersede":         meta.Method("Record that a newer decision replaces one that applies"),
		"Reopen":            meta.Method("Reopen a decision that applies - the person's act, saying why: it is open again, its choice kept in its history"),
		"Record":            meta.Method("Record a decision already taken: a decision that applies, with its original raised and resolved dates"),
		"ListOpen":          meta.Method("List the open decisions of a module"),
		"GridCheck":         {Short: "Whether an evaluation grid can be frozen: its weights add up to 100, it has a scored criterion, its ids are unique", ReadOnly: true},
		"GridFreeze":        meta.Method("Freeze an evaluation grid before anything is scored on it - refused, with the problems, while its check fails"),
		"GridReopen":        meta.Method("Reopen a frozen evaluation grid: back to draft, and every evaluation scored on it back to draft"),
		"EvaluationCompute": meta.Method("Compute a solution evaluation's total on its frozen grid: the weighted total, the eliminatory criteria it fails, the criteria not scored"),
		"EvaluationsMatrix": {Short: "Every alternative scored on an evaluation grid, criterion by criterion, with its total, ranked - the eliminated ones last", ReadOnly: true},
		"Panel":             {Short: "A decision as the page its Decision tab shows: the question, the proposal, its challenge and one card per option with a button choosing it while it is open; who chose what, when and why once decided", ReadOnly: true},
	},
}

// DecisionsService manages decision objects.
type DecisionsService interface {
	Open(ctx context.Context, in OpenInput) (OpenOutput, error)
	ProposeOption(ctx context.Context, in ProposeOptionInput) (ProposeOptionOutput, error)
	Prepare(ctx context.Context, in PrepareInput) (PrepareOutput, error)
	Challenge(ctx context.Context, in ChallengeInput) (ChallengeOutput, error)
	Resolve(ctx context.Context, in ResolveInput) (ResolveOutput, error)
	Supersede(ctx context.Context, in SupersedeInput) (SupersedeOutput, error)
	Reopen(ctx context.Context, in ReopenInput) (ReopenOutput, error)
	Record(ctx context.Context, in RecordInput) (RecordOutput, error)
	ListOpen(ctx context.Context, in ListOpenInput) (ListOpenOutput, error)
	// Panel is the Decision tab's source.
	Panel(ctx context.Context, in PanelInput) (render.Def, error)
	// Evaluation grids and solution evaluations (evaluation.go).
	GridCheck(ctx context.Context, in GridInput) (GridCheckOutput, error)
	GridFreeze(ctx context.Context, in GridMoveInput) (GridCheckOutput, error)
	GridReopen(ctx context.Context, in GridMoveInput) (GridCheckOutput, error)
	EvaluationCompute(ctx context.Context, in EvaluationInput) (EvaluationComputeOutput, error)
	EvaluationsMatrix(ctx context.Context, in MatrixInput) (MatrixOutput, error)
}

// Decision is the service's view of one decision object.
type Decision struct {
	ID       string   `json:"id"`
	Module   string   `json:"module"`
	Question string   `json:"question"`
	Status   string   `json:"status"             jsonschema:"Its lifecycle state: open or applies or superseded or retired or dropped"`
	Options  []string `json:"options,omitempty"`
	Selected []string `json:"selected,omitempty"`
	Raised   string   `json:"raised,omitempty"   jsonschema:"Date the question was raised"`
	Resolved string   `json:"resolved,omitempty" jsonschema:"Date it was decided; empty while open"`
	RaisedBy string   `json:"raised_by,omitempty"`
	Source   string   `json:"source,omitempty"`
	// Proposal and Challenge are the preparation's recommendation and the
	// points raised against it.
	Proposal  *Proposal          `json:"proposal,omitempty"`
	Challenge *ProposalChallenge `json:"challenge,omitempty"`
	// Relayed is set when the person's decision was relayed by an agent's
	// session.
	Relayed *Relayed `json:"relayed,omitempty"`
	// Owner is whose it is: person or agent (OwnerPerson when unset), and
	// OwnerReason why an agent may take it.
	Owner       string `json:"owner"                  jsonschema:"Whose decision it is: person or agent"`
	OwnerReason string `json:"owner_reason,omitempty" jsonschema:"Why an agent may take it"`
}

// Relayed records a person's decision an agent's session relayed: which
// session, how the person confirmed it (prompt or totp or entra) and when.
type Relayed struct {
	By        string `json:"by"           jsonschema:"The agent session that relayed it"`
	Confirmed string `json:"confirmed"    jsonschema:"How the person confirmed it: prompt or totp or entra"`
	At        string `json:"at,omitempty" jsonschema:"When the person confirmed it (RFC 3339)"`
}

// Proposal is the option a preparation recommends and why.
type Proposal struct {
	Option string `json:"option"        jsonschema:"The recommended option; one of the labels"`
	Why    string `json:"why,omitempty" jsonschema:"Why it is recommended; a few lines"`
}

// ProposalChallenge is the points raised against a decision's proposal.
type ProposalChallenge struct {
	By     string   `json:"by"     jsonschema:"Who challenged it"`
	At     string   `json:"at"     jsonschema:"When it was challenged (a date)"`
	Points []string `json:"points" jsonschema:"Each point against the proposal; one line each"`
}

// OpenInput opens a decision.
type OpenInput struct {
	Module   string   `json:"module"              jsonschema:"Module the decision belongs to; required"`
	Question string   `json:"question"            jsonschema:"The question in one line; required"`
	Context  string   `json:"context,omitempty"   jsonschema:"Why it is asked; the prose under the Question heading"`
	Options  []string `json:"options,omitempty"   jsonschema:"Options known so far, as short labels"`
	RaisedBy string   `json:"raised_by,omitempty" jsonschema:"Who raised it (a person, a run, a review)"`
	Source   string   `json:"source,omitempty"    jsonschema:"Where it came from; a short reference"`
	// Owner is whose it is: person (the default) or agent - an agent's says
	// why (OwnerReason).
	Owner       string `json:"owner,omitempty"        jsonschema:"Whose decision it is: person (the default: product direction or cost or a customer's commitment or an irreversible choice) or agent (technical and reversible and inside an approved study)"`
	OwnerReason string `json:"owner_reason,omitempty" jsonschema:"Why an agent may take it; required with owner agent"`
}

// ReopenInput reopens a decision that applies.
type ReopenInput struct {
	ID     string       `json:"id"     jsonschema:"Decision id; required"`
	Actor  cwmapi.Actor `json:"actor"  jsonschema:"Who reopens it - a person; required"`
	Reason string       `json:"reason" jsonschema:"Why it is reopened; required"`
}

// ReopenOutput returns the decision, open again.
type ReopenOutput struct {
	Decision Decision `json:"decision"`
}

// OpenOutput returns the opened decision.
type OpenOutput struct {
	Decision Decision `json:"decision"`
	Path     string   `json:"path" jsonschema:"Repo-relative path of the decision's file"`
}

// ProposeOptionInput adds one option to an open decision.
type ProposeOptionInput struct {
	ID     string `json:"id"     jsonschema:"Decision id; required"`
	Option string `json:"option" jsonschema:"A short label; required"`
}

// ProposeOptionOutput returns the decision with its options.
type ProposeOptionOutput struct {
	Decision Decision `json:"decision"`
}

// OptionDetail is one option as a preparation weighs it: its short label (the
// one Resolve names) and the short prose the Decision panel draws as its card.
type OptionDetail struct {
	Label       string   `json:"label"                 jsonschema:"A short label - a few words in the author's language; required"`
	Means       string   `json:"means,omitempty"       jsonschema:"What choosing it means; one or two lines"`
	Pros        []string `json:"pros,omitempty"        jsonschema:"What speaks for it; one line each"`
	Cons        []string `json:"cons,omitempty"        jsonschema:"What speaks against it; one line each"`
	Cost        string   `json:"cost,omitempty"        jsonschema:"What it costs to do"`
	Consequence string   `json:"consequence,omitempty" jsonschema:"What follows if it is chosen"`
}

// PrepareInput prepares an open decision: its options as a whole and the
// proposal, which names one of them.
type PrepareInput struct {
	ID       string         `json:"id"       jsonschema:"Decision id; required"`
	Options  []OptionDetail `json:"options"  jsonschema:"The options as a whole: they replace the labels and the prepared sections; at least two"`
	Proposal Proposal       `json:"proposal" jsonschema:"The option recommended (one of the labels) and why; required"`
	// Actor is who prepares it.
	Actor cwmapi.Actor `json:"actor" jsonschema:"Who prepares it; required"`
}

// PrepareOutput returns the prepared decision.
type PrepareOutput struct {
	Decision Decision `json:"decision"`
}

// ChallengeInput raises points against the proposal of an open decision.
type ChallengeInput struct {
	ID     string       `json:"id"     jsonschema:"Decision id; required"`
	Actor  cwmapi.Actor `json:"actor"  jsonschema:"Who challenges it; required"`
	Points []string     `json:"points" jsonschema:"Each point against the proposal; one line each; at least one"`
}

// ChallengeOutput returns the challenged decision.
type ChallengeOutput struct {
	Decision Decision `json:"decision"`
}

// PanelInput names the decision whose page to draw.
type PanelInput struct {
	ID       string `json:"id"                 jsonschema:"Decision id; required"`
	Language string `json:"language,omitempty" jsonschema:"The person's language (en or fr); empty = English"`
}

// ResolveInput decides an open decision.
type ResolveInput struct {
	ID string `json:"id" jsonschema:"Decision id; required"`
	// Actor is who decides — recorded in the decision's lifecycle history.
	Actor     cwmapi.Actor `json:"actor"               jsonschema:"Who decides; required"`
	Selected  []string     `json:"selected"            jsonschema:"The option(s) chosen; required, each one of the decision's options when it has any"`
	Rationale string       `json:"rationale,omitempty" jsonschema:"Why; appended under a Decision heading"`
}

// ResolveOutput returns the decision, which now applies.
type ResolveOutput struct {
	Decision Decision `json:"decision"`
}

// SupersedeInput records that By replaces ID.
type SupersedeInput struct {
	ID string `json:"id" jsonschema:"The decision that applies and is being replaced; required"`
	By string `json:"by" jsonschema:"The decision replacing it; required"`
	// Actor is who records the supersession — in the lifecycle history.
	Actor cwmapi.Actor `json:"actor" jsonschema:"Who records it; required"`
}

// SupersedeOutput returns the superseded decision.
type SupersedeOutput struct {
	Decision Decision `json:"decision"`
}

// RecordInput writes down a decision taken before it was recorded here — one
// moved from another record, or taken in a conversation — as a decision that
// APPLIES, that keeps its original dates.
type RecordInput struct {
	Module       string   `json:"module"                 jsonschema:"Module the decision belongs to; required"`
	Question     string   `json:"question"               jsonschema:"The question in one line; required"`
	Context      string   `json:"context,omitempty"      jsonschema:"Why it was asked; the prose under the Question heading"`
	Options      []string `json:"options,omitempty"      jsonschema:"The options that were weighed, as short labels"`
	Selected     []string `json:"selected"               jsonschema:"The option(s) chosen; required, each one of the options when there are any"`
	Rationale    string   `json:"rationale,omitempty"    jsonschema:"Why; under the Decision heading"`
	Consequences string   `json:"consequences,omitempty" jsonschema:"What follows from the choice; under a Consequences heading"`
	Raised       string   `json:"raised"                 jsonschema:"Date the question was raised, YYYY-MM-DD; required"`
	Resolved     string   `json:"resolved,omitempty"     jsonschema:"Date it was decided, YYYY-MM-DD, not before raised; empty = raised"`
	RaisedBy     string   `json:"raised_by,omitempty"    jsonschema:"Who raised it"`
	Source       string   `json:"source,omitempty"       jsonschema:"Where it was recorded before; a moved decision names its original repo-relative path"`
	// Actor is who records it — in the decision's lifecycle history.
	Actor cwmapi.Actor `json:"actor" jsonschema:"Who records it; required"`
}

// RecordOutput returns the recorded decision.
type RecordOutput struct {
	Decision Decision `json:"decision"`
	Path     string   `json:"path" jsonschema:"Repo-relative path of the decision's file"`
}

// ListOpenInput selects a module's open decisions.
type ListOpenInput struct {
	Module string `json:"module" jsonschema:"Module whose open decisions to list; required"`
}

// ListOpenOutput returns them, by id.
type ListOpenOutput struct {
	Decisions []Decision `json:"decisions"`
}
