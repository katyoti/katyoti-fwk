// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	personasapi "github.com/katyoti/katyoti-fwk/internal/extensions/personas/api"
)

// CanonicalPersonaChain is the default sequence ModuleOrchestrate
// spawns when Input.Personas is empty. Matches modules/code-gocode/concepts.md §8's
// workflow phase order (charter → specify → plan-tier → execute →
// verify → audit) mapped to the 6 baseline personas.
var CanonicalPersonaChain = []string{
	"specifier",  // souhait-side spec clarification
	"architect",  // module decomposition + decisions
	"coder",      // Go implementation
	"tester",     // L1 + L2 authoring + verify
	"reviewer",   // audit findings + lint triage
	"documenter", // charter prose + godoc sync
}

// ModuleOrchestrate is the v0 autonomous-run verb. Sequentially
// kicks off persona spawns via PersonaService.Spawn; does NOT
// block on completion. The caller observes progress through the
// returned task ids (or follow-up `module status` calls).
//
// v0 behavior:
//   - For each persona in the chain (CanonicalPersonaChain or
//     Input.Personas override), call personas.Spawn(name,
//     target=Input.ID).
//   - Record each spawn's outcome (status + reason) in the output.
//   - When Input.StopOnRejected=true and a spawn returns
//     status=rejected, abort the chain at that index. Default
//     (StopOnRejected=false) kicks off every persona regardless.
//
// v0 deliberately does NOT:
//   - Wait for spawned tasks to complete
//   - Verify the persona's default_gate advanced
//   - Retry on failure
//   - Auto-promote the module
//
// Those land in the v1 / v2 follow-ups tracked by the
// ktw-gocode-orchestrator task.
func (s *GocodeServiceImpl) ModuleOrchestrate(ctx stdctx.Context, in api.ModuleOrchestrateInput) (api.ModuleOrchestrateOutput, error) {
	if in.ID == "" {
		return api.ModuleOrchestrateOutput{}, fmt.Errorf("%w: ModuleOrchestrate: id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ModuleOrchestrateOutput{}, fmt.Errorf("ModuleOrchestrate: resolve CWMService: %w", err)
	}
	personas, err := application.UseService[personasapi.PersonaService](ctx)
	if err != nil {
		return api.ModuleOrchestrateOutput{}, fmt.Errorf("ModuleOrchestrate: resolve PersonaService: %w", err)
	}

	// Defensive: confirm the target node actually exists + is a
	// module before kicking off any spawn. Saves the user from
	// 6 sequential rejected-spawns when the id is wrong.
	module, err := cwmapi.ModuleOfObjectID(in.ID)
	if err != nil {
		return api.ModuleOrchestrateOutput{}, fmt.Errorf("ModuleOrchestrate: owning module: %w", err)
	}
	nodeOut, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return api.ModuleOrchestrateOutput{}, fmt.Errorf("ModuleOrchestrate: get module object %s: %w", in.ID, err)
	}
	if ok, err := projection.IsModule(ctx, cwm, in.ID); err != nil {
		return api.ModuleOrchestrateOutput{}, fmt.Errorf("ModuleOrchestrate: %w", err)
	} else if !ok {
		return api.ModuleOrchestrateOutput{}, fmt.Errorf("%w: ModuleOrchestrate: object %s (type %q) is not a module",
			api.ErrInvalidInput, in.ID, nodeOut.Object.Type)
	}

	chain := in.Personas
	if len(chain) == 0 {
		chain = CanonicalPersonaChain
	}

	out := api.ModuleOrchestrateOutput{
		ModuleID:  in.ID,
		Spawns:    make([]api.SpawnRecord, 0, len(chain)),
		AbortedAt: -1,
	}

	for i, persona := range chain {
		rec := api.SpawnRecord{Persona: persona}
		spawnOut, spawnErr := personas.Spawn(ctx, personasapi.SpawnInput{
			Name:   persona,
			Target: in.ID,
		})
		if spawnErr != nil {
			rec.Status = "error"
			rec.Reason = spawnErr.Error()
			out.Spawns = append(out.Spawns, rec)
			if in.StopOnRejected {
				out.AbortedAt = i
				out.Notes = fmt.Sprintf("aborted at persona[%d]=%q: %v", i, persona, spawnErr)
				return out, nil
			}
			continue
		}
		rec.SpawnID = spawnOut.SpawnID
		rec.TaskID = spawnOut.TaskID
		rec.Status = spawnOut.Status
		rec.Reason = spawnOut.Reason
		out.Spawns = append(out.Spawns, rec)

		if in.StopOnRejected && spawnOut.Status == "rejected" {
			out.AbortedAt = i
			out.Notes = fmt.Sprintf("aborted at persona[%d]=%q: %s", i, persona, spawnOut.Reason)
			return out, nil
		}
	}

	out.Notes = fmt.Sprintf("kicked off %d persona spawn(s) on module %s — observe progress via the task ids",
		len(out.Spawns), moduleName(nodeOut.Object))
	return out, nil
}

// moduleName renders an object's human label, falling back to its id so a
// message never reads "module " with a blank where the name should be.
func moduleName(o cwmapi.Object) string {
	if o.Name != "" {
		return o.Name
	}
	return o.ID
}
