// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	reviewsapi "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// GocodeService is the orchestration + item-level service for
// ktw-gocode. Auto-published via AutoKTSPContainer under the
// `gocode.*` slot namespace. CLI / KTSP / agent-tool / HTTP
// surfaces are derived.
//
// All methods follow the context-first signature. Verb naming
// is <Slot><Verb> (ModuleVerify, UCCreate, ODOpen, …) so the
// auto-CLI can group them per the SlotKey convention.
type GocodeService interface {
	// module.* — orchestration verbs (8; CharterPublish + ManifestWrite
	// moved to CodeService — Track B #4 phase 1)
	ModuleCreate(ctx context.Context, in ModuleCreateInput) (ModuleCreateOutput, error)
	ModuleDiscover(ctx context.Context, in ModuleDiscoverInput) (ModuleDiscoverOutput, error)
	ModuleStatus(ctx context.Context, in ModuleStatusInput) (ModuleStatusOutput, error)
	// ModuleDashboard answers the module's status as a page (render.Def): the
	// declared Dashboard panel shows it as is.
	ModuleDashboard(ctx context.Context, in ModuleDashboardInput) (ModuleDashboardOutput, error)
	ModulePlanTier(ctx context.Context, in ModulePlanTierInput) (ModulePlanTierOutput, error)
	ModuleVerify(ctx context.Context, in ModuleVerifyInput) (ModuleVerifyOutput, error)
	ModuleAudit(ctx context.Context, in ModuleAuditInput) (ModuleAuditOutput, error)
	// ModuleReviewCheck is the go-checks review type's check: the automated
	// perspectives and the lint as a review result, filed by the reviews
	// service.
	ModuleReviewCheck(ctx context.Context, in ModuleReviewCheckInput) (ModuleReviewCheckOutput, error)
	ModuleLint(ctx context.Context, in ModuleLintInput) (ModuleLintOutput, error)
	ModulePromote(ctx context.Context, in ModulePromoteInput) (ModulePromoteOutput, error)

	// uc.* — use-case item verbs (4)
	UCCreate(ctx context.Context, in UCCreateInput) (UCCreateOutput, error)
	UCUpdate(ctx context.Context, in UCUpdateInput) (UCUpdateOutput, error)
	UCSetStatus(ctx context.Context, in UCSetStatusInput) (UCSetStatusOutput, error)
	UCProposeFromTests(ctx context.Context, in UCProposeFromTestsInput) (UCProposeFromTestsOutput, error)

	// feature.* — feature reverse-engineering (1)
	FeatureReverseEngineer(ctx context.Context, in FeatureReverseEngineerInput) (FeatureReverseEngineerOutput, error)
	FeatureRealizationStatus(ctx context.Context, in FeatureRealizationStatusInput) (FeatureRealizationStatusOutput, error)

	// tasktest.* — L2 simulator scenario authoring + execution (2)
	TasktestCreate(ctx context.Context, in TasktestCreateInput) (TasktestCreateOutput, error)
	TasktestRun(ctx context.Context, in TasktestRunInput) (TasktestRunOutput, error)

	// module.Orchestrate — autonomous run (v0: fire-and-forget) (1)
	ModuleOrchestrate(ctx context.Context, in ModuleOrchestrateInput) (ModuleOrchestrateOutput, error)
}

// ============================================================================
// module.Create
// ============================================================================

type ModuleCreateInput struct {
	Name        string     `json:"name"                       jsonschema:"Module slug (lowercase, hyphens)"`
	ProjectID   string     `json:"project_id"                 jsonschema:"Parent project-node UUID"`
	Kind        ModuleKind `json:"kind"                       jsonschema:"framework | library | sidecar"`
	Description string     `json:"description,omitempty"`
	Facets      []Facet    `json:"facets,omitempty"           jsonschema:"Initial typed paths (code/app/...; may be empty — module.discover fills later)"`
	TierTarget  int        `json:"tier_target,omitempty"      jsonschema:"Goal tier; default 1"`
	Scaffold    bool       `json:"scaffold,omitempty"         jsonschema:"If true, create the public/internal/local Go-package skeleton on disk"`
}

type ModuleCreateOutput struct {
	Module Module `json:"module"`
}

// ============================================================================
// module.Discover — retro-engineering entry point
// ============================================================================

type ProposedUseCase struct {
	Title    string     `json:"title"`
	Severity UCSeverity `json:"severity"`
	BodyMD   string     `json:"body_md,omitempty"`
	HintedBy []string   `json:"hinted_by,omitempty" jsonschema:"Code-symbols or test names that suggested this UC"`
}

type ProposedTestCase struct {
	TCID    string  `json:"tc_id"`
	Layer   UCLayer `json:"layer"`
	TestRef string  `json:"test_ref"`
}

type ProposedDependency struct {
	FromModule string `json:"from_module"`
	ToModule   string `json:"to_module"`
}

type ModuleDiscoverInput struct {
	Path                  string `json:"path"                              jsonschema:"Root path to scan (repo-relative or absolute)"`
	TargetModuleID        string `json:"target_module_id,omitempty"        jsonschema:"Existing module to extend; empty creates a new module"`
	AutoCommitCodeSymbols bool   `json:"auto_commit_code_symbols,omitempty" jsonschema:"Default true: AST-derived items are auto-written"`
	ProposeUseCases       bool   `json:"propose_use_cases,omitempty"        jsonschema:"Default true: propose UCs for review (not auto-written)"`
	ProposeTestCases      bool   `json:"propose_test_cases,omitempty"       jsonschema:"Default true: propose test-cases from existing Test* funcs"`
	DryRun                bool   `json:"dry_run,omitempty"                  jsonschema:"If true, return proposals without writing anything"`
}

type ModuleDiscoverOutput struct {
	ModuleID             string               `json:"module_id"                  jsonschema:"Existing or newly-created module"`
	CommittedCodeSymbols int                  `json:"committed_code_symbols"     jsonschema:"Count of AST-derived items auto-written"`
	ProposedUseCases     []ProposedUseCase    `json:"proposed_use_cases,omitempty"`
	ProposedTestCases    []ProposedTestCase   `json:"proposed_test_cases,omitempty"`
	ProposedDependencies []ProposedDependency `json:"proposed_dependencies,omitempty"`
	SuggestedKind        ModuleKind           `json:"suggested_kind,omitempty"   jsonschema:"From layout signals; user confirms"`
	Warnings             []string             `json:"warnings,omitempty"`
}

// ============================================================================
// module.Status — read-only state report
// ============================================================================

type ModuleStatusInput struct {
	ID string `json:"id" jsonschema:"Module-node UUID"`
}

type ModuleStatusOutput struct {
	Module        Module        `json:"module"`
	UCTotals      TestRunTotals `json:"uc_totals"                       jsonschema:"Latest test-run totals"`
	OpenODCount   int           `json:"open_od_count"`
	AuditCritOpen int           `json:"audit_critical_open"`
	AuditHighOpen int           `json:"audit_high_open"`
	BlockingGates []string      `json:"blocking_gates,omitempty"        jsonschema:"Tier-promote gate conditions currently unmet"`
}

// ============================================================================
// module.Dashboard — the status as a page
// ============================================================================

type ModuleDashboardInput struct {
	ID string `json:"id" jsonschema:"Module-node UUID"`
}

// ModuleDashboardOutput is a render definition (kind page): it flattens into
// the answer so the panel that calls it shows it as is.
type ModuleDashboardOutput struct {
	render.Def
}

// ============================================================================
// module.PlanTier — compute tier-N gaps, file tasks
// ============================================================================

type ProposedTask struct {
	Title    string `json:"title"`
	Purpose  string `json:"purpose"     jsonschema:"feature | bug | refactor | infra | spike"`
	Closes   string `json:"closes,omitempty"   jsonschema:"UC-id or finding-id this task addresses"`
	Estimate string `json:"estimate,omitempty" jsonschema:"T-shirt size"`
}

type ModulePlanTierInput struct {
	ID         string `json:"id"`
	TargetTier int    `json:"target_tier"           jsonschema:"Tier to plan toward; defaults to module.tier_target"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

type ModulePlanTierOutput struct {
	ProposedTasks []ProposedTask `json:"proposed_tasks,omitempty"`
	GapSummary    string         `json:"gap_summary"           jsonschema:"Human-readable gap report"`
}

// ============================================================================
// module.Verify — UC verification (writes a test-run item)
// ============================================================================

type ModuleVerifyInput struct {
	ID              string `json:"id"`
	LayerFilter     string `json:"layer_filter,omitempty"   jsonschema:"L1 | L2 | L3 | L1+L2 | all (default)"`
	UCFilter        string `json:"uc_filter,omitempty"      jsonschema:"Optional UC-id pattern (UC-04*, UC-053..UC-058)"`
	Severity        string `json:"severity,omitempty"       jsonschema:"core | security | core+security | all (default)"`
	BailOnFirstFail bool   `json:"bail_on_first_fail,omitempty"`
	Archive         bool   `json:"archive,omitempty"        jsonschema:"If true, write results to <modules/m>/results/<date>/"`
}

type ModuleVerifyOutput struct {
	TestRunID   string        `json:"test_run_id"            jsonschema:"UUID of the test-run item written"`
	Totals      TestRunTotals `json:"totals"`
	TierNDoDMet bool          `json:"tier_n_dod_met"`
	Health      ModuleHealth  `json:"health,omitempty"       jsonschema:"3-state module health derived from totals + open critical/high audit findings"`
	Blockers    []string      `json:"blockers,omitempty"`
}

// ============================================================================
// module.Audit — 13-perspective code review
// ============================================================================

type ModuleAuditInput struct {
	ID      string `json:"id"`
	Archive bool   `json:"archive,omitempty"        jsonschema:"If true, write to modules/<m>/audits/<date>/"`
}

// ModuleReviewCheckInput is what the reviews service gives a check: the object
// reviewed, the review type, and the type's runner input.
type ModuleReviewCheckInput struct {
	Object string         `json:"object"          jsonschema:"The module reviewed"`
	Type   string         `json:"type,omitempty"  jsonschema:"The review type (go-checks)"`
	Input  map[string]any `json:"input,omitempty" jsonschema:"The review type's runner input"`
	// Paths are a change's paths (an automatic review of a request): the
	// checks run on the module's packages they touch, not the whole module.
	Paths []string `json:"paths,omitempty" jsonschema:"The paths a change touched; the checks run on the module packages they touch (empty = the whole module)"`
}

// ModuleReviewCheckOutput is the review result (reviewsapi.ReviewResult).
type ModuleReviewCheckOutput struct {
	reviewsapi.ReviewResult
}

type ModuleAuditOutput struct {
	ReviewID      string `json:"review_id"                 jsonschema:"The review of the module the audit wrote"`
	CriticalCount int    `json:"critical_count"`
	HighCount     int    `json:"high_count"`
	MediumCount   int    `json:"medium_count"`
	LowCount      int    `json:"low_count"`
}

// ============================================================================
// module.Lint
// ============================================================================

type ModuleLintInput struct {
	ID        string   `json:"id"`
	Detectors []string `json:"detectors,omitempty"     jsonschema:"Subset of detectors to run; empty = all"`
}

type ModuleLintOutput struct {
	Violations []LintViolation `json:"violations,omitempty"`
	ErrorCount int             `json:"error_count"        jsonschema:"Count at severity=error — blocks tier promote"`
	WarnCount  int             `json:"warn_count"`
	InfoCount  int             `json:"info_count"`
}

// ============================================================================
// module.Promote — tier flip with gate enforcement
// ============================================================================

type ModulePromoteInput struct {
	ID         string `json:"id"`
	TargetTier int    `json:"target_tier,omitempty"   jsonschema:"Tier to flip to; defaults to module.tier_target"`
	Force      bool   `json:"force,omitempty"         jsonschema:"Override unmet gates (records the bypass as a decision that applies)"`
}

type ModulePromoteOutput struct {
	NewTier  int      `json:"new_tier"`
	Forced   bool     `json:"forced,omitempty"`
	GatesMet []string `json:"gates_met,omitempty"`
	Blockers []string `json:"blockers,omitempty"      jsonschema:"Empty on success; populated on ErrPromoteGate"`
}

// ============================================================================
// uc.* — use-case CRUD + retro-engineering
// ============================================================================

type UCCreateInput struct {
	NodeID       string     `json:"node_id"            jsonschema:"Owning module-node UUID"`
	UCID         string     `json:"uc_id"              jsonschema:"Stable id (UC-001, UC-053)"`
	Title        string     `json:"title"`
	Severity     UCSeverity `json:"severity"`
	BodyMD       string     `json:"body_md,omitempty"  jsonschema:"DoD-per-UC markdown"`
	LayerTargets []UCLayer  `json:"layer_targets,omitempty"`
}

type UCCreateOutput struct {
	UseCase UseCase `json:"use_case"`
}

type UCUpdateInput struct {
	ID           string      `json:"id"                     jsonschema:"UseCase item UUID"`
	Title        *string     `json:"title,omitempty"        jsonschema:"Nil = leave unchanged"`
	Severity     *UCSeverity `json:"severity,omitempty"`
	BodyMD       *string     `json:"body_md,omitempty"`
	LayerTargets []UCLayer   `json:"layer_targets,omitempty"`
}

type UCUpdateOutput struct {
	UseCase UseCase `json:"use_case"`
}

// UCSetStatus is the ONLY write-path for use-case.status_cache.
// Lint detector StatusCacheDrift flags any other writer.
type UCSetStatusInput struct {
	ID             string    `json:"id"`
	Status         UCStatus  `json:"status"`
	FailDeferredBy string    `json:"fail_deferred_by,omitempty"`
	TestRunID      string    `json:"test_run_id"            jsonschema:"Required for non-fail-deferred writes; identifies the verify run"`
	LastVerifiedAt time.Time `json:"last_verified_at"`
}

type UCSetStatusOutput struct {
	UseCase UseCase `json:"use_case"`
}

type UCProposeFromTestsInput struct {
	ModuleID string `json:"module_id"           jsonschema:"Module to scan"`
	DryRun   bool   `json:"dry_run,omitempty"   jsonschema:"If true, return proposals without writing"`
	AutoLink bool   `json:"auto_link,omitempty" jsonschema:"If true, also write verifies edges for matched test-cases"`
}

type UCProposeFromTestsOutput struct {
	Proposed []ProposedUseCase `json:"proposed,omitempty"`
}

// ============================================================================
// feature.* — reverse-engineering
// ============================================================================

type ProposedFeature struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Modules     []string `json:"modules,omitempty"     jsonschema:"Modules whose Use Cases this proposed feature would want"`
	Confidence  string   `json:"confidence"            jsonschema:"high | medium | low"`
}

type FeatureReverseEngineerInput struct {
	ProjectID string `json:"project_id"            jsonschema:"Project-node UUID to analyze"`
	DryRun    bool   `json:"dry_run,omitempty"     jsonschema:"If true, return proposals without writing"`
}

type FeatureReverseEngineerOutput struct {
	Proposed []ProposedFeature `json:"proposed,omitempty"`
}

// FeatureRealizationStatusInput requests the souhait-side derived
// status for a feature — walks the feature's outgoing `wants` edges
// to its Use Cases, reads each UC's status_cache, derives a per-UC
// status, and returns the per-UC breakdown + the US-level worst-of
// aggregate (method-core §1/§3). Compute-on-read, no caching.
type FeatureRealizationStatusInput struct {
	FeatureID string `json:"feature_id" jsonschema:"User-story node UUID; required"`
}

// RealizingUCStatus is the per-wanted-UC breakdown row.
type RealizingUCStatus struct {
	UCID    string `json:"uc_id"             jsonschema:"Wanted Use Case item UUID"`
	UCTitle string `json:"uc_title,omitempty" jsonschema:"Convenience field — the UC's title"`
	Status  string `json:"status"            jsonschema:"not-started | in-progress | verifying | regressed | done"`
}

// FeatureRealizationStatusOutput carries the per-wanted-UC view + the
// worst-of aggregate.
type FeatureRealizationStatusOutput struct {
	FeatureID string              `json:"feature_id"`
	Aggregate string              `json:"aggregate" jsonschema:"Worst across wanted UCs; 'not-started' when the US wants no UC"`
	PerUC     []RealizingUCStatus `json:"per_uc,omitempty"`
}

// ============================================================================
// tasktest.* — L2 simulator scenarios
// ============================================================================

type TasktestCreateInput struct {
	UCID      string `json:"uc_id"               jsonschema:"UC the scenario will verify (UC-NNN)"`
	ModuleID  string `json:"module_id"           jsonschema:"Owning module-node UUID"`
	OutputDir string `json:"output_dir,omitempty" jsonschema:"Default modules/<m>/tests/L2/"`
}

type TasktestCreateOutput struct {
	Path       string `json:"path"               jsonschema:"Filesystem path to the new .tasktest file"`
	TestCaseID string `json:"test_case_id"       jsonschema:"UUID of the auto-written test-case item"`
}

type TasktestRunInput struct {
	Path string `json:"path"                          jsonschema:"Filesystem path to a .tasktest file"`
}

type TasktestRunOutput struct {
	Status       TestRunStatus `json:"status"`
	DurationMS   int64         `json:"duration_ms"`
	JournalLines int           `json:"journal_lines,omitempty"`
	Failure      string        `json:"failure,omitempty"`
}

// ============================================================================
// module.Orchestrate — autonomous run (v0)
// ============================================================================

// ModuleOrchestrateInput drives the canonical persona chain for a
// module: specifier → architect → coder → tester → reviewer →
// documenter. v0 is fire-and-forget — kicks off each spawn but
// does not block on completion; blocking with retry and a parallel
// run under a budget are features of the code-gocode module.
type ModuleOrchestrateInput struct {
	ID             string   `json:"id"                 jsonschema:"Module-node UUID; required"`
	Personas       []string `json:"personas,omitempty" jsonschema:"Override the default 6-persona chain (specifier,architect,coder,tester,reviewer,documenter)"`
	StopOnRejected bool     `json:"stop_on_rejected,omitempty" jsonschema:"When true, abort the chain on the first rejected spawn (default false — kick off all, report each)"`
}

// SpawnRecord summarises one persona-spawn invocation.
type SpawnRecord struct {
	Persona string `json:"persona"`
	SpawnID string `json:"spawn_id,omitempty"`
	TaskID  string `json:"task_id,omitempty"`
	Status  string `json:"status"          jsonschema:"running | queued | rejected | error"`
	Reason  string `json:"reason,omitempty" jsonschema:"When status is rejected/error, why"`
}

// ModuleOrchestrateOutput summarises every spawn attempted.
type ModuleOrchestrateOutput struct {
	ModuleID string        `json:"module_id"`
	Spawns   []SpawnRecord `json:"spawns"`
	// AbortedAt is the index in Spawns at which the chain stopped
	// (when StopOnRejected=true). -1 if the full chain ran.
	AbortedAt int    `json:"aborted_at"`
	Notes     string `json:"notes,omitempty" jsonschema:"Operator-facing summary (e.g. 'all 6 personas queued')"`
}
