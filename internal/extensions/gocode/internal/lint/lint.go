// SPDX-License-Identifier: Apache-2.0

// Package lint composes the Go pack's lint surface: the
// language-neutral detector base from code/lint plus the Go-specific
// detectors that only make sense for this pack's module layout.
//
// Go-specific detector set:
//
//   - SidecarMissingAppFolder — error, sidecar module without an `app` facet
//
// Deferred (per modules/code-gocode/concepts.md §2): MissingThreePackageLayout, godoc
// coverage rules.
package lint

import (
	codelint "github.com/katyoti/katyoti-fwk/internal/extensions/code/lint"
	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// The wiring layer (methods_lint.go) and goPack.Describe consume
// these shapes; they are the contract package's — identities, not
// copies.
type (
	Inputs   = codelint.Inputs
	Detector = codelint.Detector
)

const (
	SeverityError = codelint.SeverityError
	SeverityWarn  = codelint.SeverityWarn
	SeverityInfo  = codelint.SeverityInfo
)

// AllDetectors returns the composed registry: the neutral base
// followed by the Go-specific detectors.
func AllDetectors() []Detector {
	return append(codelint.BaseDetectors(), goDetectors()...)
}

func goDetectors() []Detector {
	return []Detector{
		{Name: "SidecarMissingAppFolder", Severity: SeverityError, Check: sidecarMissingAppFolder},
	}
}

// Run executes the requested detectors from the composed registry.
// Empty `requested` means "run all".
func Run(in Inputs, requested []string) ([]gcapi.LintViolation, int, int, int) {
	return codelint.RunDetectors(AllDetectors(), in, requested)
}

// sidecarMissingAppFolder flags sidecar-kind modules without an
// `app` facet. Sidecars produce an executable; without the folder
// pointer downstream verbs (charter projection, audit) can't locate
// the build target. `error` severity.
func sidecarMissingAppFolder(in Inputs) []gcapi.LintViolation {
	if in.Module.Kind != gcapi.ModuleKindSidecar {
		return nil
	}
	if in.Module.FirstFacet(gcapi.FacetApp) != "" {
		return nil
	}
	return []gcapi.LintViolation{{
		NodeID:        in.Module.ID,
		Reason:        "module kind=sidecar but no `app` facet is declared — downstream verbs can't locate the build target",
		FixSuggestion: "Add `{ kind: app, path: \"app/ktw-foo/\" }` to the module's facets and re-run lint",
	}}
}
