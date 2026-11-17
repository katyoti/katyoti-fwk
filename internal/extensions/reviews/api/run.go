// SPDX-License-Identifier: Apache-2.0

package api

// ReviewResult is what every runner of a review type answers — an agent
// following the type's skill (its output contract) or a check action: the
// rating, an optional score, the summary, the report (the review's body) and
// the findings. A finding's key names its rule or what it is about; with the
// review type and the location it makes the finding's fingerprint.
type ReviewResult struct {
	Rating   string          `json:"rating"             jsonschema:"title=Rating,description=The overall judgement,enum=good|needs-attention|critical"`
	Score    *int            `json:"score,omitempty"    jsonschema:"title=Score,description=A score from 0 to 100 when the review type defines one"`
	Summary  string          `json:"summary"            jsonschema:"title=Summary,description=The rating in a few sentences"`
	Report   string          `json:"report"             jsonschema:"title=Report,description=The full report: what was looked at and what was found and what to do next,content=markdown"`
	Findings []ResultFinding `json:"findings,omitempty" jsonschema:"title=Findings,description=What was found: one entry each"`
	// Coverage is how much of the subject the run checked: empty or all =
	// the whole of it; partial = a part (a change's packages), so an open
	// finding it did not see stays open; none = nothing, so no review is
	// filed and the previous one stands.
	Coverage string `json:"coverage,omitempty" jsonschema:"title=Coverage,description=How much of the subject the run checked: all (empty) - partial (a part: an open finding it did not see stays open) - none (nothing: no review is filed),enum=all|partial|none"`
}

// The coverages of a result (ReviewResult.Coverage); empty is all.
const (
	CoverageAll     = "all"
	CoveragePartial = "partial"
	CoverageNone    = "none"
)

// ResultFinding is one finding of a result.
type ResultFinding struct {
	Severity       string `json:"severity"                 jsonschema:"title=Severity,description=How much it matters,enum=critical|high|medium|low|info"`
	Statement      string `json:"statement"                jsonschema:"title=Statement,description=What was found in one sentence"`
	Location       string `json:"location,omitempty"       jsonschema:"title=Location,description=Where: a file and its lines or a section"`
	Recommendation string `json:"recommendation,omitempty" jsonschema:"title=Recommendation,description=What to do about it"`
	Key            string `json:"key,omitempty"            jsonschema:"title=Key,description=The rule or the subject it is about: stable from one review to the next (empty = the statement)"`
	Evidence       string `json:"evidence,omitempty"       jsonschema:"title=Evidence,description=The evidence in full,content=markdown"`
}

// FileInput files one review type's result on the object reviewed.
type FileInput struct {
	Object      string       `json:"object"                 jsonschema:"description=The object reviewed; required"`
	Type        string       `json:"type"                   jsonschema:"description=The review type that ran; required"`
	Run         string       `json:"run,omitempty"          jsonschema:"description=The run it belongs to"`
	Trigger     string       `json:"trigger,omitempty"      jsonschema:"description=manual or automatic,enum=manual|automatic"`
	RequestedBy string       `json:"requested_by,omitempty" jsonschema:"description=Who asked or the request that set it off"`
	Commit      string       `json:"commit,omitempty"       jsonschema:"description=The commit the review read"`
	Reviewers   []string     `json:"reviewers,omitempty"    jsonschema:"description=Who reviewed: the agent or the check"`
	Result      ReviewResult `json:"result"                 jsonschema:"description=What the runner answered; required"`
}

// FileOutput names the review filed and what became of the findings.
type FileOutput struct {
	Review     string          `json:"review"`
	New        []string        `json:"new,omitempty"        jsonschema:"description=Findings raised by this review"`
	Persisting []string        `json:"persisting,omitempty" jsonschema:"description=Open findings this review saw again"`
	Resolved   []string        `json:"resolved,omitempty"   jsonschema:"description=Open findings of the type this review no longer saw: closed"`
	Supersede  SupersedeOutput `json:"supersede"`
	// Files is every file the filing wrote or removed, the fold's included:
	// what a commit of the review must carry.
	Files []string `json:"files,omitempty" jsonschema:"description=The project-relative files the filing wrote or removed (the fold's included); sorted"`
	// Skipped says why nothing was filed: the result checked nothing.
	Skipped string `json:"skipped,omitempty" jsonschema:"description=Why nothing was filed: the result checked nothing (coverage none) - the previous review and its open findings stand"`
}

// StartInput starts a review of an object: the review types chosen by name,
// by preset or by dimension (any of them; all three empty = every manual type
// that applies). A run over the cost threshold asks to be confirmed.
type StartInput struct {
	Object      string   `json:"object"                 jsonschema:"description=The object to review; required"`
	Types       []string `json:"types,omitempty"        jsonschema:"description=Review types by name"`
	Preset      string   `json:"preset,omitempty"       jsonschema:"description=A preset: every type of it that applies"`
	Dimension   string   `json:"dimension,omitempty"    jsonschema:"description=A dimension: every type of it that applies"`
	Trigger     string   `json:"trigger,omitempty"      jsonschema:"description=manual (default) or automatic,enum=manual|automatic"`
	RequestedBy string   `json:"requested_by,omitempty" jsonschema:"description=Who asks or the request whose change sets it off"`
	Commit      string   `json:"commit,omitempty"       jsonschema:"description=The commit to review (empty = unrecorded)"`
	Confirm     bool     `json:"confirm,omitempty"      jsonschema:"description=Start even when the estimated cost is over the threshold"`
	// Kind and Paths are the change an automatic run follows: the request's
	// kind and the paths it changed, matched against each type's trigger.
	Kind  string   `json:"kind,omitempty"  jsonschema:"description=An automatic run: the kind of the request whose change sets it off (build or fix or another)"`
	Paths []string `json:"paths,omitempty" jsonschema:"description=An automatic run: the paths the change touched (slash-separated - relative to the project)"`
}

// StartOutput says what was started: the run, each type with how it runs
// (a check filed at once, an agent run started), or — over the threshold and
// not confirmed — nothing, with the estimate to confirm.
type StartOutput struct {
	Run          string        `json:"run,omitempty"`
	EstimateUSD  float64       `json:"estimate_usd"`
	NeedsConfirm bool          `json:"needs_confirm,omitempty" jsonschema:"description=Over the threshold: start again with confirm"`
	Started      []StartedType `json:"started,omitempty"`
}

// StartedType is one review type of a run.
type StartedType struct {
	Type    string   `json:"type"`
	Object  string   `json:"object,omitempty"  jsonschema:"description=The object it reviews when its trigger's owners chose it (empty = the object the run was started on)"`
	Review  string   `json:"review,omitempty"  jsonschema:"description=A check's review filed at once"`
	Files   []string `json:"files,omitempty"   jsonschema:"description=The project-relative files that filing wrote or removed (its fold included): what a commit of the review carries"`
	TaskID  string   `json:"task_id,omitempty" jsonschema:"description=An agent's run"`
	Skipped string   `json:"skipped,omitempty" jsonschema:"description=Why a check filed nothing: it checked nothing"`
	Error   string   `json:"error,omitempty"`
}

// The tasks the reviews extension runs (app/ktw-agents/embed/tasks): one
// review type performed by an agent then filed.
const (
	ReviewRunTask  = "review-run"
	ConfirmOverUSD = 5.0
)
