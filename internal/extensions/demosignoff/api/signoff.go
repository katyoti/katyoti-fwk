// SPDX-License-Identifier: Apache-2.0

// Package api is the contract of the demo sign-off service — the service behind
// ktw-demo's `signoff` capability, the reference extension that uses every
// mechanism a sidecar can give (modules/cwm/objects/
// what-a-capability-and-a-sidecar-give.cwm.md §7). Every URI of the pack
// (app/ktw-demo/embed/cwm) that starts with `:signoff/` names a method here.
//
// A sign-off is a field-group on any object carrying the capability: who
// reviews it, who approved, the verdict and the notes. A type sets how many
// approvals its objects need in its `signoff:` section. The service answers the
// capability's hooks (a refusing before_update, an on_update, a lint, a
// verify, the signoff_digest point the pack opens), its function signedoff(),
// the two panels' sources, and the acts a person runs from the panel.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The names the pack declares, as the service reads them.
const (
	// Capability is the sign-off capability: its group on an object, its section
	// on a type.
	Capability = "signoff"
	// LogCapability is attached to every object carrying `signoff`; it owns the
	// object's sign-off log, a named file.
	LogCapability = "signoff-log"
	// LogFile is the sign-off log's name among the named files of LogCapability.
	LogFile = "log"
	// ItemType is the object type the pack declares.
	ItemType = "signoff-item"
	// EdgeContests points from an object at the object it contests.
	EdgeContests = "contests"
	// DigestPoint is the hook point the pack opens (a hook-type), run on
	// demand with ObjectRunHooks.
	DigestPoint cwmapi.HookPoint = "signoff_digest"
	// SignoffRequestTask is the task the pack offers on sign-off items.
	SignoffRequestTask = "ktw-demo:tasks/signoff-request"
	// ReviewerRowType is the data-type of a reviewer row on the board.
	ReviewerRowType = "ktw-demo:/data-type/signoff-reviewer"
	// ContestFormType is the data-type of the board's contest form.
	ContestFormType = "ktw-demo:/data-type/contest-form"
	// VerdictFormType is the data-type of the board's verdict form.
	VerdictFormType = "ktw-demo:/data-type/signoff-verdict"
	// Methods the pages run.
	ApproveURI = "ktw-demo:signoff/approve"
	ContestURI = "ktw-demo:signoff/contest"
	WriteURI   = "ktw-demo:signoff/write"
)

// Verdicts a sign-off may hold.
const (
	VerdictPending  = "pending"
	VerdictApproved = "approved"
	VerdictChanges  = "changes"
)

// SignoffGroup is the sign-off capability's field-group on an object
// (`capabilities.signoff`). Its data-type, cap-signoff, is rendered from it.
type SignoffGroup struct {
	Reviewers []string `json:"reviewers,omitempty" jsonschema:"title=Reviewers,description=Who reviews the object"`
	Approvals []string `json:"approvals,omitempty" jsonschema:"title=Approvals,description=The reviewers who approved it"`
	Verdict   string   `json:"verdict,omitempty"   jsonschema:"title=Verdict,description=Where the sign-off stands,enum=pending|approved|changes"`
	Notes     string   `json:"notes,omitempty"     jsonschema:"title=Notes,description=What the reviewers said,content=markdown"`
}

// SignoffSettings is the capability's section on a type (`signoff:`). Its
// data-type, type-signoff, is rendered from it.
type SignoffSettings struct {
	RequiredReviewers int `json:"required_reviewers,omitempty" jsonschema:"title=Required reviewers,description=How many approvals an object of the type needs (one when unset)"`
}

// Required is the number of approvals the settings ask for, one at least.
func (s SignoffSettings) Required() int {
	if s.RequiredReviewers < 1 {
		return 1
	}
	return s.RequiredReviewers
}

// ItemFields are the signoff-item type's own fields (`fields:`). Its
// data-type, signoff-item, is rendered from it.
type ItemFields struct {
	Severity string `json:"severity,omitempty" jsonschema:"title=Severity,description=How much the item matters,enum=low|medium|high"`
	Area     string `json:"area,omitempty"     jsonschema:"title=Area,description=The part of the work the item is about"`
}

// ReviewerRow is one row of the board's reviewers table. Its data-type,
// signoff-reviewer, is rendered from it.
type ReviewerRow struct {
	Name     string `json:"name"     jsonschema:"title=Reviewer,description=The reviewer"`
	Approved bool   `json:"approved" jsonschema:"title=Approved,description=Whether they approved"`
}

// ContestRow is one object that contests another: a row of the
// Contests panel. Its data-type, contest-row, is rendered from it.
type ContestRow struct {
	ID      string `json:"id"      jsonschema:"description=The contester"`
	Name    string `json:"name"    jsonschema:"description=Its name"`
	Type    string `json:"type"    jsonschema:"description=Its type"`
	Verdict string `json:"verdict" jsonschema:"description=Its own sign-off verdict (empty when it carries no sign-off)"`
}

// SignoffServiceDef describes the `signoff` slot: the panels, the acts, the
// function, and what CWM asks on a write or on demand — the capability's and
// the policy's hooks, the pack's own point and its bootstrap. A hook may read
// CWM back while CWM waits on it: the bus serves the slots' requests
// concurrently.
var SignoffServiceDef = meta.ServiceDef{
	Name:    "signoff",
	Short:   "Sign-offs of workspace objects: the panels, the acts, the function and the hooks of ktw-demo's sign-off capability",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"SignedoffFunction": {Short: "signedoff() in an expression: the object has the approvals its type requires", ReadOnly: true},
		"Board":             {Short: "The Sign-off panel: the sign-off of an object drawn as a page with its acts", ReadOnly: true},
		"ContestsOf":        {Short: "The Contests panel's rows: the objects that contest an object, with their sign-off", ReadOnly: true},
		"Log":               {Short: "An object's sign-off log: one line per write of its sign-off", ReadOnly: true},
		"Approve":           {Short: "A reviewer approves an object: their name joins its approvals", NoLLM: true},
		"Write":             {Short: "Writes an object's verdict and notes; the before_update hook may refuse it, and the refusal is the answer"},
		"Contest":           {Short: "One object contests another: a contests edge from the contester"},
		"CheckWrite":        {Short: "The before_update hook of an object under sign-off: an approved verdict needs the approvals its type requires", ReadOnly: true},
		"Record":            {Short: "The on_update hook of an object under sign-off: appends the sign-off's state to the object's sign-off log"},
		"Lint":              {Short: "The lint hook of an object under sign-off: what its sign-off still lacks, as findings", ReadOnly: true},
		"Verify":            {Short: "The verify hook of an object under sign-off: pass when approved with its approvals, fail when changes are asked, unproven otherwise", ReadOnly: true},
		"VerifyNotes":       {Short: "The verify hook of the signoff-has-notes policy: a sign-off says why", ReadOnly: true},
		"Digest":            {Short: "The signoff_digest point: the sign-off of an object and how many contest it, in one line", ReadOnly: true},
		"EnsureDesk":        {Short: "The bootstrap hook: a project's demo module has its welcome sign-off item"},
	},
}

// SignoffService answers the `signoff` slot. Every method reaches CWM through
// the CWMService in its context.
type SignoffService interface {
	SignedoffFunction(ctx context.Context, in cwmapi.FunctionCall) (bool, error)
	Board(ctx context.Context, in ObjectInput) (render.Def, error)
	ContestsOf(ctx context.Context, in ObjectInput) ([]ContestRow, error)
	Log(ctx context.Context, in ObjectInput) (LogOutput, error)
	Approve(ctx context.Context, in ApproveInput) (render.Result, error)
	Write(ctx context.Context, in WriteInput) (render.Result, error)
	Contest(ctx context.Context, in ContestInput) (render.Result, error)
	CheckWrite(ctx context.Context, in cwmapi.HookCall) (cwmapi.HookAccept, error)
	Record(ctx context.Context, in cwmapi.HookCall) (RecordOutput, error)
	Lint(ctx context.Context, in cwmapi.HookCall) (LintOutput, error)
	Verify(ctx context.Context, in VerifyInput) (VerifyOutput, error)
	VerifyNotes(ctx context.Context, in VerifyInput) (VerifyOutput, error)
	Digest(ctx context.Context, in cwmapi.HookCall) (LintOutput, error)
	EnsureDesk(ctx context.Context, in EnsureDeskInput) (EnsureOutput, error)
}

// ObjectInput names one object: what a panel's source is called with.
type ObjectInput struct {
	ID       string `json:"id" jsonschema:"description=The object; required"`
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}

// ApproveInput is one reviewer's approval of an object.
type ApproveInput struct {
	ID       string `json:"id"       jsonschema:"description=The object; required"`
	Reviewer string `json:"reviewer" jsonschema:"description=Who approves; one of its reviewers"`
}

// VerdictForm is what the Sign-off panel's form edits: the verdict and the
// notes. Its data-type, signoff-verdict, is rendered from it.
type VerdictForm struct {
	Verdict string `json:"verdict,omitempty" jsonschema:"title=Verdict,description=Where the sign-off stands,enum=pending|approved|changes"`
	Notes   string `json:"notes,omitempty"   jsonschema:"title=Notes,description=What the reviewers said,content=markdown"`
}

// WriteInput is a person's write of an object's verdict and notes.
type WriteInput struct {
	ID      string `json:"id"                jsonschema:"description=The object; required"`
	Verdict string `json:"verdict,omitempty" jsonschema:"description=The verdict,enum=pending|approved|changes"`
	Notes   string `json:"notes,omitempty"   jsonschema:"description=The notes,content=markdown"`
}

// ContestInput says that one object contests another.
type ContestInput struct {
	ID        string `json:"id"         jsonschema:"description=The object contested; required"`
	Contester string `json:"contester" jsonschema:"description=The object that contests it; required,content=object-ref"`
}

// VerifyInput is a verify hook's call: the default {point, object, prior}
// (the capability's entry), or the policy's declared input {id, rule}.
type VerifyInput struct {
	Point  cwmapi.HookPoint `json:"point,omitempty"`
	Object *cwmapi.Object   `json:"object,omitempty"`
	ID     string           `json:"id,omitempty"   jsonschema:"description=The object when no object is given"`
	Rule   string           `json:"rule,omitempty" jsonschema:"description=The rule to check: approvals (the default) or notes"`
}

// Verify statuses: only pass is a pass; anything else is unproven.
const (
	StatusPass     = "pass"
	StatusFail     = "fail"
	StatusUnproven = "unproven"
)

// VerifyOutput is a verify hook's answer.
type VerifyOutput struct {
	Status   string `json:"status"             jsonschema:"description=pass or fail or unproven"`
	Evidence string `json:"evidence,omitempty" jsonschema:"description=Why"`
}

// LintFinding is one finding of the lint hook.
type LintFinding struct {
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Severity string `json:"severity" jsonschema:"description=error or warn or info"`
}

// LintOutput is a findings answer (the lint and signoff_digest points).
type LintOutput struct {
	Status   string        `json:"status" jsonschema:"description=ok or findings"`
	Findings []LintFinding `json:"findings"`
}

// The rules the lint names.
const (
	RuleNoReviewer          = "no-reviewer"
	RuleChangesWithoutNotes = "changes-without-notes"
	RuleApprovalNotReviewer = "approval-not-a-reviewer"
	RuleDigest              = "signoff-digest"
)

// RecordOutput is what the on_update hook wrote.
type RecordOutput struct {
	Line string `json:"line"`
}

// LogOutput is an object's sign-off log.
type LogOutput struct {
	ID    string   `json:"id"`
	Lines []string `json:"lines"`
}

// EnsureDeskInput is the bootstrap hook's declared input: the module whose
// welcome sign-off item is ensured.
type EnsureDeskInput struct {
	Module string `json:"module" jsonschema:"description=The module that holds the welcome sign-off item; required"`
}

// WelcomeLocal is the local id of a module's welcome sign-off item.
const WelcomeLocal = "signoff-welcome"

// EnsureOutput says what the bootstrap hook found or created.
type EnsureOutput struct {
	ID      string `json:"id,omitempty"`
	Created bool   `json:"created"`
}
