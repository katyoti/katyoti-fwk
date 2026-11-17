// SPDX-License-Identifier: Apache-2.0

package api

import "github.com/katyoti/katyoti-fwk/internal/core/meta"

// GocodeServiceDef is the meta.ServiceDef for GocodeService.
// AutoKTSPContainer + AutoHTTPServer consume it for CLI / OpenAPI /
// agent-tool descriptor generation.
//
// Method-key naming follows the <slot>.<Verb> convention so the
// CLI auto-generator groups them into sub-commands
// (gocode module verify, gocode uc create, etc.).
var GocodeServiceDef = meta.ServiceDef{
	Name:  "gocode",
	Short: "Go-coding method pack: charter / UCs / tests / audits / tier promotion",
	Long: `ktw-gocode is the L1 method pack on top of CWM that takes a Go module
from tier 0 to rock-solid: charter, use-case-driven testing (L1/L2/L3),
audit bundles, tier-promotion gates. Both greenfield (` + "`module create`" + `)
and retro-engineering (` + "`module discover`" + `) modes supported.`,
	Version: "0.1.0",
	// Keyed by the Go method name: the KTSP publisher looks a method's
	// definition up by that name (internal/kts/local/reflect_dispatch.go), so
	// its description, read-only flag and timeout reach the catalog. The CLI
	// groups the verbs by their kebab name (module-create, uc-create …).
	Methods: map[string]meta.MethodDef{
		// module verbs
		"ModuleCreate":      meta.Method("Scaffold a new module (greenfield)"),
		"ModuleDiscover":    meta.Method("Retro-engineer an existing Go path into a tracked module"),
		"ModuleStatus":      {Short: "Read-only state report — tier, use-case totals, audit findings, blocking gates", ReadOnly: true},
		"ModuleDashboard":   {Short: "The module's status as a page: health, tier, use-case totals, audit, promote gates and facets", ReadOnly: true},
		"ModulePlanTier":    meta.Method("Compute tier-N gaps; propose tasks"),
		"ModuleVerify":      meta.Method("Run L1+L2 (+L3 on demand); write a test-run item; update UC status_cache"),
		"ModuleAudit":       meta.Method("Run the automated code checks; write a review of the module with its findings"),
		"ModuleReviewCheck": {Short: "The go-checks review type's check: build, vet, tests, package layout, dead code and lint as a review result", ReadOnly: true, TimeoutS: 2400},
		"ModuleLint":        meta.Method("Check rule conformance for the module's kind; emit drift findings"),
		"ModulePromote":     meta.Method("Tier flip — refuses on unmet gates unless --force"),
		"ModuleOrchestrate": meta.Method("Autonomous run: spawn the 6 baseline personas in canonical order (v0: fire-and-forget)"),

		// use-case verbs
		"UCCreate":           meta.Method("Create a use-case item (UC-NNN) on a module"),
		"UCUpdate":           meta.Method("Patch mutable fields on a use-case (omit-to-leave semantics)"),
		"UCSetStatus":        meta.Method("Write status_cache (the only legit writer; called by verify)"),
		"UCProposeFromTests": meta.Method("Scan existing _test.go files; propose seed UCs"),

		// feature verbs
		"FeatureReverseEngineer":   meta.Method("Analyze cross-module APIs; propose seed user-stories + their wanted Use Cases"),
		"FeatureRealizationStatus": {Short: "A feature's derived status: each wanted use case's status and the worst of them", ReadOnly: true},

		// tasktest verbs
		"TasktestCreate": meta.Method("Scaffold a .tasktest L2 scenario from a UC"),
		"TasktestRun":    meta.Method("Execute a .tasktest scenario via simulator (token-free)"),
	},
}

// GoToolServiceDef is the meta.ServiceDef for the Go-toolchain wrapper
// surface. Methods auto-published under the `gotool.*` slot namespace.
var GoToolServiceDef = meta.ServiceDef{
	Name:  "gotool",
	Short: "Pure Go toolchain wrappers (go build / test / vet / lint / fmt / mod / …)",
	Long: `Thin wrappers around the Go toolchain with structured output. Used by
GocodeService orchestration verbs and exposable directly to agents that
need a single Go command.`,
	Version: "0.1.0",
	Methods: map[string]meta.MethodDef{
		"go.Build":        meta.Method("go build ./..."),
		"go.Test":         meta.Method("go test with optional -run filter; -json parsing supported"),
		"go.Vet":          meta.Method("go vet ./..."),
		"go.GolangciLint": meta.Method("golangci-lint run"),
		"go.APIDiff":      meta.Method("go-apidiff between two refs to detect breaking changes"),
		"go.PkgInventory": meta.Method("List packages in a module; classify public/internal/local/other"),
		"go.DepCheck":     meta.Method("Verify imports respect the public/internal layout boundaries"),
		"gofmt":           meta.Method("gofmt check or apply"),
		"goimports":       meta.Method("goimports check or apply"),
		"go.ModTidy":      meta.Method("go mod tidy (with optional --diff)"),
	},
}
