// SPDX-License-Identifier: Apache-2.0

// Package api is the `project-contributions` slot: the source of the
// Contributions panel of the system module that operates the project's
// contributions (sys-contributions).
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// ServiceDef describes the `project-contributions` slot.
var ServiceDef = meta.ServiceDef{
	Name:    "project-contributions",
	Short:   "The project's own contributions: what the universe publishes under the project's provider token, by kind",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Panel": {Short: "The Contributions panel: the entries the universe publishes under the project's provider token, grouped by kind", ReadOnly: true},
	},
}

// ProjectContributionsService answers the Contributions panel (named for itself: services are keyed by type name).
type ProjectContributionsService interface {
	Panel(ctx context.Context, in ObjectInput) (PanelOutput, error)
}

// ModuleID is the system module that holds and shows the project's
// contributions.
const ModuleID = "sys-contributions"

// ObjectInput names one object: what a panel's source is called with.
type ObjectInput struct {
	ID       string `json:"id" jsonschema:"description=The object; required"`
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}

// PanelOutput is the project's contributions as the universe serves them.
type PanelOutput struct {
	Provider string  `json:"provider" jsonschema:"description=The project's provider token"`
	Folder   string  `json:"folder" jsonschema:"description=The contributions folder; relative to the project root"`
	Groups   []Group `json:"groups" jsonschema:"description=One per kind; by kind name"`
	// Overrides are the members' entries the project overrides (its
	// contributions folder's overwrites/<member>/), each with the layers it
	// serves of them.
	Overrides []Overridden `json:"overrides" jsonschema:"description=The members' entries the project overrides; by URI"`
}

// Overridden is one member entry the project overrides, and which of its
// layers.
type Overridden struct {
	URI    string   `json:"uri"`
	Layers []string `json:"layers"`
}

// Group is the entries of one kind.
type Group struct {
	Kind    string  `json:"kind"`
	Entries []Entry `json:"entries" jsonschema:"description=By id"`
}

// Entry is one contribution: its URI opens it.
type Entry struct {
	URI         string `json:"uri"`
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}
