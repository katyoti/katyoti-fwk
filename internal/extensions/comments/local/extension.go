// SPDX-License-Identifier: Apache-2.0

// Package local wires the comments service into a binary
// (`<provider>:comments/...`) and its agent tools (plugin `comments`). Its CWM
// pack — the `comments` capability — is contrib.Pack.
package local

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/comments/api"
	commentscore "github.com/katyoti/katyoti-fwk/internal/extensions/comments/internal/core"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// Extension publishes the CommentsService over KTSP.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.CommentsService](
		func(context.Context) (api.CommentsService, error) { return commentscore.New(), nil },
		publish.WithServiceDef(api.CommentsServiceDef),
		publish.WithKTSPContainer(),
	)
}

func commentsTool[I, O any](name, desc string, call func(api.CommentsService, context.Context, I) (O, error), opts ...toolsapi.ToolOption) toolsapi.Tool {
	return toolsapi.NewFunctionTool[I, O](name, desc,
		func(ctx context.Context, in I) (O, error) {
			var zero O
			svc, err := application.UseService[api.CommentsService](ctx)
			if err != nil || svc == nil {
				return zero, fmt.Errorf("resolve CommentsService: %v", err)
			}
			return call(svc, ctx, in)
		}, opts...)
}

// Tools are the comment tools.
func Tools() []toolsapi.Tool {
	return []toolsapi.Tool{
		commentsTool("comment-list", "Read an object's comments, oldest first: who said what, and what each answers.",
			func(s api.CommentsService, ctx context.Context, in api.ListInput) (api.ListOutput, error) {
				return s.List(ctx, in)
			},
			toolsapi.WithReadOnly()),
		commentsTool("comment-add", "Say something about an object, or answer one of its comments (reply_to). Comments are kept beside the object and never change it.",
			func(s api.CommentsService, ctx context.Context, in api.AddInput) (api.AddOutput, error) {
				return s.Add(ctx, in)
			},
			toolsapi.WithMutating()),
	}
}

const pluginSystemMessage = `Comment tools: read what was said about a workspace object, say something
about it, or answer one of its comments.`

var pluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
	"comments", "Comment on workspace objects",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "comments",
			CreateFunc: func(context.Context, agentapi.LoopContext, []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{toolsapi.NewToolGroupPlugin("comments", pluginSystemMessage, Tools())}, nil
			},
		}, nil
	},
)

// AgentPluginExtension contributes the `comments` PluginFactory. Only a binary
// with the agent loop (ktw-agents) adds it.
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("comments", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, pluginStrategy)
		})
	}
}
