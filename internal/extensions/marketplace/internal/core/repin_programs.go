// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/wire"
	orchcatalog "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// RepinPrograms points every universe of this installation at its own
// programs: make install copies a checkout's bin/ into the
// control plane and bin/dev, then this re-pins each member still running a
// program that lies in no installation to the installation's copy, and scans
// the registry again so its discovered programs are the installation's. A
// later make build or go test then changes nothing the installation runs.
func (s *infraOps) RepinPrograms(_ context.Context, in mktapi.RepinProgramsInput) (mktapi.RepinProgramsOutput, error) {
	var out mktapi.RepinProgramsOutput
	inst := cenv.CurrentInstallation()
	if inst == nil {
		return out, fmt.Errorf("repin-programs: this katyoti belongs to no installation (run the installation's own katyoti, in its control-plane folder)")
	}
	out.Installation = inst.Name
	moved, missing, err := wire.RepinToPrograms(controlplane.ProgramPath, in.DryRun)
	out.Moved, out.Missing = moved, missing
	if err != nil || in.DryRun {
		return out, err
	}
	path, err := orchcatalog.DefaultSidecarsPath()
	if err != nil {
		return out, err
	}
	merged, _, err := orchcatalog.PersistInventory(path, orchcatalog.ScanLocal(orchcatalog.DefaultScanRoots()))
	if err != nil {
		return out, fmt.Errorf("repin-programs: the registry: %w", err)
	}
	for _, e := range merged.Sidecars {
		if orchcatalog.IsScanSource(e.Source) {
			out.Discovered = append(out.Discovered, e.Name)
		}
	}
	return out, nil
}
