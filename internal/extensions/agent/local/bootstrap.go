// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	coreprovider "github.com/katyoti/katyoti-fwk/internal/core/provider"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	iplugin "github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/plugin"
	iprofile "github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/profile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/runtime"
	ktspengine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	iproviders "github.com/katyoti/katyoti-fwk/internal/models/local"
)

// NewPluginDomainExt returns an Extension that bootstraps the plugin domain.
func NewPluginDomainExt() application.Extension {
	return iplugin.NewPluginDomainExt()
}

// PluginsYAMLProvider creates a config provider from embedded + user plugin declarations.
func NewPluginsYAMLProvider(embedded, user []api.PluginRef) *iplugin.PluginsYAMLProvider {
	return &iplugin.PluginsYAMLProvider{
		EmbeddedPlugins: embedded,
		UserPlugins:     user,
	}
}

// PluginDomain re-exports the internal plugin domain for external type contributions.
var PluginDomain = iplugin.PluginDomain

// PluginManager type re-export for DI deps.
type PluginManager = iplugin.PluginManager

// StubSenderFactory is a SenderFactory that always returns an error.
// Used for boot-time registration — actual transport wiring happens at session time.
var StubSenderFactory = SenderFactory(func(containerName string) (ktsp.Sender, error) {
	return nil, fmt.Errorf("no sender for container %q (sidecar not started)", containerName)
})

// ResolveVariables resolves built-in variables for a session.
var ResolveVariables = iplugin.ResolveVariables

// ExpandVariables expands ${var} references in a string.
var ExpandVariables = iplugin.ExpandVariables

// MergeTaskContext merges task + CLI context with variable expansion.
var MergeTaskContext = iplugin.MergeTaskContext

// Dispatcher is re-exported for KTSP tool dispatching.
type Dispatcher = runtime.Dispatcher

// NewDispatcher creates an empty tool dispatcher.
func NewDispatcher() *Dispatcher {
	return runtime.NewDispatcher()
}

// WithPermissionProfile re-exports runtime.WithPermissionProfile so
// CLI commands in app/ktw-agents can set the active permission profile
// in context without importing the internal runtime package directly.
//
//	ctx = local.WithPermissionProfile(ctx, "plan-mode")
func WithPermissionProfile(ctx context.Context, name string) context.Context {
	return runtime.WithPermissionProfile(ctx, name)
}

// PermissionProfileFrom re-exports runtime.PermissionProfileFrom so callers
// (e.g. TaskService) can check whether an EXPLICIT profile is already set in
// context before applying a task-declared default — explicit selection wins.
func PermissionProfileFrom(ctx context.Context) string {
	return runtime.PermissionProfileFrom(ctx)
}

// ContributeStrategy contributes plugin strategies to the domain.
func ContributeStrategy(app *application.Application, defs ...api.PluginStrategyDef) error {
	return strategies.ContributeStrategy(app, defs...)
}

// ContributeProvider contributes a config provider to the plugin domain.
func ContributeProvider(app *application.Application, providers ...coreprovider.ConfigProvider) error {
	return api.ContributePluginProvider(app, providers...)
}

// ProvidersExtension registers the LLM model domain with provider strategies.
var ProvidersExtension = iproviders.Extension

// ProfilesExtension registers LLMProfiles as a DI service.
var ProfilesExtension = iprofile.LLMProfilesExtension

// AgentFactoryExtension registers AgentMachineFactory as a DI service.
// Used by the executor for restart: UseService[*AgentMachineFactory](ctx).
var AgentFactoryExtension = application.DeclareSimpleService[*AgentMachineFactory](newAgentFactory)

func newAgentFactory(ctx context.Context) (*AgentMachineFactory, error) {
	profileMgr, err := application.UseService[iprofile.LLMProfiles](ctx)
	if err != nil {
		return nil, fmt.Errorf("AgentMachineFactory: resolve LLMProfiles: %w", err)
	}
	pluginMgr, err := application.UseService[iplugin.PluginManager](ctx)
	if err != nil {
		return nil, fmt.Errorf("AgentMachineFactory: resolve PluginManager: %w", err)
	}
	return NewAgentMachineFactory(profileMgr, pluginMgr), nil
}

// MachineStrategyExtension contributes the agent-loop machine strategy.
// Registered under THREE names: "agent-loop" (internal), "agent" (canonical TASK.md name), "task" (backward compat).
var MachineStrategyExtension application.Extension = func(init *application.ApplicationInit) {
	init.AddExtension(AgentFactoryExtension)
	init.AddContribute("machine", ".agent-loop", func(app *application.Application) error {
		strategy := &agentLoopStrategy{}
		return strategies.ContributeStrategy(app,
			ktspengine.MachineStrategyDef{
				DefMeta:  strategies.DefMeta{Name: "agent-loop", Label: "LLM agent loop"},
				Strategy: strategy,
			},
			ktspengine.MachineStrategyDef{
				DefMeta:  strategies.DefMeta{Name: "agent", Label: "LLM agent (canonical name)"},
				Strategy: strategy,
			},
			ktspengine.MachineStrategyDef{
				DefMeta:  strategies.DefMeta{Name: "task", Label: "LLM agent task (backward compat alias)"},
				Strategy: strategy,
			},
		)
	})
}

// agentLoopStrategy implements Strategy[MachineFactory] for agent-loop machines.
type agentLoopStrategy struct{}

func (s *agentLoopStrategy) NewTypeConfig() (any, func(context.Context) (ktspengine.MachineFactory, error)) {
	return nil, func(ctx context.Context) (ktspengine.MachineFactory, error) {
		profileMgr, err := application.UseService[iprofile.LLMProfiles](ctx)
		if err != nil {
			return nil, fmt.Errorf("agent-loop: resolve LLMProfiles: %w", err)
		}
		pluginMgr, err := application.UseService[iplugin.PluginManager](ctx)
		if err != nil {
			return nil, fmt.Errorf("agent-loop: resolve PluginManager: %w", err)
		}
		return NewAgentMachineFactory(profileMgr, pluginMgr), nil
	}
}

func (s *agentLoopStrategy) CreateTypeDef(name string, factory ktspengine.MachineFactory) strategies.TypeDef[ktspengine.MachineFactory] {
	return strategies.TypeDef[ktspengine.MachineFactory]{
		DefMeta:  strategies.DefMeta{Name: name, Label: "Agent loop: " + name},
		Strategy: "agent-loop",
		Impl:     factory,
	}
}
