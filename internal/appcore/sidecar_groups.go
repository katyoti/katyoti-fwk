// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	"github.com/spf13/cobra"
)

// Named-service keys used to locate the sc + llm cobra groups in the
// ServiceManager. Extensions that want to register subcommands under
// these groups call AppUseNamedService[CobraCommand] with the matching
// key, OR use the SCCommandExt / LLMCommandExt helpers.
const (
	GroupNameSidecar = "sidecar" // alias: "sc"
	GroupNameLLM     = "llm"
)

// SidecarGroupsExt registers the `sc | sidecar` and `llm` cobra groups
// on the root command. Per the command-surface decision (three command
// families: domain / sc / llm) — locked at this layer so every binary
// gets the same shape uniformly without per-bootstrap wire-up.
//
// Place AFTER NewAppExt in buildExtensions:
//
//	appcore.NewAppExt(cfg),
//	appcore.SidecarGroupsExt(),
//	appcore.SidecarExt(efs),
//	...
//
// The two groups are stored as named CobraCommand services in the SM
// under "sidecar" and "llm" so any extension that wants to register
// subcommands can resolve the right parent.
func SidecarGroupsExt() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddBootstrap("appcore", ".sidecar-groups", func(app *application.Application) error {
			cr, err := application.AppUseNamedService[CobraCommand](app, "root")
			if err != nil {
				return fmt.Errorf("sidecar-groups: root cobra unavailable: %w", err)
			}
			root := cr.Cmd

			// `sc` cobra group (alias of `sidecar`). Operator + dev
			// surface — config, skills, tasks, contributions, ktsp,
			// schema, dirs, server, etc.
			sidecarCmd := &cobra.Command{
				Use:     GroupNameSidecar,
				Aliases: []string{"sc"},
				Short:   "Sidecar admin (config, skills, tasks, resources, ...)",
				Long: `The sidecar group consolidates all operator-facing + dev-facing
verbs for managing the sidecar runtime: config status + setup, skills /
tasks / data-types / references listing, KTSP introspection, schema
parsing, server lifecycle, and resolved paths.

Short alias: 'sc'. Both names work identically.

LLM-facing surface: run '<binary> llm info' for the LLM-driving verbs.`,
			}
			root.AddCommand(sidecarCmd)
			application.PutService[CobraCommand](app, GroupNameSidecar, CobraCommand{Cmd: sidecarCmd})

			// `llm` cobra group. LLM-facing surface — JSON-default for
			// every descendant via SetGroupDefault per the command-surface decision
			//
			llmCmd := &cobra.Command{
				Use:   GroupNameLLM,
				Short: "LLM-facing surface (JSON-default output)",
				Long: `The llm group is the LLM-driving entry point for this binary.
Every verb under llm defaults to JSON output (override with --out
rich). Start with 'llm info' for a single-call discovery of the
binary's tools + resources.`,
			}
			console.SetGroupDefault(llmCmd, console.OutputJSON)
			root.AddCommand(llmCmd)
			application.PutService[CobraCommand](app, GroupNameLLM, CobraCommand{Cmd: llmCmd})

			return nil
		})
	}
}

// SCCommandExt appends a subcommand under the `sc` cobra group. Used
// by extensions that today register under root but should live under
// `sc` per the canonical layout (config, skills, contributions, schema,
// dirs, etc.).
//
// Equivalent to CommandExt but targets the sc group's command. On
// binaries WITHOUT `SidecarGroupsExt()` (ktw-hub, ktw-test, …) the
// hook silently no-ops — those binaries don't ship the canonical
// sidecar surface and the missing command is the expected outcome.
func SCCommandExt(cmd *cobra.Command) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddBootstrap("appcore", ".sc."+cmd.Use, func(app *application.Application) error {
			group, err := application.AppUseNamedService[CobraCommand](app, GroupNameSidecar)
			if err != nil {
				return nil // no sc group on this binary — skip silently
			}
			group.Cmd.AddCommand(cmd)
			return nil
		})
	}
}

// LLMCommandExt appends a subcommand under the `llm` cobra group.
// Subcommands inherit the JSON-default output mode via SetGroupDefault
// (per the command-surface decision). On binaries WITHOUT
// `SidecarGroupsExt()` the hook silently no-ops.
func LLMCommandExt(cmd *cobra.Command) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddBootstrap("appcore", ".llm."+cmd.Use, func(app *application.Application) error {
			group, err := application.AppUseNamedService[CobraCommand](app, GroupNameLLM)
			if err != nil {
				return nil // no llm group on this binary — skip silently
			}
			group.Cmd.AddCommand(cmd)
			return nil
		})
	}
}
