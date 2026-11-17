// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
)

// UniverseChecks answers the guide's checks about universes: read-only
// questions a recipe asks at its last step to prove its result is there
// (modules/guide/concepts.md, decision OD-009). Each takes the universe on
// screen and the step's `with` values, and answers {ok, detail}.
//
// It is its own slot (`universe-checks`) beside the orchestrator rather than
// methods on OrchestratorService: the checks read the orchestrator, they are
// not part of its contract with its other clients.
type UniverseChecks interface {
	// UniverseReady: the universe on screen is registered and is a user
	// universe (not the protected admin universe).
	UniverseReady(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error)
	// ProjectAttached: the universe has at least one attached project.
	ProjectAttached(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error)
	// CapabilityWired: the sidecar named by `with.sidecar` is a member of the
	// universe; without it, any member other than ktw-agents.
	CapabilityWired(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error)
	// RepositoriesHaveRemote: every git repository holding the universe's
	// home or one of its attached projects has a remote. A repository with
	// none fails with a warning naming it: its work is not backed up off
	// this machine. A folder in no repository is not counted.
	RepositoriesHaveRemote(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error)
}

// UniverseChecksServiceDef declares the checks for KTSP publication. Every
// method is read-only: it may serve as a recipe's check and leaves no record.
var UniverseChecksServiceDef = meta.ServiceDef{
	Name:    "universe-checks",
	Short:   "Read-only checks a guide recipe runs to prove its result: universe ready, project attached, capability wired",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"UniverseReady":          {Short: "Is the universe on screen a registered user universe?", ReadOnly: true},
		"ProjectAttached":        {Short: "Does the universe on screen have an attached project?", ReadOnly: true},
		"CapabilityWired":        {Short: "Is the capability (with.sidecar, else any beyond ktw-agents) wired into the universe on screen?", ReadOnly: true},
		"RepositoriesHaveRemote": {Short: "Does every repository of the universe on screen (its home, its projects) have a remote? None: a warning that its work is not backed up", ReadOnly: true},
	},
}
