// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
)

// UniverseSetup holds the guide's "Do it for me" actions on a universe: what
// a recipe step may do for the person, after they confirm
// (modules/guide/concepts.md). They change things, so they are apart from the
// read-only UniverseChecks. Each takes the universe on screen and the step's
// `with` values.
type UniverseSetup interface {
	// EnsureAgents adds ktw-agents — the inbox, the tasks, the chat agent — to
	// the universe when it is not a member yet.
	EnsureAgents(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error)
	// CreateSampleProject writes the sample project into `with.folder` (default
	// <home>/katyoti-sample) and attaches it to the universe.
	CreateSampleProject(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error)
}

// UniverseSetupServiceDef declares the setup actions for KTSP publication.
var UniverseSetupServiceDef = meta.ServiceDef{
	Name:    "universe-setup",
	Short:   "Actions a guide recipe does for the person on their universe: add ktw-agents, create and attach a sample project",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"EnsureAgents":        {Short: "Add ktw-agents to the universe on screen when it is not a member"},
		"CreateSampleProject": {Short: "Write the sample project into with.folder (default <home>/katyoti-sample) and attach it to the universe on screen"},
	},
}
