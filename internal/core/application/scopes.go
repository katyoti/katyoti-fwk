// SPDX-License-Identifier: Apache-2.0

package application

import "github.com/katyoti/katyoti-fwk/internal/core/services"

// Scope aliases — re-exported from services for convenience.
// Use these at runtime. For framework-level scope work, import services directly.

// Scope defines a lifecycle boundary for service instances.
type Scope = services.Scope

// ScopeContainer is a per-scope service cache.
type ScopeContainer = services.ScopeContainer

// Predefined scopes.
var (
	ApplicationScope = &services.ApplicationScope
	SessionScope     = services.SessionScope
	TaskScope        = services.TaskScope
)

// NewTransientScope creates a new transient scope with the given name.
var NewTransientScope = services.NewTransientScope
