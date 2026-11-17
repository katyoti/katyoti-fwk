// SPDX-License-Identifier: Apache-2.0

package engine

import "context"

// MachineFactory creates a Machine from a serializable request and context.
// The request contains ONLY serializable data (it IS the checkpoint).
// Runtime dependencies (model, plugins, event bus) are resolved from ctx
// via UseService[T](ctx) inside the factory.
//
// Same code runs for fresh start AND restart from checkpoint:
//  1. Load request from checkpoint (or receive from caller)
//  2. factory.CreateMachine(ctx, request)  → resolves deps from context
//  3. executor.Run(ctx, startEvent)        → runs the machine
//
// Each machine type provides its own factory. The agent extension provides
// AgentMachineFactory. The workflow extension provides WorkflowMachineFactory.
type MachineFactory interface {
	// Name identifies this factory (matches the Machine it creates).
	Name() string

	// CreateMachine creates a Machine from a serializable request.
	// ctx carries all runtime dependencies (services, config, prompter, etc.).
	// request is the serializable input (e.g., *AgentRunRequest).
	CreateMachine(ctx context.Context, request any) (Machine, error)
}
