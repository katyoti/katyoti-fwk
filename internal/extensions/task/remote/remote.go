// SPDX-License-Identifier: Apache-2.0

// Package remote is the bus-backed TaskService for binaries that do not host
// the task service: every method is one round-trip to ktw-agents, which does
// . A sidecar wires RemoteExtension and its services resolve
// TaskService the standard way - application.UseService[taskapi.TaskService](ctx)
// - to start a run and follow it, without knowing the service is elsewhere.
//
// The methods are GENERATED from the interface (zz_client.go, `go generate`);
// TestGeneratedClientIsCurrent refuses a stale file. BackgroundManager and
// SuspensionManager are host-local: they answer nil here.
//
// HOST vs CONSUMER: a binary either hosts the task service (task/local) or
// consumes it (RemoteExtension). Never both.
package remote

//go:generate go run ./gen

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore/busclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// Host is the binary that publishes TaskService.
const Host = "ktw-agents"

// New returns the client addressing the task service on its host.
func New() Client { return NewClient(busclient.New(Host, taskapi.TaskServiceDef)) }

// RemoteExtension registers the client as this binary's TaskService.
func RemoteExtension() application.Extension {
	return application.DeclareSimpleService[taskapi.TaskService](
		func(context.Context) (taskapi.TaskService, error) { return New(), nil },
	)
}
