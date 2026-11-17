// SPDX-License-Identifier: Apache-2.0

// Package api defines the public types for the sidecar contract services.
// These interfaces are implemented by the generic defaults in internal/.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// Sidecar provides identity and introspection for a sidecar binary.
// Every sidecar publishes this service via SidecarExt(efs).
type Sidecar interface {
	// Info returns a structured report of the sidecar's identity, services,
	// required tools, config files, skills, and capabilities.
	Info(ctx context.Context) (InfoReport, error)
}

// SidecarServiceDef is the service definition for the Sidecar service.
var SidecarServiceDef = meta.ServiceDef{
	Name:    "sidecar",
	Short:   "Sidecar identity and introspection",
	Long:    "Provides a structured report of the sidecar's identity, published services, required tools, config files, skills, and capabilities.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Info": meta.Method("Return sidecar identity and introspection report"),
	},
}

// InfoReport is the structured introspection response.
// One call to Info() gives an external caller (installer, LLM, operator)
// everything it needs to know about the sidecar with zero prior knowledge.
type InfoReport struct {
	Name          string           `json:"name"           description:"Binary name (e.g. ktw-docs)"`
	Version       string           `json:"version"        description:"Semantic version"`
	Description   string           `json:"description"    description:"One-sentence purpose"`
	Services      []ServiceInfo    `json:"services"       description:"Published domain services"`
	RequiredTools []ToolInfo       `json:"required_tools" description:"External programs the sidecar needs"`
	RequiredKeys  []APIKeyInfo     `json:"required_keys"  description:"Environment variables for API keys"`
	ConfigFiles   []ConfigFileInfo `json:"config_files"   description:"Config files managed by the sidecar"`
	Skills        []SkillInfo      `json:"skills"         description:"Advertised skills (name + description only)"`
	Capabilities  CapabilityFlags  `json:"capabilities"   description:"Transport capabilities"`
}

// ServiceInfo describes a published domain service.
type ServiceInfo struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Version     string       `json:"version"`
	Methods     []MethodInfo `json:"methods"`
}

// MethodInfo describes a published method on a service.
type MethodInfo struct {
	Name        string `json:"name"`
	ToolName    string `json:"tool_name"    description:"Sidecar-unique callable identity"`
	Description string `json:"description"`
}

// ToolInfo describes an external program dependency.
type ToolInfo struct {
	Name            string `json:"name"`
	RequiredVersion string `json:"required_version,omitempty"`
	FoundVersion    string `json:"found_version,omitempty"`
	OK              bool   `json:"ok"`
	Hint            string `json:"hint,omitempty"`
}

// APIKeyInfo describes a required or optional environment variable.
type APIKeyInfo struct {
	EnvName     string `json:"env_name"`
	Required    bool   `json:"required"`
	Set         bool   `json:"set"`
	Description string `json:"description"`
}

// ConfigFileInfo describes a config file managed by the sidecar.
type ConfigFileInfo struct {
	Name   string `json:"name"   description:"File name (e.g. DocService.yaml)"`
	Path   string `json:"path"   description:"Resolved path on disk"`
	Exists bool   `json:"exists" description:"Whether the file exists on disk"`
}

// SkillInfo describes an advertised skill (L0 metadata only).
type SkillInfo struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Description string `json:"description"`
}

// CapabilityFlags indicates which transport modes the sidecar supports.
type CapabilityFlags struct {
	CLI         bool `json:"cli"`
	KTSP        bool `json:"ktsp"`
	HTTP        bool `json:"http"`
	MCP         bool `json:"mcp"`
	TaskEnabled bool `json:"task_enabled"`
}
