// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// THE OBJECT-CONTEXT PLUGIN. A run with a target object (`_target` in its
// plugin context) gets it from the agent factory: the `<cwm_object_context>`
// sections AgentContext resolved for the object, snapshot at plugin creation;
// the tasks it names as typed tools; and the skills, plugins and action tools
// through the plugin resolver. It always offers `object-effective`. Every
// failure degrades to that tool alone — a chat on an object the service cannot
// resolve is still a chat.

// contextPlugin carries the block in its system message and no tool.
type contextPlugin struct{ block string }

func (p *contextPlugin) Name() string                 { return "object-context-sections" }
func (p *contextPlugin) SystemMessage() string        { return p.block }
func (p *contextPlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (p *contextPlugin) Hooks() agentapi.PluginHooks  { return agentapi.PluginHooks{} }
func (p *contextPlugin) Stable() bool                 { return true }
func (p *contextPlugin) HandleToolCall(context.Context, string, json.RawMessage) (*agentapi.ToolCallResult, error) {
	return nil, fmt.Errorf("the object context has no tool")
}

// render is the block the agent reads: every section in the resolved order,
// then — when the budget trimmed something — which ids, so the agent knows it
// works without them and can fetch what it needs.
func render(out api.AgentContextOutput) string {
	if len(out.System) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<cwm_object_context object="` + out.ObjectID + `">`)
	for _, s := range out.System {
		b.WriteString("\n\n")
		if s.Title != "" {
			b.WriteString("## " + s.Title + "\n")
		}
		b.WriteString(strings.TrimSpace(s.Text))
	}
	var trimmed []string
	for _, n := range out.Notes {
		if n.Kind == api.NoteTrimmed && n.List == "system" {
			trimmed = append(trimmed, n.Key)
		}
	}
	if len(trimmed) > 0 {
		b.WriteString("\n\n(Left out for size: " + strings.Join(trimmed, ", ") + ".)")
	}
	b.WriteString("\n</cwm_object_context>")
	return b.String()
}

// refsOf splits the resolved tools and skills into what this plugin builds
// itself — tasks, as typed tools — and the refs the resolver builds: ONE skill
// ref carrying every selector, each plugin ref, and ONE tool-proxy ref carrying
// every other action URI.
func refsOf(out api.AgentContextOutput) (tasks []string, refs []agentapi.PluginRef) {
	if len(out.Skills) > 0 {
		list := make([]any, len(out.Skills))
		for i, s := range out.Skills {
			list[i] = s.Selector
		}
		refs = append(refs, agentapi.PluginRef{Type: "skill", Props: map[string]any{"skills": list}})
	}
	var uris []any
	for _, t := range out.Tools {
		switch {
		case t.Plugin != "":
			refs = append(refs, agentapi.ParsePluginRef(t.Plugin))
		case isTaskRef(t.URI):
			tasks = append(tasks, t.URI)
		default:
			uris = append(uris, t.URI)
		}
	}
	if len(uris) > 0 {
		refs = append(refs, agentapi.PluginRef{Type: "tool", Props: map[string]any{"tools": uris}})
	}
	return tasks, refs
}

// isTaskRef: a bare task name, or a `<provider>:tasks/<name>` URI.
func isTaskRef(uri string) bool {
	i := strings.Index(uri, ":")
	return i < 0 || strings.HasPrefix(uri[i+1:], "tasks/")
}

// Tools are the plugin's own tools.
func Tools() []toolsapi.Tool {
	return []toolsapi.Tool{
		toolsapi.NewFunctionTool[api.EffectiveInput, api.EffectiveOutput]("object-effective",
			"The objects of one type in force at an object (`id`, `type`, `include_by`): its own, what the type's propagation reaches, what its type injects — e.g. the rules, decisions and patterns that apply here.",
			func(ctx context.Context, in api.EffectiveInput) (api.EffectiveOutput, error) {
				svc, err := application.UseService[api.ObjectContextService](ctx)
				if err != nil || svc == nil {
					return api.EffectiveOutput{}, fmt.Errorf("resolve ObjectContextService: %v", err)
				}
				return svc.Effective(ctx, in)
			}, toolsapi.WithReadOnly()),
	}
}

const pluginSystemMessage = `object-effective lists the objects of one type in force at an object — the
rules, decisions and patterns that apply there, inherited along the tree.`

// create builds the plugins of a run: the tool group always, and for a run on
// an object its context.
func create(ctx context.Context, loop agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	plugins := []agentapi.AgentPlugin{toolsapi.NewToolGroupPlugin(agentapi.ObjectContextPlugin, pluginSystemMessage, Tools())}
	objectID, _ := agentapi.PluginContextFrom(ctx)[agentapi.PluginContextTarget].(string)
	if objectID == "" {
		return plugins, nil
	}
	svc, err := application.UseService[api.ObjectContextService](ctx)
	if err != nil || svc == nil {
		return plugins, nil
	}
	out, err := svc.AgentContext(ctx, api.AgentContextInput{ID: objectID})
	if err != nil {
		log.Printf("[object-context] %q: %v", objectID, err)
		return plugins, nil
	}
	for _, n := range out.Notes {
		if n.Kind == api.NoteError || n.Kind == api.NoteTrimmed {
			log.Printf("[object-context] %s: %s %s %q from %s: %s", objectID, n.Kind, n.List, n.Key, n.Source, n.Reason)
		}
	}
	if block := render(out); block != "" {
		plugins = append(plugins, &contextPlugin{block: block})
	}
	tasks, refs := refsOf(out)
	if p := newNodeActionsPlugin(ctx, tasks); p != nil {
		plugins = append(plugins, p)
	}
	if len(refs) == 0 {
		return plugins, nil
	}
	mgr, err := application.UseService[agentapi.PluginManager](ctx)
	if err != nil || mgr == nil {
		return plugins, nil
	}
	// Re-entrant: the resolver is stateless across calls. A missing skill or
	// plugin is logged and skipped rather than failing the chat.
	extras, err := mgr.CreatePlugins(refs, nil, nil, ctx, loop)
	if err != nil {
		log.Printf("[object-context] %s: plugin resolve failed (refs=%d): %v", objectID, len(refs), err)
		return plugins, nil
	}
	return append(plugins, extras...), nil
}

var pluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
	agentapi.ObjectContextPlugin, "What a chat on an object receives",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{FactoryName: agentapi.ObjectContextPlugin, CreateFunc: create}, nil
	},
)

// AgentPluginExtension contributes the `object-context` PluginFactory. Only a
// binary with the agent loop (ktw-agents) adds it; its factory then attaches
// it to every run with a target object.
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("object-context", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, pluginStrategy)
		})
	}
}
