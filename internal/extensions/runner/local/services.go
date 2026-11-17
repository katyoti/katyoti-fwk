// SPDX-License-Identifier: Apache-2.0

// Package local provides the in-process implementation of RunnerService.
// RunnerService lists LLM profiles only — task listing moved to
// `tasks list` (TaskService) which sources the same data through the
// universe-aware composite library.
//
// CLI surface: `<binary> sc llm-profiles list` (ktw-agents only — this
// extension is wired into ktw-agents' bootstrap and no one else's).
// KTSP/HTTP surface: slot `llm-profiles`, method `list`. AutoKTSPContainer's
// top-level CLI auto-generation is SUPPRESSED via WithKTSPSlotTags({"cli":
// "false"}) so the only CLI path is the manual `sc llm-profiles list`
// wiring below.
package local

import (
	"context"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	runnerapi "github.com/katyoti/katyoti-fwk/internal/extensions/runner/api"
	"github.com/spf13/cobra"
)

type service struct{}

func NewService(_ context.Context) (runnerapi.RunnerService, error) {
	return &service{}, nil
}

// Extension registers RunnerService + wires the `sc llm-profiles`
// Cobra subgroup. One line in ktw-agents' bootstrap.go.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		// 1) Publish RunnerService — KTSP/HTTP enabled, top-level CLI suppressed.
		init.AddExtension(appcore.PublishSimpleService[runnerapi.RunnerService](
			NewService,
			appcore.WithServiceDef(runnerapi.RunnerServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
		))

		// 2) Manually wire `sc llm-profiles list` under the sc cobra group.
		init.AddExtension(appcore.SCCommandExt(newLLMProfilesCommand()))
	}
}

// newLLMProfilesCommand builds the `llm-profiles` cobra subgroup that
// lives under `sc`. Today only `list` is exposed — additional verbs
// (get / set-default / validate) will plug in here.
func newLLMProfilesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "llm-profiles",
		Short: "LLM profile management (list configured profiles)",
		Long: `'sc llm-profiles' is ktw-agents' LLM-profile management surface.
Profiles are loaded from LLMProfiles.yaml; the agent loop resolves
which profile to use per-task via the runtime/profile-resolver chain.

Note: this command exists ONLY on ktw-agents — sidecars don't carry
LLM profiles directly.`,
	}
	cmd.AddCommand(newLLMProfilesListCommand())
	return cmd
}

func newLLMProfilesListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured LLM profiles",
	}
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[runnerapi.RunnerService](cmd.Context())
		if err != nil {
			return err
		}
		out, err := svc.List(cmd.Context(), runnerapi.ListProfilesInput{})
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "llm-profiles-list", out, "")
	}
	return cmd
}

func (s *service) List(_ context.Context, _ runnerapi.ListProfilesInput) (runnerapi.ListProfilesOutput, error) {
	cs := application.Config()
	cfgMap, ok := cs.GetMap("LLMProfiles")
	if !ok {
		return runnerapi.ListProfilesOutput{}, nil
	}

	defaultProfile, _ := cfgMap["default"].(string)
	profilesRaw := normalizeMap(cfgMap["profiles"])

	var profiles []runnerapi.ProfileInfo
	var names []string
	for name := range profilesRaw {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		prof := normalizeMap(profilesRaw[name])
		label := name
		if name == defaultProfile {
			label = name + " *"
		}

		modelName := resolveModelName(prof)
		maxTurns := "-"
		if mt, ok := prof["max_turns"]; ok {
			maxTurns = fmt.Sprintf("%v", mt)
		}

		profiles = append(profiles, runnerapi.ProfileInfo{
			Name:     label,
			Model:    modelName,
			MaxTurns: maxTurns,
		})
	}

	return runnerapi.ListProfilesOutput{
		Default:  defaultProfile,
		Profiles: profiles,
	}, nil
}

// normalizeMap coerces both map[string]any and map[any]any (the latter is
// what some YAML decoders produce for nested maps) into map[string]any.
// Returns nil if the input is neither.
func normalizeMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			if ks, ok := k.(string); ok {
				out[ks] = val
			}
		}
		return out
	}
	return nil
}

func resolveModelName(prof map[string]any) string {
	if prof == nil {
		return "-"
	}
	model := prof["model"]
	if model == nil {
		return "-"
	}
	if s, ok := model.(string); ok {
		return s
	}
	if m, ok := model.(map[string]any); ok {
		if name, ok := m["name"].(string); ok {
			return name
		}
	}
	return "-"
}
