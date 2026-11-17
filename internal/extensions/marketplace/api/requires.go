// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// ToolNeed is a program a catalog item needs (publish.ToolRequirement as the
// catalog carries it), with the sidecars that declare it.
type ToolNeed struct {
	Name string `yaml:"name" json:"name"`
	// Required: a sidecar cannot run without it; an optional program is
	// offered, never preselected.
	Required bool `yaml:"required,omitempty" json:"required,omitempty"`
	// Version is the highest version a declaring sidecar is validated with.
	Version      string   `yaml:"version,omitempty" json:"version,omitempty"`
	Commands     []string `yaml:"commands,omitempty" json:"commands,omitempty"`
	InstallSkill string   `yaml:"installSkill,omitempty" json:"installSkill,omitempty"`
	// Platforms are the platforms (runtime.GOOS) a recipe installs it on.
	Platforms []string `yaml:"platforms,omitempty" json:"platforms,omitempty"`
	// Recipes are the structured install steps, per platform.
	Recipes []publish.InstallRecipe `yaml:"recipes,omitempty" json:"recipes,omitempty"`
	// NeededBy are the sidecars that declare it.
	NeededBy []string `yaml:"neededBy,omitempty" json:"neededBy,omitempty"`
}

// KeyNeed is a key (an environment variable) a catalog item asks for.
type KeyNeed struct {
	Name        string   `yaml:"name" json:"name"` // the variable, e.g. ANTHROPIC_API_KEY
	Label       string   `yaml:"label,omitempty" json:"label,omitempty"`
	Required    bool     `yaml:"required,omitempty" json:"required,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	NeededBy    []string `yaml:"neededBy,omitempty" json:"neededBy,omitempty"`
}

// LoginNeed is a login a catalog item relies on.
type LoginNeed struct {
	Name        string   `yaml:"name" json:"name"`
	Label       string   `yaml:"label,omitempty" json:"label,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	LoginHint   string   `yaml:"loginHint,omitempty" json:"loginHint,omitempty"`
	NeededBy    []string `yaml:"neededBy,omitempty" json:"neededBy,omitempty"`
}

// An older catalog listed bare names under tools, apiKeys and auth: a scalar
// still loads, as a need of that name.

func (n *ToolNeed) UnmarshalYAML(node *yaml.Node) error {
	type plain ToolNeed
	return unmarshalNeedYAML(node, &n.Name, (*plain)(n))
}

func (n *KeyNeed) UnmarshalYAML(node *yaml.Node) error {
	type plain KeyNeed
	return unmarshalNeedYAML(node, &n.Name, (*plain)(n))
}

func (n *LoginNeed) UnmarshalYAML(node *yaml.Node) error {
	type plain LoginNeed
	return unmarshalNeedYAML(node, &n.Name, (*plain)(n))
}

func (n *ToolNeed) UnmarshalJSON(b []byte) error {
	type plain ToolNeed
	return unmarshalNeedJSON(b, &n.Name, (*plain)(n))
}

func (n *KeyNeed) UnmarshalJSON(b []byte) error {
	type plain KeyNeed
	return unmarshalNeedJSON(b, &n.Name, (*plain)(n))
}

func (n *LoginNeed) UnmarshalJSON(b []byte) error {
	type plain LoginNeed
	return unmarshalNeedJSON(b, &n.Name, (*plain)(n))
}

func unmarshalNeedYAML(node *yaml.Node, name *string, full any) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(name)
	}
	return node.Decode(full)
}

func unmarshalNeedJSON(b []byte, name *string, full any) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, name)
	}
	return json.Unmarshal(b, full)
}

// NeedsOf is what one sidecar's declarations (its describe, or a content
// folder's config block) put in its catalog item's requires: each program,
// key and login, needed by that sidecar.
func NeedsOf(sidecar string, tools []publish.ToolRequirement, keys []publish.APIKeyRequirement, auth []publish.AuthRequirement) Requires {
	var r Requires
	by := []string{sidecar}
	for _, t := range tools {
		r.Tools = append(r.Tools, ToolNeed{
			Name: t.Name, Required: t.Required, Version: t.Version, Commands: t.Commands,
			InstallSkill: t.InstallSkill, Platforms: recipePlatforms(t.Recipes), Recipes: t.Recipes,
			NeededBy: by,
		})
	}
	for _, k := range keys {
		r.APIKeys = append(r.APIKeys, KeyNeed{Name: k.EnvName, Label: k.Label, Required: k.Required, Description: k.Description, NeededBy: by})
	}
	for _, a := range auth {
		r.Auth = append(r.Auth, LoginNeed{Name: a.Name, Label: a.Label, Description: a.Description, LoginHint: a.LoginHint, NeededBy: by})
	}
	return r
}

// recipePlatforms are the platforms a set of recipes covers, in first-seen order.
func recipePlatforms(recipes []publish.InstallRecipe) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range recipes {
		if r.Platform != "" && !seen[r.Platform] {
			seen[r.Platform] = true
			out = append(out, r.Platform)
		}
	}
	return out
}
