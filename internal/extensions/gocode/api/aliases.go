// SPDX-License-Identifier: Apache-2.0

package api

import codeapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"

// The language-neutral domain model lives in the contract layer
// (code/api — Track B #3, 2026-06-12): the methodology owns the
// module / use-case / test-case / test-run / decision / audit /
// feature shapes; this pack imports them. Everything below is a type
// IDENTITY (Go type alias) or const/var re-export, not a parallel
// shape — one type, re-exported for the pack's callers. The Go-pack
// surface that stays HERE: GocodeService + GoToolService verb I/O and
// discovery shapes.

// ── Facets ───────────────────────────────────────────────────────────

type (
	Facet     = codeapi.Facet
	FacetKind = codeapi.FacetKind
)

const (
	FacetCode     = codeapi.FacetCode
	FacetDocs     = codeapi.FacetDocs
	FacetUCs      = codeapi.FacetUCs
	FacetTests    = codeapi.FacetTests
	FacetApp      = codeapi.FacetApp
	FacetCharter  = codeapi.FacetCharter
	FacetManifest = codeapi.FacetManifest

	// CodePackagesField names the code capability's declared list of code
	// packages — the one source of a module's code facets.
	CodePackagesField = codeapi.CodePackagesField
)

// ValidFacetKinds re-exports the contract's closed kind set.
var ValidFacetKinds = codeapi.ValidFacetKinds

// ── Module ───────────────────────────────────────────────────────────

type (
	Module        = codeapi.Module
	ModuleSummary = codeapi.ModuleSummary
	ModuleKind    = codeapi.ModuleKind
	TierState     = codeapi.TierState
	ModuleHealth  = codeapi.ModuleHealth
)

const (
	ModuleKindFramework = codeapi.ModuleKindFramework
	ModuleKindLibrary   = codeapi.ModuleKindLibrary
	ModuleKindSidecar   = codeapi.ModuleKindSidecar

	TierStateBootstrapping = codeapi.TierStateBootstrapping
	TierStateSpecifying    = codeapi.TierStateSpecifying
	TierStatePlanning      = codeapi.TierStatePlanning
	TierStateExecuting     = codeapi.TierStateExecuting
	TierStateVerifying     = codeapi.TierStateVerifying
	TierStateAuditing      = codeapi.TierStateAuditing
	TierStateGated         = codeapi.TierStateGated
	TierStatePromoted      = codeapi.TierStatePromoted

	ModuleHealthHealthy  = codeapi.ModuleHealthHealthy
	ModuleHealthDegraded = codeapi.ModuleHealthDegraded
	ModuleHealthBroken   = codeapi.ModuleHealthBroken
)

// ── Use cases ────────────────────────────────────────────────────────

type (
	UseCase    = codeapi.UseCase
	UCSeverity = codeapi.UCSeverity
	UCStatus   = codeapi.UCStatus
	UCLayer    = codeapi.UCLayer
)

const (
	UCSeverityCore     = codeapi.UCSeverityCore
	UCSeveritySecurity = codeapi.UCSeveritySecurity
	UCSeverityQoL      = codeapi.UCSeverityQoL
	UCSeverityCosmetic = codeapi.UCSeverityCosmetic

	UCStatusPass         = codeapi.UCStatusPass
	UCStatusFail         = codeapi.UCStatusFail
	UCStatusNoTest       = codeapi.UCStatusNoTest
	UCStatusFailDeferred = codeapi.UCStatusFailDeferred
	UCStatusSkipped      = codeapi.UCStatusSkipped
	UCStatusInDev        = codeapi.UCStatusInDev
	UCStatusCovered      = codeapi.UCStatusCovered
	UCStatusValidated    = codeapi.UCStatusValidated
	UCStatusRegressed    = codeapi.UCStatusRegressed

	UCLayerL1 = codeapi.UCLayerL1
	UCLayerL2 = codeapi.UCLayerL2
	UCLayerL3 = codeapi.UCLayerL3
)

// ── Test cases + test runs ───────────────────────────────────────────

type (
	TestCase       = codeapi.TestCase
	TestRun        = codeapi.TestRun
	TestRunStatus  = codeapi.TestRunStatus
	TestRunTotals  = codeapi.TestRunTotals
	TestRunVerdict = codeapi.TestRunVerdict
)

const (
	TestRunPass         = codeapi.TestRunPass
	TestRunFail         = codeapi.TestRunFail
	TestRunNotRun       = codeapi.TestRunNotRun
	TestRunHarnessError = codeapi.TestRunHarnessError
)

// ── Open decisions ───────────────────────────────────────────────────

type (
	OpenDecision = codeapi.OpenDecision
	ODState      = codeapi.ODState
)

const (
	ODStateOpen     = codeapi.ODStateOpen
	ODStateResolved = codeapi.ODStateResolved
)

// ── Audit ────────────────────────────────────────────────────────────

type (
	AuditBundle      = codeapi.AuditBundle
	AuditFinding     = codeapi.AuditFinding
	AuditPerspective = codeapi.AuditPerspective
	FindingSeverity  = codeapi.FindingSeverity
	FindingState     = codeapi.FindingState
)

const (
	AuditPerspectiveCorrectness   = codeapi.AuditPerspectiveCorrectness
	AuditPerspectiveQuality       = codeapi.AuditPerspectiveQuality
	AuditPerspectivePackageLayout = codeapi.AuditPerspectivePackageLayout
	AuditPerspectiveDeadCode      = codeapi.AuditPerspectiveDeadCode
	AuditPerspectiveLint          = codeapi.AuditPerspectiveLint

	FindingSeverityCritical = codeapi.FindingSeverityCritical
	FindingSeverityHigh     = codeapi.FindingSeverityHigh
	FindingSeverityMedium   = codeapi.FindingSeverityMedium
	FindingSeverityLow      = codeapi.FindingSeverityLow
	FindingSeverityInfo     = codeapi.FindingSeverityInfo

	FindingStateOpen     = codeapi.FindingStateOpen
	FindingStateClosed   = codeapi.FindingStateClosed
	FindingStateDeferred = codeapi.FindingStateDeferred

	AuditReviewType  = codeapi.AuditReviewType
	AuditFindingType = codeapi.AuditFindingType
	AuditReviewer    = codeapi.AuditReviewer
)

// ── Feature realization ──────────────────────────────────────────────

type (
	FeatureSummary    = codeapi.FeatureSummary
	RealizationStatus = codeapi.RealizationStatus
	WantedUCEdge      = codeapi.WantedUCEdge
)

const (
	RealizationNotStarted = codeapi.RealizationNotStarted
	RealizationInProgress = codeapi.RealizationInProgress
	RealizationVerifying  = codeapi.RealizationVerifying
	RealizationRegressed  = codeapi.RealizationRegressed
	RealizationDone       = codeapi.RealizationDone
)

// ── Lint ─────────────────────────────────────────────────────────────

// LintViolation is the contract's shape (the gcapi decl was an exact
// mirror; collapsed to the identity).
type LintViolation = codeapi.LintViolation

// The relation deferring a use case (code/api).
const EdgeDeferredBy = codeapi.EdgeDeferredBy

// DeferredByLocal reads a deferred-by target's local id (code/api).
var DeferredByLocal = codeapi.DeferredByLocal
