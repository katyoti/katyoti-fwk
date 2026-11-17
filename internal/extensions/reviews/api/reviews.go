// SPDX-License-Identifier: Apache-2.0

// Package api is the contract of the reviews extension: a review is one review
// type run on what it reviews (a module, any object carrying the `reviewable`
// capability) at one time on one state of the code — its type, when, the
// commit it read, what triggered it, its reviewers, its rating, an optional
// score and its summary, its body the report. Its findings are objects filed
// under the object reviewed, each with a severity, a statement, a fingerprint
// and a state of its own: the review that raised it (`found-in`) and the
// latest review that saw it (`seen-in`) point from it, so a finding stays one
// object across the reviews of its type. A review also holds the documents,
// the slide decks and the overview written for it.
//
// The pack (contrib/embed/cwm) declares the types, the capability, the two
// workflows and the two panels; this service answers the panels' sources.
// Everything is read through the Object API: the service keeps no storage.
package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// The names the pack declares, as the service reads them.
const (
	// Capability lets an object have reviews: the Reviews panel shows on it.
	Capability = "reviewable"
	// ReviewType is a dated review of a module or an object.
	ReviewType = "review"
	// FindingType is one finding of a review, a child of it.
	FindingType = "finding"
	// DocumentType and DeckType are the other children a review holds.
	DocumentType = "document"
	DeckType     = "slide-deck"
	// EdgeAbout points from a finding at the object it is about (CWM's own
	// edge kind).
	EdgeAbout = "about"
	// EdgeFoundIn points from a finding at the review that raised it.
	EdgeFoundIn = "found-in"
	// EdgeSeenIn points from a finding at the latest review that saw it.
	EdgeSeenIn = "seen-in"
)

// Ratings a review may hold, best first.
const (
	RatingGood           = "good"
	RatingNeedsAttention = "needs-attention"
	RatingCritical       = "critical"
)

// What started a review.
const (
	TriggerManual    = "manual"
	TriggerAutomatic = "automatic"
)

// Severities a finding may hold, most severe first.
var Severities = []string{"critical", "high", "medium", "low", "info"}

// Finding states (the finding workflow).
const (
	FindingOpen      = "open"
	FindingAddressed = "addressed"
	FindingClosed    = "closed"
	FindingAccepted  = "accepted"
)

// ReviewFields are the review type's own fields (`fields:`). Its data-type,
// review, is rendered from it.
type ReviewFields struct {
	Type        string   `json:"type,omitempty"         jsonschema:"title=Review type,description=The review type it ran: the name of a review-type contribution"`
	Scope       string   `json:"scope,omitempty"        jsonschema:"title=Scope,description=What the review looked at: a module or an object or a path"`
	ReviewedAt  string   `json:"reviewed_at,omitempty"  jsonschema:"title=Reviewed at,description=When the review was held,content=date-time"`
	Commit      string   `json:"commit,omitempty"       jsonschema:"title=Commit,description=The commit of the code the review read"`
	Trigger     string   `json:"trigger,omitempty"      jsonschema:"title=Trigger,description=What started it: a person or the request whose change it reviewed,enum=manual|automatic"`
	RequestedBy string   `json:"requested_by,omitempty" jsonschema:"title=Requested by,description=Who asked for it (manual) or the request that set it off (automatic)"`
	Run         string   `json:"run,omitempty"          jsonschema:"title=Run,description=The run it belongs to: the reviews started together share it"`
	Reviewers   []string `json:"reviewers,omitempty"    jsonschema:"title=Reviewers,description=Who reviewed: people or agents"`
	Rating      string   `json:"rating,omitempty"       jsonschema:"title=Rating,description=The overall judgement,enum=good|needs-attention|critical"`
	Score       *int     `json:"score,omitempty"        jsonschema:"title=Score,description=A score from 0 to 100 when the review type defines one"`
	Summary     string   `json:"summary,omitempty"      jsonschema:"title=Summary,description=The rating in a few sentences,content=markdown"`
}

// FindingFields are the finding type's own fields. Its data-type, finding, is
// rendered from it.
type FindingFields struct {
	Severity       string `json:"severity,omitempty"       jsonschema:"title=Severity,description=How much it matters,enum=critical|high|medium|low|info"`
	Statement      string `json:"statement,omitempty"      jsonschema:"title=Statement,description=What was found in one sentence"`
	Location       string `json:"location,omitempty"       jsonschema:"title=Location,description=Where it was found: a file and its lines or a section"`
	Recommendation string `json:"recommendation,omitempty" jsonschema:"title=Recommendation,description=What to do about it,content=markdown"`
	FoundBy        string `json:"found_by,omitempty"       jsonschema:"title=Found by,description=The review type that surfaced it"`
	Fingerprint    string `json:"fingerprint,omitempty"    jsonschema:"title=Fingerprint,description=What makes it the same finding from one review to the next: its review type and location and rule"`
}

// Fingerprint is the identity of a finding across the reviews of one type:
// the review type, where it was found and the rule or statement that names
// it, normalised (case and spacing), hashed short. Two reviews that find the
// same problem at the same place give the same fingerprint.
func Fingerprint(reviewType, location, key string) string {
	norm := func(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }
	sum := sha256.Sum256([]byte(norm(reviewType) + "|" + norm(location) + "|" + norm(key)))
	return hex.EncodeToString(sum[:8])
}

// ReviewsServiceDef describes the `reviews` slot: the sources of the two
// panels, and the review types an object can be reviewed by.
var ReviewsServiceDef = meta.ServiceDef{
	Name:    "reviews",
	Short:   "Reviews of modules and objects: the Reviews panel of a reviewable object and the Review panel of a review",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"ReviewsOf": {Short: "The Reviews panel: the reviews of an object, newest first, each with its rating and its open findings by severity", ReadOnly: true},
		"Review":    {Short: "The Review panel: a review's rating, its findings by severity, its documents and slide decks", ReadOnly: true},
		"TypesFor":  {Short: "The review types that apply to an object, grouped by dimension, and the presets they form", ReadOnly: true},
		"Supersede": {Short: "After a review is filed: keep it and the one before it of its type, fold the older ones into the object's history and remove them"},
		"Overview":  {Short: "The Reviews overview panel: one row per module, a cell per review dimension (worst current rating, trend, age), open findings by severity", ReadOnly: true},
		"File":      {Short: "File one review type's result on the object reviewed: the review, its findings matched by fingerprint (new, persisting, resolved), then supersede"},
		"Start":     {Short: "Start a review of an object: the review types chosen by name, preset or dimension, each run by its check (filed at once) or its agent (review-run)", TimeoutS: 3600},
	},
}

// ReviewsService answers the `reviews` slot. Every method reaches CWM through
// the CWMService in its context.
type ReviewsService interface {
	ReviewsOf(ctx context.Context, in ObjectInput) (render.Def, error)
	Review(ctx context.Context, in ObjectInput) (render.Def, error)
	// TypesFor lists the review types (the review-type contributions of the
	// universe) whose applies selects the object, grouped by dimension.
	TypesFor(ctx context.Context, in TypesInput) (TypesOutput, error)
	// Supersede keeps the review in.ID and the one before it of its type
	// under the object reviewed; each older one is folded into the object's
	// reviewable history, then removed with its children.
	Supersede(ctx context.Context, in SupersedeInput) (SupersedeOutput, error)
	// Overview is the Reviews overview panel: every module (in.ID empty or not
	// a module) or in.ID and the modules under it.
	Overview(ctx context.Context, in ObjectInput) (render.Def, error)
	// File files one review type's result: a review under the object, each
	// finding matched to the open one of its type with the same fingerprint
	// (seen again) or filed new, the open ones not seen closed; then Supersede.
	File(ctx context.Context, in FileInput) (FileOutput, error)
	// Start runs the review types chosen on the object: a check is run and
	// filed at once, an agent type starts the review-run task.
	Start(ctx context.Context, in StartInput) (StartOutput, error)
}

// ObjectInput names one object: what a panel's source is called with.
type ObjectInput struct {
	ID       string `json:"id" jsonschema:"description=The object; required"`
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}
