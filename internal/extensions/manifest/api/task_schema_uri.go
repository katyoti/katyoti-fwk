// SPDX-License-Identifier: Apache-2.0

package api

import contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

// SchemaRole names which side of a task's contract a published schema
// describes.
type SchemaRole string

const (
	SchemaRoleInput  SchemaRole = "input"
	SchemaRoleOutput SchemaRole = "output"
)

// TaskSchemaID is the contribution ID a task's published schema takes.
//
// # Why this is a function
//
// This shape used to be a string concatenation in the producer and a DIFFERENT
// string concatenation in ktw-gui, which rebuilt the full URI to look the
// schema up. Two writers of one fact, in two packages, with nothing that would
// notice them disagreeing: the GUI asks for a URI nobody published, gets a 404,
// and renders no form — the exact failure c27 was opened on, with a different
// cause.
//
// So the shape has one writer. A producer names its contribution with this; a
// consumer addresses it with TaskSchemaURI. Neither spells it out.
func TaskSchemaID(taskName string, role SchemaRole) string {
	return ResourceTypeTask + "/" + taskName + "/" + string(role)
}

// TaskSchemaURI is the resolvable address of a task's published schema.
//
// Empty when the provider is unknown, because a resource URI without a provider
// addresses nothing — and returning a half-built one would send a consumer to
// look up a URI that cannot resolve, which is indistinguishable from a task
// that declared no schema.
func TaskSchemaURI(provider, taskName string, role SchemaRole) string {
	if provider == "" || taskName == "" {
		return ""
	}
	return provider + ":/" + contribapi.ResourceTypeDataType + "/" + TaskSchemaID(taskName, role)
}
