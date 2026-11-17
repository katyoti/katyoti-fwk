// SPDX-License-Identifier: Apache-2.0

// Package local provides the Extension declaration for the skills system.
// This is the single DI entry point — all real logic is in internal/.
//
// Skill + reference contribution is no longer per-type: sidecars wire
// `ContributeSkillResources` and `manifestcontrib.ContributeReferenceResources`
// instead — both feed `mbapi.ContributionProvidersPoint`, which SkillManager
// then filters by type at runtime.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
	skillcmd "github.com/katyoti/katyoti-fwk/internal/extensions/skills/commands"
	skillcore "github.com/katyoti/katyoti-fwk/internal/extensions/skills/internal/core"
)

// Option configures Extension. The only knob today is WithConfigFile.
type Option func(*options)

type options struct {
	advertiseConfigFile bool
}

// WithConfigFile makes the binary advertise + materialize
// `SkillManager.yaml` (the `extra_dirs` external-skill-directories knob).
//
// Only binaries that actually honor `extra_dirs` should pass this —
// today that's ktw-agents, the end-user orchestrator. Plain sidecars
// ship embedded skills only; they never read `extra_dirs`, so they must
// NOT advertise the file (otherwise `sc config check` reports a missing,
// default-less config for a knob the binary ignores). Without it the
// publication declines the automatic advertisement (publish.WithoutConfigFile).
//
// The opt-in binary MUST ship its own `embed/config/SkillManager.yaml`
// template — `DefaultConfigService` materializes the default from the
// BINARY's embed, the same path as every other config file.
func WithConfigFile() Option {
	return func(o *options) { o.advertiseConfigFile = true }
}

// Extension returns the skills system extension. Publishes SkillManager
// (runtime ops: GetReference, RunScript, GetSkill) and wires the `skills`
// Cobra command. Skills + references are contributed via the manifest's
// ContributionProvidersPoint (see `ContributeSkillResources` /
// `manifestcontrib.ContributeReferenceResources`).
func Extension(opts ...Option) application.Extension {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	return func(init *application.ApplicationInit) {
		// Phase E §4.3: SkillManager is a LOCAL service — consumed in-process
		// by the binary's own CLI (`<binary> sc skills …`) and, in ktw-agents,
		// by the agent loop's `skill_access` plugin. Skills themselves are
		// surfaced cross-binary as `Type: "skill"` Contributions (see
		// `ContributeSkillResources`); the runtime that EXECUTES a skill
		// is bound to the binary that owns it. There's no useful remote
		// surface for SkillManager — a remote caller can't run a script
		// inside another binary's process, and `contributions/get` already
		// returns the canonical skill body for read access.
		//
		// `SkillManager.yaml` (the `extra_dirs` knob) is advertised by
		// default, derived from the service name — see WithConfigFile's doc
		// for why a plain sidecar declines. The config LOADS either way,
		// through the standard by-name pipeline; the declaration only drives
		// the advertise/materialize/check surface (`sc config …`, `resources
		// list --type config-file`).
		pubOpts := []appcore.PublishOption{
			appcore.WithServiceDef(api.SkillManagerDef),
			appcore.WithLocal(),
		}
		if !o.advertiseConfigFile {
			// The file is derived and advertised by default; this binary does
			// not honour `extra_dirs`, so it declines.
			pubOpts = append(pubOpts, appcore.WithoutConfigFile())
		}
		init.AddExtension(appcore.PublishService[api.SkillManager, skillcore.SkillManagerConfig](
			skillcore.NewSkillManager,
			pubOpts...,
		))

		// Contribute the `skills` Cobra command under `sc skills` (per
		// the command-surface decision — three command families: domain
		// / sc / llm). Any binary that imports this extension AND
		// `appcore.SidecarGroupsExt()` automatically gains the
		// LLM-drivable CLI surface.
		init.AddExtension(appcore.SCCommandExt(skillcmd.NewSkillsCommand()))

		// Register SkillManager.Reload as a contribution RefreshHook so the
		// in-process skill/reference maps re-scan on a `contributions-changed`
		// event (e.g. a user drops a SKILL.md into `<home>/skills/`). This makes
		// SkillManager "one subscriber among others" — the event subscriber fans
		// out to every hook instead of hard-wiring Reload. Harmless in binaries
		// without the subscriber (only ktw-agents walks the point).
		manifestlocal.ContributeRefreshHook(mbapi.RefreshHook{
			Name:  "skill-manager",
			Kinds: []string{"skill", "reference"},
			Refresh: func(ctx context.Context, _ contribapi.ContributionsChangedEvent) error {
				mgr, err := application.UseService[api.SkillManager](ctx)
				if err != nil {
					return err
				}
				return mgr.Reload(ctx)
			},
		})(init)
	}
}
