// SPDX-License-Identifier: Apache-2.0

// Package local wires the sources service into a binary (`<provider>:sources/...`).
// Its CWM pack — the `source-document` type and the `derived-from` edge — is
// contrib.Pack. It needs a CWMService in the binary.
package local

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescore "github.com/katyoti/katyoti-fwk/internal/extensions/sources/internal/core"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// Extension publishes the SourcesService over KTSP.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.SourcesService](
		func(context.Context) (api.SourcesService, error) { return sourcescore.New(), nil },
		publish.WithServiceDef(api.SourcesServiceDef),
		publish.WithKTSPContainer(),
	)
}

// Tools are the source tools.
func Tools() []toolsapi.Tool {
	return []toolsapi.Tool{
		toolsapi.NewFunctionTool[api.IngestInput, api.IngestOutput]("source-ingest",
			"Anchor an outside document's section: read it (a file:// source only inside the project), hash and snapshot it exactly as source-reanchor will, and create or update the source-document recording it.",
			func(ctx context.Context, in api.IngestInput) (api.IngestOutput, error) {
				svc, err := service(ctx)
				if err != nil {
					return api.IngestOutput{}, err
				}
				return svc.Ingest(ctx, in)
			}, toolsapi.WithMutating()),
		toolsapi.NewFunctionTool[api.ReanchorInput, api.ReanchorOutput]("source-reanchor",
			"Read a source-document's source again: match, drift or missing, recorded on the document, and the objects derived from it (derived-from edges) that are now stale or missing.",
			func(ctx context.Context, in api.ReanchorInput) (api.ReanchorOutput, error) {
				svc, err := service(ctx)
				if err != nil {
					return api.ReanchorOutput{}, err
				}
				return svc.Reanchor(ctx, in)
			}, toolsapi.WithMutating()),
		toolsapi.NewFunctionTool[api.StaleInput, api.StaleOutput]("source-stale",
			"List a module's source-documents whose anchor is stale or missing, each with the objects derived from it - what to recheck.",
			func(ctx context.Context, in api.StaleInput) (api.StaleOutput, error) {
				svc, err := service(ctx)
				if err != nil {
					return api.StaleOutput{}, err
				}
				return svc.Stale(ctx, in)
			}, toolsapi.WithReadOnly()),
	}
}

func service(ctx context.Context) (api.SourcesService, error) {
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil || svc == nil {
		return nil, fmt.Errorf("resolve SourcesService: %v", err)
	}
	return svc, nil
}

const pluginSystemMessage = `Source tools anchor an outside document a workspace object is derived from
(source-ingest), read it again (source-reanchor) and list the anchors left stale
or missing (source-stale) - never compute a content hash by hand.`

// pluginStrategy registers the `sources` PluginFactory. Bind via
// `plugins: [sources]` in a TASK.md.
var pluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
	"sources", "Ingest and re-anchor outside source documents",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "sources",
			CreateFunc: func(context.Context, agentapi.LoopContext, []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{toolsapi.NewToolGroupPlugin("sources", pluginSystemMessage, Tools())}, nil
			},
		}, nil
	},
)

// AgentPluginExtension contributes the `sources` PluginFactory. Only a binary
// with the agent loop (ktw-agents) adds it.
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("sources", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, pluginStrategy)
		})
	}
}
