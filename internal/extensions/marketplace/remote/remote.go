// SPDX-License-Identifier: Apache-2.0

// Package remote is a KTSP-backed implementation of mktapi.SetupService: each
// method dispatches to the marketplace service served in the admin universe
// (`ktw-univers:marketplace/<method>`) over KTSP, rather than running the engine
// in-process. A consumer (ktw-gui, headless CLI) that has a `ktsapi.Sender` scoped
// to the admin universe swaps its in-process engine for NewRemote(sender) and
// keeps calling the SAME SetupService interface — no call-site changes.
//
// This is the client half of the pure-KTSP-client / hub-is-sole-writer model
// ((modules/univers/rules/R-001-the-hub-owns-management-state-clients-are-pure.cwm.md), (modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md)): the hub
// performs every write; this client only sends requests.
//
// NOTE: the wire method names are kebab-case (ToKebab of the Go method name) —
// the KTSP slot matches them EXACTLY (Slot.Handle), so they must be verbatim.
package remote

import (
	"context"
	"encoding/json"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// slotBase is the provider:slot prefix for the marketplace service as served by
// the admin universe's management sidecar (ktw-univers). A trailing `/<method>`
// completes the address.
const slotBase = "ktw-univers:marketplace/"

// NewRemote returns a SetupService that dispatches over KTSP via s. s should be a
// sender scoped to the admin universe (e.g. ktw-gui's universeSender("admin")).
func NewRemote(s ktsapi.Sender) mktapi.SetupService {
	return &remoteSetup{s: s}
}

type remoteSetup struct{ s ktsapi.Sender }

// compile-time proof the KTSP client implements the full contract.
var _ mktapi.SetupService = (*remoteSetup)(nil)

// call is the single dispatch chokepoint: send `in` to slotBase+method, decode
// the JSON response into Out. A nil/`null`/empty body yields the zero Out.
func call[Out any](ctx context.Context, s ktsapi.Sender, method string, in any) (Out, error) {
	var out Out
	raw, err := s.Call(ctx, slotBase+method, in)
	if err != nil {
		return out, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	return out, nil
}

// ── subscription ──
func (r *remoteSetup) ListMarketplaces(ctx context.Context) ([]mktapi.Marketplace, error) {
	return call[[]mktapi.Marketplace](ctx, r.s, "list-marketplaces", nil)
}
func (r *remoteSetup) AddMarketplace(ctx context.Context, in mktapi.AddMarketplaceInput) (mktapi.AddMarketplaceOutput, error) {
	return call[mktapi.AddMarketplaceOutput](ctx, r.s, "add-marketplace", in)
}
func (r *remoteSetup) RemoveMarketplace(ctx context.Context, in mktapi.RemoveMarketplaceInput) (mktapi.RemoveMarketplaceOutput, error) {
	return call[mktapi.RemoveMarketplaceOutput](ctx, r.s, "remove-marketplace", in)
}
func (r *remoteSetup) SetMarketplaceEnabled(ctx context.Context, in mktapi.SetMarketplaceEnabledInput) (mktapi.SetMarketplaceEnabledOutput, error) {
	return call[mktapi.SetMarketplaceEnabledOutput](ctx, r.s, "set-marketplace-enabled", in)
}
func (r *remoteSetup) SetMarketplaceChannel(ctx context.Context, in mktapi.SetMarketplaceChannelInput) (mktapi.SetMarketplaceChannelOutput, error) {
	return call[mktapi.SetMarketplaceChannelOutput](ctx, r.s, "set-marketplace-channel", in)
}
func (r *remoteSetup) RefreshCatalog(ctx context.Context, in mktapi.RefreshCatalogInput) (mktapi.Catalog, error) {
	return call[mktapi.Catalog](ctx, r.s, "refresh-catalog", in)
}

// ── catalog read ──
func (r *remoteSetup) SearchItems(ctx context.Context, in mktapi.SearchItemsInput) ([]mktapi.ItemSummary, error) {
	return call[[]mktapi.ItemSummary](ctx, r.s, "search-items", in)
}
func (r *remoteSetup) GetItem(ctx context.Context, in mktapi.GetItemInput) (mktapi.Item, error) {
	return call[mktapi.Item](ctx, r.s, "get-item", in)
}
func (r *remoteSetup) ItemVersions(ctx context.Context, in mktapi.ItemVersionsInput) (mktapi.ItemVersionsOutput, error) {
	return call[mktapi.ItemVersionsOutput](ctx, r.s, "item-versions", in)
}

// ── install / update ──
func (r *remoteSetup) Install(ctx context.Context, ref mktapi.ItemRef) (mktapi.Installed, error) {
	return call[mktapi.Installed](ctx, r.s, "install", ref)
}
func (r *remoteSetup) Uninstall(ctx context.Context, in mktapi.UninstallInput) (mktapi.UninstallOutput, error) {
	return call[mktapi.UninstallOutput](ctx, r.s, "uninstall", in)
}
func (r *remoteSetup) ListInstalled(ctx context.Context) ([]mktapi.Installed, error) {
	return call[[]mktapi.Installed](ctx, r.s, "list-installed", nil)
}
func (r *remoteSetup) Outdated(ctx context.Context) ([]mktapi.Update, error) {
	return call[[]mktapi.Update](ctx, r.s, "outdated", nil)
}
func (r *remoteSetup) Update(ctx context.Context, in mktapi.UpdateInput) ([]mktapi.Update, error) {
	return call[[]mktapi.Update](ctx, r.s, "update", in)
}
func (r *remoteSetup) SetupMachine(ctx context.Context) (mktapi.SetupMachineOutput, error) {
	return call[mktapi.SetupMachineOutput](ctx, r.s, "setup-machine", nil)
}

// The infra MUTATION verbs (InstallInfra / SelfUpdate / SetInfraVersion) have
// no proxy here on purpose: they are not on SetupService, because a hub cannot
// meaningfully replace its own binary over a wire call. Reads below are fine.
func (r *remoteSetup) InfraVersions(ctx context.Context) (mktapi.InfraVersionsOutput, error) {
	return call[mktapi.InfraVersionsOutput](ctx, r.s, "infra-versions", nil)
}
func (r *remoteSetup) InfraOutdated(ctx context.Context) ([]mktapi.Update, error) {
	return call[[]mktapi.Update](ctx, r.s, "infra-outdated", nil)
}
func (r *remoteSetup) ListConfigSets(ctx context.Context) ([]mktapi.ItemSummary, error) {
	return call[[]mktapi.ItemSummary](ctx, r.s, "list-config-sets", nil)
}
func (r *remoteSetup) ShowConfigSet(ctx context.Context, in mktapi.ShowConfigSetInput) (mktapi.ShowConfigSetOutput, error) {
	return call[mktapi.ShowConfigSetOutput](ctx, r.s, "show-config-set", in)
}
func (r *remoteSetup) ApplyConfigSet(ctx context.Context, in mktapi.ApplyConfigSetInput) (mktapi.ApplyConfigSetOutput, error) {
	return call[mktapi.ApplyConfigSetOutput](ctx, r.s, "apply-config-set", in)
}
func (r *remoteSetup) ListSeeds(ctx context.Context) ([]mktapi.ItemSummary, error) {
	return call[[]mktapi.ItemSummary](ctx, r.s, "list-seeds", nil)
}
func (r *remoteSetup) ShowSeed(ctx context.Context, in mktapi.ShowSeedInput) (mktapi.ShowSeedOutput, error) {
	return call[mktapi.ShowSeedOutput](ctx, r.s, "show-seed", in)
}
func (r *remoteSetup) ApplySeed(ctx context.Context, in mktapi.ApplySeedInput) (mktapi.ApplySeedOutput, error) {
	return call[mktapi.ApplySeedOutput](ctx, r.s, "apply-seed", in)
}
func (r *remoteSetup) ExportKeys(ctx context.Context, in mktapi.ExportKeysInput) (mktapi.ExportKeysOutput, error) {
	return call[mktapi.ExportKeysOutput](ctx, r.s, "export-keys", in)
}
func (r *remoteSetup) ImportVault(ctx context.Context, in mktapi.ImportVaultInput) (mktapi.ImportKeysOutput, error) {
	return call[mktapi.ImportKeysOutput](ctx, r.s, "import-vault", in)
}
func (r *remoteSetup) ImportKeys(ctx context.Context, in mktapi.ImportKeysInput) (mktapi.ImportKeysOutput, error) {
	return call[mktapi.ImportKeysOutput](ctx, r.s, "import-keys", in)
}
func (r *remoteSetup) SetVersion(ctx context.Context, in mktapi.SetVersionInput) (mktapi.Installed, error) {
	return call[mktapi.Installed](ctx, r.s, "set-version", in)
}

// ── configure ──
func (r *remoteSetup) ConfigureItem(ctx context.Context, in mktapi.ConfigureItemInput) (sidecarapi.ConfigCheckReport, error) {
	return call[sidecarapi.ConfigCheckReport](ctx, r.s, "configure-item", in)
}
func (r *remoteSetup) Doctor(ctx context.Context, in mktapi.DoctorInput) ([]mktapi.ItemCheck, error) {
	return call[[]mktapi.ItemCheck](ctx, r.s, "doctor", in)
}
func (r *remoteSetup) GetItemConfig(ctx context.Context, in mktapi.GetItemConfigInput) (mktapi.GetItemConfigOutput, error) {
	return call[mktapi.GetItemConfigOutput](ctx, r.s, "get-item-config", in)
}
func (r *remoteSetup) SetItemConfig(ctx context.Context, in mktapi.SetItemConfigInput) (mktapi.SetItemConfigOutput, error) {
	return call[mktapi.SetItemConfigOutput](ctx, r.s, "set-item-config", in)
}

// ── enable orchestration (AVAILABLE↔USED) ──
func (r *remoteSetup) WireIntoUniverse(ctx context.Context, in mktapi.WireIntoUniverseInput) (mktapi.WireIntoUniverseOutput, error) {
	return call[mktapi.WireIntoUniverseOutput](ctx, r.s, "wire-into-universe", in)
}
func (r *remoteSetup) Unwire(ctx context.Context, in mktapi.UnwireInput) (mktapi.UnwireOutput, error) {
	return call[mktapi.UnwireOutput](ctx, r.s, "unwire", in)
}
func (r *remoteSetup) PlanPrereqs(ctx context.Context, in mktapi.PrereqsPlanInput) (mktapi.PrereqsPlan, error) {
	return call[mktapi.PrereqsPlan](ctx, r.s, "plan-prereqs", in)
}
func (r *remoteSetup) InstallPrereqs(ctx context.Context, in mktapi.PrereqsInstallInput) (mktapi.PrereqsInstallOutput, error) {
	return call[mktapi.PrereqsInstallOutput](ctx, r.s, "install-prereqs", in)
}
func (r *remoteSetup) KeySets(ctx context.Context, in mktapi.KeySetsInput) (mktapi.KeySetsOutput, error) {
	return call[mktapi.KeySetsOutput](ctx, r.s, "key-sets", in)
}
func (r *remoteSetup) ImportKeySets(ctx context.Context, in mktapi.ImportKeySetsInput) (mktapi.ImportKeySetsOutput, error) {
	return call[mktapi.ImportKeySetsOutput](ctx, r.s, "import-key-sets", in)
}
func (r *remoteSetup) ProjectsFolder(ctx context.Context) (mktapi.ProjectsFolderOutput, error) {
	return call[mktapi.ProjectsFolderOutput](ctx, r.s, "projects-folder", nil)
}
func (r *remoteSetup) SetProjectsFolder(ctx context.Context, in mktapi.SetProjectsFolderInput) (mktapi.ProjectsFolderOutput, error) {
	return call[mktapi.ProjectsFolderOutput](ctx, r.s, "set-projects-folder", in)
}
func (r *remoteSetup) EarlierProjects(ctx context.Context) (mktapi.EarlierProjectsOutput, error) {
	return call[mktapi.EarlierProjectsOutput](ctx, r.s, "earlier-projects", nil)
}
func (r *remoteSetup) EarlierInstallations(ctx context.Context) (mktapi.EarlierInstallationsOutput, error) {
	return call[mktapi.EarlierInstallationsOutput](ctx, r.s, "earlier-installations", nil)
}
func (r *remoteSetup) DeleteInstallation(ctx context.Context, in mktapi.DeleteInstallationInput) (mktapi.DeleteInstallationOutput, error) {
	return call[mktapi.DeleteInstallationOutput](ctx, r.s, "delete-installation", in)
}
func (r *remoteSetup) ImportProject(ctx context.Context, in mktapi.ImportProjectInput) (mktapi.ImportProjectOutput, error) {
	return call[mktapi.ImportProjectOutput](ctx, r.s, "import-project", in)
}
func (r *remoteSetup) InstallSolution(ctx context.Context, in mktapi.InstallSolutionInput) (mktapi.InstallSolutionOutput, error) {
	return call[mktapi.InstallSolutionOutput](ctx, r.s, "install-solution", in)
}
func (r *remoteSetup) RepinSolution(ctx context.Context, in mktapi.RepinSolutionInput) (mktapi.RepinSolutionOutput, error) {
	return call[mktapi.RepinSolutionOutput](ctx, r.s, "repin-solution", in)
}
