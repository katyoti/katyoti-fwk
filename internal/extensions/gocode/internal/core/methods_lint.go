// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/lint"
)

// ModuleLint is the verb behind `ktw-gocode module-lint`. Runs the
// v1 detector set (see internal/lint/lint.go) over the module's
// items + tells the caller how many violations landed at each
// severity. `severity: error` counts feed promote-gate #4 — see
// methods_promote.go for the wire-up.
//
// Detector subset: caller may pass Input.Detectors to limit the run
// to a named subset; empty = all v1 detectors.
//
// Drift detectors (CharterProjectionDrift / ManifestDrift) compare
// the on-disk projection files against a fresh regeneration. The
// drift booleans are computed here (filesystem access lives in the
// wiring layer) and fed into the pure detectors as inputs.
func (s *GocodeServiceImpl) ModuleLint(ctx stdctx.Context, in api.ModuleLintInput) (api.ModuleLintOutput, error) {
	if in.ID == "" {
		return api.ModuleLintOutput{}, fmt.Errorf("%w: ModuleLint: id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ModuleLintOutput{}, fmt.Errorf("ModuleLint: resolve CWMService: %w", err)
	}

	inputs, err := projection.LoadManifestInputs(ctx, cwm, in.ID)
	if err != nil {
		return api.ModuleLintOutput{}, fmt.Errorf("ModuleLint: load inputs: %w", err)
	}

	// Test-cases aren't part of the projection inputs but the lint
	// detectors don't currently need them (the v1 set is item-
	// content-driven). When edge-walking detectors land we'll pull
	// them via loadTestCases like ModuleVerify does.
	//
	// CharterDrift / ManifestDrift are passed false in v1 — actually
	// computing them requires re-running the projection builders +
	// reading the on-disk files. That's a small follow-up; the
	// detector framework is already wired so they'll just light up
	// when the wiring layer feeds true.

	violations, errCount, warnCount, infoCount := lint.Run(lint.Inputs{
		Module:        inputs.Module,
		UseCases:      inputs.UseCases,
		OpenDecisions: inputs.OpenDecisions,
		CharterDrift:  false, // pending: wire up dry-run charter diff
		ManifestDrift: false, // pending: wire up dry-run manifest diff
	}, in.Detectors)

	return api.ModuleLintOutput{
		Violations: violations,
		ErrorCount: errCount,
		WarnCount:  warnCount,
		InfoCount:  infoCount,
	}, nil
}
