// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	publicationsapi "github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
)

// PUBLICATIONS — a management view published to a target. A `publication`
// object (in sys-planification) records the view and its scope, and the
// target: a converter's action URI and its settings. Publishing renders the
// view and runs the converter with its settings plus the documents; the
// converter, owned by the target (a Confluence connector for Confluence), knows how to
// lay the documents out there. A new format is a new converter, never a change
// here. The object type is the publications pack's version-neutral
// `publication`: planification publishes its views
// through it.

const (
	// PublicationType is the object type of a publication.
	PublicationType = publicationsapi.PublicationType
	// CapPublication is its field-group.
	CapPublication = publicationsapi.CapPublication
)

// The views a publication can publish.
const (
	ViewStream       = "stream"
	ViewChangeLog    = "changelog"
	ViewReleaseBoard = "release-board"
	ViewReleasePlan  = "release-plan"
)

// PublicationFields is the `publication` field-group (cap-publication), the
// publications pack's.
type PublicationFields = publicationsapi.PublicationFields

// Published is the outcome of a publication.
type Published = publicationsapi.Published

// Publication is a publication as the service reads it.
type Publication struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	PublicationFields
}

// PublishDocumentsKey is where a converter finds the documents in its input;
// the publication's settings sit beside it, flattened.
const PublishDocumentsKey = "documents"

// PublicationServiceDef describes the service for KTSP/CLI auto-generation.
var PublicationServiceDef = meta.ServiceDef{
	Name:    "publications",
	Short:   "Publish management views to a target: which view, which scope, which converter",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Create":  meta.Method("Record a publication: a view and its scope, the converter's action URI and its settings"),
		"Get":     meta.Method("Read one publication"),
		"List":    meta.Method("List the publications"),
		"Preview": meta.Method("Render a publication's documents without publishing them"),
		"Publish": meta.Method("Render the view and run the converter; the outcome is recorded on the publication"),
	},
}

// PublicationService manages publications.
type PublicationService interface {
	Create(ctx context.Context, in CreatePublicationInput) (PublicationOutput, error)
	Get(ctx context.Context, in GetInput) (PublicationOutput, error)
	List(ctx context.Context, in ListAllInput) (ListPublicationsOutput, error)
	Preview(ctx context.Context, in GetInput) (DocumentsOutput, error)
	Publish(ctx context.Context, in GetInput) (PublishOutput, error)
}

// CreatePublicationInput records a publication.
type CreatePublicationInput struct {
	Name      string         `json:"name"                 jsonschema:"Its name; required"`
	View      string         `json:"view"                 jsonschema:"stream or changelog or release-board or release-plan; required"`
	Scope     string         `json:"scope,omitempty"      jsonschema:"The stream's name or the module or the release"`
	Lang      string         `json:"lang,omitempty"       jsonschema:"en or fr"`
	Target    string         `json:"target"               jsonschema:"The converter's action URI; required"`
	Settings  map[string]any `json:"settings,omitempty"   jsonschema:"What the converter needs"`
	OnRelease bool           `json:"on_release,omitempty" cli:"on-release" jsonschema:"Publish again whenever a release is released"`
}

// PublicationOutput returns a publication.
type PublicationOutput struct {
	Publication Publication `json:"publication"`
	Path        string      `json:"path,omitempty"`
}

// ListPublicationsOutput lists them, sorted by id.
type ListPublicationsOutput struct {
	Publications []Publication `json:"publications"`
}

// PublishOutput is what publishing did: the publication, its outcome in `last`.
type PublishOutput struct {
	Publication Publication `json:"publication"`
}
