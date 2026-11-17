// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// The manifest DTOs live in sidecar/api; aliases point downward at the lighter
// package. This file keeps the builder, which needs a live KTSP container.
type (
	EndpointManifest     = sidecarapi.EndpointManifest
	ManifestCapabilities = sidecarapi.ManifestCapabilities
)

func BuildEndpointManifest(name, version, description string, transports []string, container ktsp.Container) EndpointManifest {
	ktspInfo := ktsp.GenerateManifest(container)
	if transports == nil {
		transports = []string{"stdio"}
	}
	return EndpointManifest{
		Name:        name,
		Version:     version,
		Description: description,
		Transports:  transports,
		KTSP:        ktspInfo,
	}
}
