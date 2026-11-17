// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// LanguagePack KTSP method slot names. These are the EXACT slot names the bus
// uses — they equal `reflect.ToKebab(<GoMethodName>)`, so they agree with the
// auto-derived KTSP container regardless of whether it reads the ServiceDef
// Methods map or reflects the interface. The bus PROXY (BusLanguagePack) and the
// PUBLISH (gocode) both reference these constants, so server + client can never
// drift.
const (
	LPMethodDescribe                = "describe"
	LPMethodDiscover                = "discover"
	LPMethodProposeUCsFromTests     = "propose-u-cs-from-tests" // ToKebab("ProposeUCsFromTests")
	LPMethodReverseEngineerFeatures = "reverse-engineer-features"
	LPMethodRunUnitTest             = "run-unit-test"
	LPMethodGateCheck               = "gate-check"
)

// LanguagePackBusSlot is the KEBAB bus slot name for the published LanguagePack
// service. The KTSP/URI grammar is kebab-only, so the bus address is
// `<provider>:/language-pack/<method>` — NOT the PascalCase LanguagePackSlotType
// (that const stays the in-process DI / discovery type identifier). This mirrors
// CodeServiceDef.Name == "code-service": ServiceDef.Name IS the bus slot, used
// verbatim, so it must already be kebab.
const LanguagePackBusSlot = "language-pack"

// LanguagePackServiceDef is the KTSP service definition published by each
// `ktw-<lang>code` peer (bus slot `language-pack`; the language is the provider).
// Method keys are the kebab slot names above; descriptions are informational.
var LanguagePackServiceDef = meta.ServiceDef{
	Name:    LanguagePackBusSlot,
	Short:   "Language-specific code methodology pack (discover / verify / gate)",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		LPMethodDescribe:                meta.Method("Static self-description: identity, extensions, gates, check kinds, persona augmentations"),
		LPMethodDiscover:                meta.Method("Scan a code root and propose the seed shape (inventory + facet/UC/test-case proposals)"),
		LPMethodProposeUCsFromTests:     meta.Method("Read existing test files and propose seed use-cases (one per test cluster)"),
		LPMethodReverseEngineerFeatures: meta.Method("Analyze cross-module APIs and propose feature candidates (low-confidence)"),
		LPMethodRunUnitTest:             meta.Method("Execute ONE probe (an AC test_ref) and return its outcome (L1+L2 only)"),
		LPMethodGateCheck:               meta.Method("Evaluate one declared gate against a module's facets"),
	},
}

// LanguagePack is the contract every `ktw-<lang>code` peer implements
// (Track B B1; D-1 resolved 2026-06-12: KTSP slot via the standard
// PublishService pattern).
//
// Registration + discovery: each pack publishes its implementation
// with `publish.PublishService` under SlotType "LanguagePack" and the
// LANGUAGE as the instance name ("go", "typescript", …). ktw-code
// resolves `application.UseNamedService[LanguagePack](ctx, lang)` —
// in-process dispatch when the pack is bundled in the same binary
// (today's deployed reality: gocode inside ktw-agents), a KTSP slot
// send when the pack runs as its own sidecar. One contract, both
// topologies.
//
// Shape rules (the substance of D-1):
//   - every method is `ctx + serializable input → serializable
//     output` — no Go interfaces, no funcs, no `any` in the contract;
//   - the pack's build/test tool surface is NOT part of this
//     interface — packs publish their own tool service (gocode's
//     GoToolService already exists) and name its slot in
//     Describe().ToolServiceSlot;
//   - gate CHECKS are the GateCheck verb; gate DECLARATIONS are data
//     (Describe().Gates) — replaces the old `GateContributions()
//     []GateHandler` idea, which returned non-serializable handlers.
type LanguagePack interface {
	// Describe returns everything static about the pack in ONE
	// round-trip: identity, file extensions, tool-service slot, gate
	// declarations, check kinds, layout rules, persona augmentations.
	Describe(ctx context.Context, in DescribeInput) (LanguagePackInfo, error)

	// Discover scans a code root and proposes the seed shape:
	// inventory (auto-commit-safe), facet proposals, UC proposals
	// (judgment — specifier reviews), test-case proposals.
	Discover(ctx context.Context, in DiscoverInput) (DiscoverOutput, error)

	// ProposeUCsFromTests reads existing test files and proposes seed
	// UCs (one per test cluster) — the focused subset of Discover.
	ProposeUCsFromTests(ctx context.Context, in ProposeUCsInput) (ProposeUCsOutput, error)

	// ReverseEngineerFeatures analyzes cross-module APIs and proposes
	// feature candidates. Low-confidence by design; never auto-commit.
	ReverseEngineerFeatures(ctx context.Context, in ReverseEngineerInput) (ReverseEngineerOutput, error)

	// RunUnitTest executes ONE probe (an AC's test_ref) and returns
	// its outcome. L1+L2 only — verify never dispatches L3 probes
	// (L3 runs only on explicit user request via its own harness).
	RunUnitTest(ctx context.Context, in RunUnitTestInput) (RunUnitTestOutput, error)

	// GateCheck evaluates one declared gate (Describe().Gates) against
	// a module's facets. Wired into the module-version FSM transitions.
	GateCheck(ctx context.Context, in GateCheckInput) (GateCheckOutput, error)
}

// LanguagePackSlotType is the published SlotType; the instance name is
// the language ("go", "typescript", …). Mirrors the skills precedent:
// service type name for both local DI and remote discovery.
const LanguagePackSlotType = "LanguagePack"

// DescribeInput is empty today; reserved for future filtering.
type DescribeInput struct{}

// LanguagePackInfo is the pack's full static self-description.
type LanguagePackInfo struct {
	Language             string                `json:"language"                 jsonschema:"go | typescript | python | …"`
	FileExtensions       []string              `json:"file_extensions"          jsonschema:"e.g. [\".go\"]"`
	ToolServiceSlot      string                `json:"tool_service_slot,omitempty" jsonschema:"Slot name of the pack's own build/test tool service (e.g. gocode's GoToolService)"`
	Gates                []GateSpec            `json:"gates,omitempty"`
	CheckKinds           []CheckKindSpec       `json:"check_kinds,omitempty"`
	LayoutRules          []LayoutRuleSpec      `json:"layout_rules,omitempty"`
	PersonaAugmentations []PersonaAugmentation `json:"persona_augmentations,omitempty"`
}

type DiscoverInput struct {
	Path   string `json:"path"             jsonschema:"Code root to scan (repo-relative or absolute)"`
	DryRun bool   `json:"dry_run,omitempty"`
}

type DiscoverOutput struct {
	Inventory         Inventory          `json:"inventory"`
	FacetProposals    []FacetProposal    `json:"facet_proposals,omitempty"`
	UCProposals       []UCProposal       `json:"uc_proposals,omitempty"`
	TestCaseProposals []TestCaseProposal `json:"test_case_proposals,omitempty"`
	Warnings          []string           `json:"warnings,omitempty"`
}

type ProposeUCsInput struct {
	Path string `json:"path" jsonschema:"Code root whose tests to read"`
}

type ProposeUCsOutput struct {
	UCProposals []UCProposal `json:"uc_proposals,omitempty"`
}

type ReverseEngineerInput struct {
	Path string `json:"path" jsonschema:"Project root spanning the modules to analyze"`
}

type ReverseEngineerOutput struct {
	FeatureProposals []FeatureProposal `json:"feature_proposals,omitempty"`
}

type RunUnitTestInput struct {
	Ref TestRef `json:"ref"`
}

type RunUnitTestOutput struct {
	Result TestResult `json:"result"`
}
