// SPDX-License-Identifier: Apache-2.0

// Package core provides generic default implementations of the sidecar contract services.
package core

import (
	"context"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	skillsapi "github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
)

// SidecarIdentity holds the static identity fields set at bootstrap time.
type SidecarIdentity struct {
	AppName     string
	Version     string
	Description string
}

// DefaultSidecar is the generic Sidecar implementation.
// It reads from PublishedServicesPoint + application.Env() to build InfoReport.
type DefaultSidecar struct {
	Identity SidecarIdentity
}

// NewDefaultSidecar creates a DefaultSidecar with the given identity.
func NewDefaultSidecar(id SidecarIdentity) *DefaultSidecar {
	return &DefaultSidecar{Identity: id}
}

// Info implements api.Sidecar.
func (s *DefaultSidecar) Info(ctx context.Context) (api.InfoReport, error) {
	report := api.InfoReport{
		Name:        s.Identity.AppName,
		Version:     s.Identity.Version,
		Description: s.Identity.Description,
		Capabilities: api.CapabilityFlags{
			CLI: true, // always true for any sidecar
		},
	}

	// Aggregate from PublishedService entries via context
	published := application.UseExtensions[publish.PublishedService](ctx)
	for _, ps := range published {
		// Services
		if ps.Definition != nil {
			si := api.ServiceInfo{
				Name:        ps.Definition.Name,
				Description: ps.Definition.Description(),
				Version:     ps.Definition.Version,
			}
			for mName, mDef := range ps.Definition.Methods {
				si.Methods = append(si.Methods, api.MethodInfo{
					Name:        mName,
					ToolName:    toolNameFromMethod(s.Identity.AppName, ps.Definition.Name, mName, mDef.CLIName),
					Description: mDef.Description(),
				})
			}
			report.Services = append(report.Services, si)
		}

		// Tools (external programs)
		for _, t := range ps.Tools {
			report.RequiredTools = append(report.RequiredTools, api.ToolInfo{
				Name: t.Name,
				OK:   true, // checked at runtime by Check, not Info
			})
		}

		// API keys
		for _, k := range ps.APIKeys {
			report.RequiredKeys = append(report.RequiredKeys, api.APIKeyInfo{
				EnvName:     k.EnvName,
				Required:    k.Required,
				Set:         os.Getenv(k.EnvName) != "",
				Description: k.Description,
			})
		}

		// Config files
		for _, f := range ps.ConfigFiles {
			report.ConfigFiles = append(report.ConfigFiles, api.ConfigFileInfo{
				Name: f,
			})
		}

		// Capabilities
		if ps.HasCapability("ktsp-container") {
			report.Capabilities.KTSP = true
		}
		if ps.HasCapability("http-server") {
			report.Capabilities.HTTP = true
		}
		if ps.HasCapability("agent-plugin") {
			report.Capabilities.TaskEnabled = true
		}
	}

	// Populate skills from the catalog (post step-13 cutover —
	// SkillManager no longer carries the List method).
	if cat, err := application.UseService[mbapi.Catalog](ctx); err == nil && cat != nil {
		if skills, err := skillsapi.SkillsFromCatalog(ctx, cat); err == nil {
			for _, s := range skills {
				report.Skills = append(report.Skills, api.SkillInfo{
					Name:        s.URI(),
					Description: s.Description,
				})
			}
		}
	}

	return report, nil
}

// toolNameFromMethod derives the canonical Tool URI for a service
// method per the URI grammar:
// `<provider>:<slot>/<method-kebab>` (default instance — parens
// omitted). When the service def declares a custom CLIName, that wins
// over the kebab-cased Go method name as the URI's method segment.
func toolNameFromMethod(provider, serviceName, methodName, cliName string) string {
	method := cliName
	if method == "" {
		method = toKebab(methodName)
	}
	return provider + ":" + serviceName + "/" + method
}

// toKebab converts PascalCase to kebab-case.
func toKebab(s string) string {
	var result []byte
	for i, c := range s {
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				result = append(result, '-')
			}
			result = append(result, byte(c)+32)
		} else {
			result = append(result, byte(c))
		}
	}
	return string(result)
}
