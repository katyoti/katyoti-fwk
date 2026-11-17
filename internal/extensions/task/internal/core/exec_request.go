// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

func (s *Service) getFactory(typeName string) (engine.MachineFactory, error) {
	td := engine.MachineDomain.GetTypeDef(typeName)
	if td.Impl == nil {
		return nil, fmt.Errorf("no factory for type %q in MachineDomain", typeName)
	}
	return td.Impl, nil
}

// resolveSessionID applies the session-ID precedence:
//
//  1. in.SessionID — caller supplied (resume flow, multi-step pipelines).
//  2. Parent run's SessionID — sub-agent / delegated runs ALWAYS share the
//     parent's session directory so all events from a delegation tree land
//     in the same llm.jsonl + are correlatable.
//  3. Fresh UUIDv7 (engine.GenerateSessionID) — root runs.
//
// Resolved BEFORE the exec request is built so factories see the final
// value in Runtime.SessionID (the agent factory places session-scoped
// artifacts like llm-model-calls.jsonl with it).
func (s *Service) resolveSessionID(in taskapi.TaskRunRequest, taskName string) string {
	if in.SessionID != "" {
		return in.SessionID
	}
	if in.ParentRunID != "" {
		if sid := s.reg.sessionIDOf(in.ParentRunID); sid != "" {
			return sid
		}
	}
	return engine.GenerateSessionID(taskName)
}

// buildExecRequest builds a TaskExecRequest with config, input, body, and runtime
// cleanly separated. If the factory implements SchemaProvider, body is injected
// into config under the strategy's declared body field name.
func buildExecRequest(t *taskapi.Task, in taskapi.TaskRunRequest, factory engine.MachineFactory) *taskapi.TaskExecRequest {
	config := t.Config

	// Inject body into config if strategy declares a body field name.
	// Also merge any caller-supplied ConfigOverride (per-invocation config like SpawnPermissions).
	bodyField := engine.GetBodyFieldName(factory)
	if bodyField != "" && t.Body != "" || len(in.ConfigOverride) > 0 {
		merged := make(map[string]any, len(config)+1+len(in.ConfigOverride))
		for k, v := range config {
			merged[k] = v
		}
		if bodyField != "" && t.Body != "" {
			merged[bodyField] = t.Body
		}
		// ConfigOverride wins over task-level config for this invocation.
		for k, v := range in.ConfigOverride {
			merged[k] = v
		}
		config = merged
	}

	return &taskapi.TaskExecRequest{
		Config: config,
		Input:  in.Input,
		Body:   t.Body,
		Runtime: taskapi.RuntimeContext{
			WorkspaceRoot: in.WorkspaceRoot,
			ParentRunID:   in.ParentRunID,
			SessionID:     in.SessionID,
			DryRunMode:    in.DryRunMode,
			CurrentNode:   in.CurrentNode,
			OutputSchema:  engine.ResolveOutputSchema(factory, t.OutputSchema),
		},
	}
}
