// SPDX-License-Identifier: Apache-2.0

// Package available reads the machine-wide available-items registry — the
// existing sidecars.yaml (extended with marketplace-installed + folder-hosted
// content entries) reconciled with a live PATH scan. It reuses the
// orchestrator/catalog registry (ADR: reconcile, don't fork).
package available

import (
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	inv "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// List returns everything available on this machine: the recorded sidecars.yaml
// entries (marketplace-installed + folder-hosted content) FIRST, then a live
// PATH scan augments with binaries not already recorded (dev/PATH discovery).
// Recorded entries win — a live scan never overwrites a marketplace install.
func List() ([]mktapi.Installed, error) {
	path, err := inv.DefaultSidecarsPath()
	if err != nil {
		return nil, err
	}
	existing, err := inv.LoadSidecarsYaml(path)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []mktapi.Installed
	for _, e := range existing.Sidecars {
		seen[e.Name] = true
		out = append(out, toInstalled(e))
	}
	for _, e := range inv.ScanLocal(inv.DefaultScanRoots()) {
		if seen[e.Name] {
			continue // recorded entry wins over a live-scanned dev/PATH binary
		}
		seen[e.Name] = true
		out = append(out, toInstalled(e))
	}
	return out, nil
}

func toInstalled(e inv.SidecarsYamlEntry) mktapi.Installed {
	return mktapi.Installed{
		Name:        e.Name,
		Kind:        e.Kind,
		Version:     e.Version,
		Source:      e.Source,
		Marketplace: e.Marketplace,
		Sha256:      e.Sha256,
		BinaryPath:  e.BinaryPath,
		FolderPath:  e.FolderPath,
	}
}

// Record upserts a katyoti-installed item into sidecars.yaml (one row per
// name — the store dir holds all versions; a fully version-aware index is the
// ADR §14 refinement).
//
// The caller's Source is PRESERVED. It used to be overwritten with the literal
// "marketplace", which threw away the one fact that distinguishes a control-plane
// binary from a catalog sidecar: install-infra records its four with
// Source=SourceInfraChannel ("infra-channel"), and this silently relabelled them.
//
// Everything downstream filters on that field, so the loss surfaced as a string
// of unrelated-looking bugs: `katyoti doctor` exec'd `ktw-gui sidecar config
// check` and OPENED A WINDOW; the GUI Marketplace tab listed katyoti and ktw-gui
// as sidecars with an uninstall button; `katyoti installed` called them
// marketplace sidecars while printing the infra CHANNEL as their marketplace.
//
// Empty defaults to "marketplace" so existing callers that do not set it keep
// their behaviour.
func Record(in mktapi.Installed) error {
	path, err := inv.DefaultSidecarsPath()
	if err != nil {
		return err
	}
	// Route through the shared, inventoryMu-locked writer so the marketplace-install
	// path and the inventory-scan path serialize on sidecars.yaml (one authority).
	return inv.UpsertSidecarEntry(path, entryFor(in))
}

// entryFor maps an Installed to the on-disk row. Split out from Record so the
// mapping is testable without writing to the ambient katyoti root — the Source
// handling below is load-bearing and was silently wrong.
func entryFor(in mktapi.Installed) inv.SidecarsYamlEntry {
	source := in.Source
	if source == "" {
		source = "marketplace"
	}
	return inv.SidecarsYamlEntry{
		Name:        in.Name,
		BinaryPath:  in.BinaryPath,
		Version:     in.Version,
		Source:      source,
		Kind:        in.Kind,
		FolderPath:  in.FolderPath,
		Marketplace: in.Marketplace,
		Sha256:      in.Sha256,
	}
}

// RemoveInstalled drops a marketplace-installed item from sidecars.yaml (only the
// marketplace-sourced row, never a PATH-scan row of the same name).
func RemoveInstalled(name string) error {
	path, err := inv.DefaultSidecarsPath()
	if err != nil {
		return err
	}
	_, err = inv.RemoveSidecarEntry(path, name, "marketplace")
	return err
}
