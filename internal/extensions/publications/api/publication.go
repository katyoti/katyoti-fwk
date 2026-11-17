// SPDX-License-Identifier: Apache-2.0

package api

// THE PUBLICATION OBJECT (under): one
// version-neutral object type, `publication`, for everything published - a
// management view of planification (a stream, a change log, a release board or
// plan) or any document-tree source object, such as an as-is study of any
// version, named by its id. It names its target - the publication connector
// it publishes through (Confluence, …) and that instance's settings - and
// keeps the outcome of its last publish. No owner of a source declares a
// publication type of its own: a study's publications are this type, children
// of the study.

import (
	"fmt"
	"strings"
)

const (
	// PublicationType is the object type of a publication.
	PublicationType = "publication"
	// CapPublication is its field-group.
	CapPublication = "publication"
)

// PublicationFields is the `publication` field-group (cap-publication): what
// it publishes - a management view (View, Scope, Lang) or a document-tree
// source object (Object) - and its target - a publication connector
// (Connector) or, for a management view, a converter's action URI (Converter)
// - with the instance's settings, and the last outcome.
type PublicationFields struct {
	Object    string         `json:"object,omitempty" jsonschema:"description=The document-tree source it publishes (a study of any version) by its id; empty for a management view"`
	View      string         `json:"view,omitempty" jsonschema:"description=The management view it publishes: stream or changelog or release-board or release-plan; empty for an object"`
	Scope     string         `json:"scope,omitempty" jsonschema:"description=The view's scope: the stream's name (stream) or the module (changelog) or the release (release-board) (release-plan)"`
	Lang      string         `json:"lang,omitempty" jsonschema:"description=The labels' language: en or fr"`
	Connector string         `json:"connector,omitempty" jsonschema:"description=The publication connector it publishes through (confluence ...) - its settings are that connector's"`
	Converter string         `json:"converter,omitempty" jsonschema:"description=A management view's converter: the action URI that lays its documents out at the target"`
	Settings  map[string]any `json:"settings,omitempty" jsonschema:"description=The target's settings (a Confluence connector: its space and parent page)"`
	OnRelease bool           `json:"on_release,omitempty" jsonschema:"description=Published again whenever a release is released"`
	Last      *Published     `json:"last,omitempty" jsonschema:"description=The last publication's outcome"`
}

// Published is the outcome of a publication.
type Published struct {
	At        string `json:"at" jsonschema:"description=RFC 3339"`
	Documents int    `json:"documents" jsonschema:"description=Pages sent to the target"`
	Result    string `json:"result,omitempty" jsonschema:"description=What the target answered"`
	Error     string `json:"error,omitempty" jsonschema:"description=Why it failed"`
}

// Check refuses a publication that does not say what it publishes - an
// object or a view, never both - or where: a connector or a converter, never
// both. Whether the view or the connector exists is its publisher's to say.
func (f PublicationFields) Check() error {
	object, view := strings.TrimSpace(f.Object), strings.TrimSpace(f.View)
	switch {
	case object == "" && view == "":
		return fmt.Errorf("a publication names what it publishes: an object or a view")
	case object != "" && view != "":
		return fmt.Errorf("a publication publishes an object or a view, not both (%s, %s)", object, view)
	}
	connector, converter := strings.TrimSpace(f.Connector), strings.TrimSpace(f.Converter)
	switch {
	case connector == "" && converter == "":
		return fmt.Errorf("a publication names its target: a publication connector or a converter")
	case connector != "" && converter != "":
		return fmt.Errorf("a publication publishes through a connector or a converter, not both (%s, %s)", connector, converter)
	}
	return nil
}

// Destination is where a connector publication goes - its connector, and the
// space and parent page its settings hold - the target a publish that writes
// passes apart from the settings, so the run's receipt names it
// ; ok is false while the settings
// name no space or no parent page: nothing may be written then.
func (f PublicationFields) Destination() (ReceiptTarget, bool) {
	str := func(k string) string { v, _ := f.Settings[k].(string); return strings.TrimSpace(v) }
	t := ReceiptTarget{Connection: strings.TrimSpace(f.Connector), Space: str("space"), Parent: str("parent")}
	return t, t.Connection != "" && t.Space != "" && t.Parent != ""
}
