// SPDX-License-Identifier: Apache-2.0

// Package agentplugin registers the `person-language` agent plugin: it tells a
// run the language of the person it works for (Profile.language), so the
// agent's replies and the documents it writes follow it.
//
// A package of its own, as skills/agentplugin: only a binary that runs the
// agent loop links it.
package agentplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/remote"
)

// PersonLanguagePlugin is the agent plugin that tells a run the person's
// language (Profile.language): its replies and the documents it writes follow
// it. Its block is api.LanguagePrompt.
const PersonLanguagePlugin = "person-language"

// languageOf reads the person's language from the profile service in the
// admin universe; "" when it cannot be reached or none is set.
var languageOf = func(ctx context.Context) string {
	sender := appcore.NewAdminHubSender()
	defer sender.Close()
	p, err := remote.NewRemote(sender).Get(ctx)
	if err != nil {
		slog.Debug("person-language: the profile is not reachable; no language hint", "err", err)
		return ""
	}
	return p.Language
}

var personLanguageStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
	PersonLanguagePlugin, "Tell the agent the person's language",
	func() (agentapi.PluginFactory, error) { return personLanguageFactory(), nil },
)

// personLanguageFactory makes the plugin for a run: the block for the
// person's language, or no plugin when none is known.
func personLanguageFactory() agentapi.PluginFactory {
	return &agentapi.FuncFactory{
		FactoryName: PersonLanguagePlugin,
		CreateFunc: func(ctx context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
			prompt := api.LanguagePrompt(languageOf(ctx))
			if prompt == "" {
				return nil, nil
			}
			return []agentapi.AgentPlugin{&languagePlugin{prompt: prompt}}, nil
		},
	}
}

// Extension contributes the `person-language` plugin. Only a binary
// with the agent loop (ktw-agents) adds it; a run gets it through a plugin
// group that lists it (Plugins.yaml `standard`).
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("profile", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, personLanguageStrategy)
		})
	}
}

// languagePlugin is a system-prompt block and nothing else.
type languagePlugin struct{ prompt string }

func (p *languagePlugin) Name() string                 { return PersonLanguagePlugin }
func (p *languagePlugin) SystemMessage() string        { return p.prompt }
func (p *languagePlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (p *languagePlugin) Stable() bool                 { return true }
func (p *languagePlugin) Hooks() agentapi.PluginHooks  { return agentapi.PluginHooks{} }
func (p *languagePlugin) HandleToolCall(_ context.Context, name string, _ json.RawMessage) (*agentapi.ToolCallResult, error) {
	return nil, fmt.Errorf("%s handles no tool (got %q)", PersonLanguagePlugin, name)
}
