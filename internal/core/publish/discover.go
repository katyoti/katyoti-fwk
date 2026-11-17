// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
)

// ServiceEntry describes a discovered published service instance.
type ServiceEntry[T any] struct {
	// Name is the service instance name (e.g., "doc-create", "docs").
	Name string

	// Container is the source container (e.g., "ktw-docs", "local").
	Container string

	// Service is the resolved service instance.
	Service T

	// Source indicates where the service was found: "local" or "remote".
	Source string
}

// FindOptions configures service discovery.
type FindOptions struct {
	// LocalOnly restricts discovery to local DI services (no remote containers).
	LocalOnly bool

	// ContainerFilter restricts to a specific container name (empty = all).
	ContainerFilter string
}

// FindPublishedServices discovers all published service instances of type T.
//
// Discovery uses the service interface type name (config.TypeName[T]()) as the
// unified identifier — the same name used for local DI and remote SlotType matching.
//
// Sources:
//  1. Published services EM point: entries where ServiceType.Name() == TypeName[T]()
//  2. Remote containers: (future) containers with SlotType == TypeName[T]()
func FindPublishedServices[T any](ctx context.Context, opts FindOptions) ([]ServiceEntry[T], error) {
	typeName := config.TypeName[T]()
	var entries []ServiceEntry[T]

	// Source 1: Published services EM point — match by service type name.
	published := application.UseExtensions[PublishedService](ctx)
	for _, ps := range published {
		if ps.ServiceType == nil || ps.ServiceType.Name() != typeName {
			continue
		}
		if opts.ContainerFilter != "" && opts.ContainerFilter != "local" {
			continue
		}
		// Resolve the service instance from DI.
		svc, err := application.UseNamedService[T](ctx, "default")
		if err != nil {
			continue
		}
		entries = append(entries, ServiceEntry[T]{
			Name:      ps.Name,
			Container: "local",
			Service:   svc,
			Source:    "local",
		})
	}

	// Source 2: Remote containers (future — transport-agnostic).
	// Will query discovered containers for slots with SlotType == typeName
	// and create typed stubs that implement T.
	if !opts.LocalOnly {
		// TODO: integrate with container discovery
	}

	return entries, nil
}
