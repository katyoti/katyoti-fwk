// SPDX-License-Identifier: Apache-2.0

// Package remote is a KTSP-backed implementation of api.OrchestratorService: each
// method dispatches to the universe-registry service served in the admin universe
// (`ktw-univers:orchestrator/<method>`) over KTSP, rather than reading/writing the
// registry files in-process. A consumer (ktw-gui, a headless CLI) with a
// `ktsapi.Sender` scoped to the admin universe swaps its in-process/vendored path for
// NewRemote(sender) and keeps calling the SAME OrchestratorService interface.
//
// The client half of the pure-KTSP-client / hub-is-sole-writer model
// ((modules/univers/rules/R-001-the-hub-owns-management-state-clients-are-pure.cwm.md), (modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md)). Wire method
// names are kebab-case (ToKebab of the Go method name) — Slot.Handle matches them
// EXACTLY, so they must be verbatim.
package remote

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// slotBase is the provider:slot prefix for the universe-registry service as served
// by the admin universe's management sidecar (ktw-univers).
const slotBase = "ktw-univers:orchestrator/"

// NewRemote returns an OrchestratorService that dispatches over KTSP via s. s should
// be a sender scoped to the admin universe (e.g. ktw-gui's universeSender("admin")).
func NewRemote(s ktsapi.Sender) api.OrchestratorService {
	return &remoteOrch{s: s}
}

type remoteOrch struct{ s ktsapi.Sender }

// compile-time proof the KTSP client implements the full contract.
var _ api.OrchestratorService = (*remoteOrch)(nil)

// call is the single dispatch chokepoint: send `in` to slotBase+method, decode the
// JSON response into Out. A nil/`null`/empty body yields the zero Out.
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

func (r *remoteOrch) ListUniverses(ctx context.Context, in api.ListUniversesInput) (api.ListUniversesOutput, error) {
	return call[api.ListUniversesOutput](ctx, r.s, "list-universes", in)
}
func (r *remoteOrch) GetUniverse(ctx context.Context, in api.GetUniverseInput) (api.UniverseInfo, error) {
	return call[api.UniverseInfo](ctx, r.s, "get-universe", in)
}
func (r *remoteOrch) CreateUniverse(ctx context.Context, in api.CreateUniverseInput) (api.UniverseInfo, error) {
	return call[api.UniverseInfo](ctx, r.s, "create-universe", in)
}
func (r *remoteOrch) DeleteUniverse(ctx context.Context, in api.DeleteUniverseInput) (api.DeleteUniverseOutput, error) {
	return call[api.DeleteUniverseOutput](ctx, r.s, "delete-universe", in)
}
func (r *remoteOrch) ScanInventory(ctx context.Context, in api.ScanInventoryInput) (api.ScanInventoryOutput, error) {
	return call[api.ScanInventoryOutput](ctx, r.s, "scan-inventory", in)
}
func (r *remoteOrch) AddSidecar(ctx context.Context, in api.AddSidecarInput) (api.UniverseInfo, error) {
	return call[api.UniverseInfo](ctx, r.s, "add-sidecar", in)
}
func (r *remoteOrch) RemoveSidecar(ctx context.Context, in api.RemoveSidecarInput) (api.UniverseInfo, error) {
	return call[api.UniverseInfo](ctx, r.s, "remove-sidecar", in)
}
func (r *remoteOrch) AttachProject(ctx context.Context, in api.AttachProjectInput) (api.AttachProjectOutput, error) {
	return call[api.AttachProjectOutput](ctx, r.s, "attach-project", in)
}
func (r *remoteOrch) DetachProject(ctx context.Context, in api.DetachProjectInput) (api.DetachProjectOutput, error) {
	return call[api.DetachProjectOutput](ctx, r.s, "detach-project", in)
}
func (r *remoteOrch) SwitchProject(ctx context.Context, in api.SwitchProjectInput) (api.SwitchProjectOutput, error) {
	return call[api.SwitchProjectOutput](ctx, r.s, "switch-project", in)
}

func (r *remoteOrch) CreateProject(ctx context.Context, in api.CreateProjectInput) (api.CreateProjectOutput, error) {
	return call[api.CreateProjectOutput](ctx, r.s, "create-project", in)
}
func (r *remoteOrch) ListProjects(ctx context.Context, in api.ListProjectsInput) (api.ListProjectsOutput, error) {
	return call[api.ListProjectsOutput](ctx, r.s, "list-projects", in)
}
func (r *remoteOrch) ResolveProject(ctx context.Context, in api.ResolveProjectInput) (api.ResolveProjectOutput, error) {
	return call[api.ResolveProjectOutput](ctx, r.s, "resolve-project", in)
}
func (r *remoteOrch) UpdateUniverse(ctx context.Context, in api.UpdateUniverseInput) (api.UniverseInfo, error) {
	return call[api.UniverseInfo](ctx, r.s, "update-universe", in)
}
func (r *remoteOrch) AddExistingUniverse(ctx context.Context, in api.AddExistingUniverseInput) (api.UniverseInfo, error) {
	return call[api.UniverseInfo](ctx, r.s, "add-existing-universe", in)
}
func (r *remoteOrch) SetUniverseSolution(ctx context.Context, in api.SetUniverseSolutionInput) (api.UniverseInfo, error) {
	return call[api.UniverseInfo](ctx, r.s, "set-universe-solution", in)
}
func (r *remoteOrch) RecordCreatedProject(ctx context.Context, in api.RecordCreatedProjectInput) (api.RecordCreatedProjectOutput, error) {
	return call[api.RecordCreatedProjectOutput](ctx, r.s, "record-created-project", in)
}
func (r *remoteOrch) ForgetCreatedProject(ctx context.Context, in api.ForgetCreatedProjectInput) (api.ForgetCreatedProjectOutput, error) {
	return call[api.ForgetCreatedProjectOutput](ctx, r.s, "forget-created-project", in)
}
