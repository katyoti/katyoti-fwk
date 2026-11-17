// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// Re-export types from core/publish for backward compatibility.
// New code should import core/publish directly.
type PublishedService = publish.PublishedService
type Capability = publish.Capability
type KTSPContainerCap = publish.KTSPContainerCap
type HTTPServerCap = publish.HTTPServerCap
type AgentPluginCap = publish.AgentPluginCap
type PublishOption = publish.PublishOption
type ToolDef = publish.ToolDef
type APIKeyDef = publish.APIKeyDef
type AuthDef = publish.AuthDef

// Re-export options
var (
	WithDescription   = publish.WithDescription
	WithServiceDef    = publish.WithServiceDef
	WithVersion       = publish.WithVersion
	WithKTSPContainer = publish.WithKTSPContainer
	WithKTSPSlotTags  = publish.WithKTSPSlotTags
	WithHTTPServer    = publish.WithHTTPServer
	WithAgentPlugin   = publish.WithAgentPlugin
	WithConfigFiles   = publish.WithConfigFiles
	WithoutConfigFile = publish.WithoutConfigFile
	WithConfigDirs    = publish.WithConfigDirs
	WithTools         = publish.WithTools
	WithAPIKeys       = publish.WithAPIKeys
	WithAuth          = publish.WithAuth
	WithLocal         = publish.WithLocal
)

// Re-export EM point
var NewPublishedServicesPoint = publish.NewPublishedServicesPoint

// Re-export publish functions (generic functions require wrapper)

// PublishService declares a published service with config.
func PublishService[TService any, TConfig any](
	factory func(ctx context.Context, cfg *TConfig) (TService, error),
	opts ...PublishOption,
) application.Extension {
	return publish.PublishService[TService, TConfig](factory, opts...)
}

// PublishSimpleService declares a published service with no config.
func PublishSimpleService[TService any](
	factory func(ctx context.Context) (TService, error),
	opts ...PublishOption,
) application.Extension {
	return publish.PublishSimpleService[TService](factory, opts...)
}

// DeclareInfra contributes infrastructure needs as a PublishedService with no service factory.
var DeclareInfra = publish.DeclareInfra
