// SPDX-License-Identifier: Apache-2.0

package sidecar

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	scinternal "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// FolderConfig is a served folder's own configuration needs, the `config:`
// block of its ktw-host.yaml, in author-friendly YAML: the compiled-sidecar
// config surface (publish.APIKeyDef / ToolDef / AuthDef) for a folder. The
// generic host publishes it as an infra service; a folder member, which runs
// no process, is checked from it in process (CheckFolderConfig).
type FolderConfig struct {
	APIKeys []FolderAPIKey `yaml:"apiKeys,omitempty"`
	Tools   []FolderTool   `yaml:"tools,omitempty"`
	Auth    []FolderAuth   `yaml:"auth,omitempty"`
	Files   []string       `yaml:"files,omitempty"`
}

// FolderAPIKey is an environment variable the folder's content needs.
type FolderAPIKey struct {
	EnvName     string `yaml:"envName"`
	Label       string `yaml:"label,omitempty"`
	Required    bool   `yaml:"required,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// FolderTool is a program the folder's content runs: publish.ToolDef in the
// folder's YAML, its recipes in the publish.InstallRecipe shape
// (`platform`, `kind`, `package`, `url`, `sha256`, `script`, `minVersion`).
type FolderTool struct {
	Name         string                  `yaml:"name"`
	Commands     []string                `yaml:"commands,omitempty"`
	InstallSkill string                  `yaml:"installSkill,omitempty"`
	VersionFlag  string                  `yaml:"versionFlag,omitempty"`
	Version      string                  `yaml:"version,omitempty"`
	Optional     bool                    `yaml:"optional,omitempty"`
	Recipes      []publish.InstallRecipe `yaml:"recipes,omitempty"`
}

// ToolDef is the folder tool as the compiled-sidecar declaration.
func (t FolderTool) ToolDef() publish.ToolDef {
	return publish.ToolDef{
		Name: t.Name, Commands: t.Commands, InstallSkill: t.InstallSkill, VersionFlag: t.VersionFlag,
		Version: t.Version, Optional: t.Optional, Recipes: t.Recipes,
	}
}

// Validate refuses a block an installer could not read: a tool without a
// name, or a recipe that does not validate (publish.ToolDef.Validate).
func (c FolderConfig) Validate() error {
	tools := make([]publish.ToolDef, 0, len(c.Tools))
	for _, t := range c.Tools {
		tools = append(tools, t.ToolDef())
	}
	return publish.ValidateTools(tools)
}

// FolderAuth is a login the folder's content relies on.
type FolderAuth struct {
	Name      string `yaml:"name"`
	Label     string `yaml:"label,omitempty"`
	LoginHint string `yaml:"loginHint,omitempty"`
}

// PublishOptions are the folder's needs as an infra service declares them;
// none when the block declares nothing.
func (c FolderConfig) PublishOptions() []publish.PublishOption {
	var opts []publish.PublishOption
	for _, k := range c.APIKeys {
		opts = append(opts, publish.WithAPIKeys(publish.APIKeyDef{
			EnvName: k.EnvName, Label: k.Label, Required: k.Required, Description: k.Description,
		}))
	}
	for _, t := range c.Tools {
		opts = append(opts, publish.WithTools(t.ToolDef()))
	}
	for _, a := range c.Auth {
		opts = append(opts, publish.WithAuth(publish.AuthDef{
			Name: a.Name, Label: a.Label, LoginHint: a.LoginHint,
		}))
	}
	if len(c.Files) > 0 {
		opts = append(opts, publish.WithConfigFiles(c.Files...))
	}
	return opts
}

// Requirements are the folder's needs as data, the shape a compiled sidecar's
// describe carries: what a catalog copies into the content item's requires.
func (c FolderConfig) Requirements() ([]publish.ToolRequirement, []publish.APIKeyRequirement, []publish.AuthRequirement) {
	var tools []publish.ToolRequirement
	var keys []publish.APIKeyRequirement
	var auth []publish.AuthRequirement
	for _, t := range c.Tools {
		tools = append(tools, t.ToolDef().Requirement())
	}
	for _, k := range c.APIKeys {
		keys = append(keys, publish.APIKeyDef{EnvName: k.EnvName, Label: k.Label, Required: k.Required, Description: k.Description}.Requirement())
	}
	for _, a := range c.Auth {
		auth = append(auth, publish.AuthDef{Name: a.Name, Label: a.Label, LoginHint: a.LoginHint}.Requirement())
	}
	return tools, keys, auth
}

// CheckFolderConfig is the config check of a folder member, in process: the
// report the generic host's `sidecar config check` gives for the same block,
// against the universe at home ("" for this process's own).
func CheckFolderConfig(ctx context.Context, home, name string, c FolderConfig) (api.ConfigCheckReport, error) {
	if err := c.Validate(); err != nil {
		return api.ConfigCheckReport{}, fmt.Errorf("%s: config: %w", name, err)
	}
	deps := scinternal.ConfigDeps{AppName: name, ConfigFiles: c.Files}
	for _, k := range c.APIKeys {
		deps.APIKeys = append(deps.APIKeys, publish.APIKeyDef{EnvName: k.EnvName, Label: k.Label, Required: k.Required, Description: k.Description})
	}
	for _, t := range c.Tools {
		deps.Tools = append(deps.Tools, t.ToolDef())
	}
	for _, a := range c.Auth {
		deps.Auth = append(deps.Auth, publish.AuthDef{Name: a.Name, Label: a.Label, LoginHint: a.LoginHint})
	}
	env := application.Env()
	if home != "" {
		env = application.EnvOf(home)
	}
	return scinternal.NewDefaultConfigService(deps).CheckIn(ctx, env.ConfigDir(), env.HomeDir())
}
