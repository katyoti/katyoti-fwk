// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// verificationTool builds a tool whose handler resolves the
// VerificationService from context and forwards the typed call.
func verificationTool[I, O any](name, desc string, call func(api.VerificationService, context.Context, I) (O, error), opts ...toolsapi.ToolOption) toolsapi.Tool {
	return toolsapi.NewFunctionTool[I, O](name, desc,
		func(ctx context.Context, in I) (O, error) {
			var zero O
			svc, err := application.UseService[api.VerificationService](ctx)
			if err != nil || svc == nil {
				return zero, fmt.Errorf("resolve VerificationService: %v", err)
			}
			return call(svc, ctx, in)
		}, opts...)
}

// Tools are the verification tools: verify an object and everything under
// it, lint an object (or its subtree) or a module. Call them to verify — never assert
// verification in prose; the result is what the hooks answered.
func Tools() []toolsapi.Tool {
	return []toolsapi.Tool{
		verificationTool("object-lint",
			"Lint one object (id; with subtree: true, the object and everything under it) or a whole module (module) — exactly one of id and module: run every lint hook in force and return the findings (rule, message, severity). Findings, never a verdict.",
			func(s api.VerificationService, ctx context.Context, in api.LintInput) (api.LintOutput, error) {
				return s.Lint(ctx, in)
			}, toolsapi.WithReadOnly()),
		verificationTool("object-verify",
			"Verify an object and everything under it: run every verify hook in force (acceptance criteria record a verdict each) and return {objects, runs, passed, failed[], unproven[]}. Only pass is a pass; unproven is neither. Lint findings are object-lint's.",
			func(s api.VerificationService, ctx context.Context, in api.VerifyInput) (api.VerifyOutput, error) {
				return s.Verify(ctx, in)
			}, toolsapi.WithMutating()),
	}
}

const pluginSystemMessage = `Verification tools read the verify and lint hook points over workspace
objects: object-verify runs the acceptance checks under an object and records
their verdicts; object-lint returns the findings of the linters in force.`

// pluginStrategy registers the `verification` PluginFactory. Bind via
// `plugins: [verification]` in a TASK.md.
var pluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
	"verification", "Verify and lint workspace objects",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "verification",
			CreateFunc: func(context.Context, agentapi.LoopContext, []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{toolsapi.NewToolGroupPlugin("verification", pluginSystemMessage, Tools())}, nil
			},
		}, nil
	},
)

// AgentPluginExtension contributes the `verification` PluginFactory. Only a
// binary with the agent loop (ktw-agents) adds it.
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("verification", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, pluginStrategy)
		})
	}
}
