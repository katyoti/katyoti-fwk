// SPDX-License-Identifier: Apache-2.0

package api

// « + NOUVEAU › PUBLICATION » (mock-up v6 view 29): a
// publication is created under the object it publishes - a document-tree
// source such as a study - through its type's create page: where it publishes
// (one contributed publication connector, chosen) and its name; it then opens,
// its destination set in its own tabs.

import "github.com/katyoti/katyoti-fwk/internal/core/render"

// NewPublicationPageURI is the publication type's create_task: the page the
// create dialog draws under the holder.
const NewPublicationPageURI = "ktw-agents:publishers/new-publication-page"

// PublicationNewURI is the create page's « Créer ».
const PublicationNewURI = "ktw-agents:publishers/publication-new"

// NewPublicationPageInput asks a publication's create page under its holder.
type NewPublicationPageInput struct {
	Parent string `json:"parent" jsonschema:"description=The object the publication publishes and is created under - a document-tree source such as a study; required"`
}

// PublicationPageOutput is a page drawn by the host: its tree and its data.
type PublicationPageOutput struct {
	DSLTree map[string]any `json:"dslTree"`
	Data    map[string]any `json:"data,omitempty"`
	Title   string         `json:"title,omitempty"`
}

// PublicationNewInput creates a publication of Parent through Connector.
type PublicationNewInput struct {
	Parent    string `json:"parent"         jsonschema:"description=The object it publishes and is created under; required"`
	Connector string `json:"connector"      jsonschema:"description=The publication connector it publishes through; required"`
	Name      string `json:"name,omitempty" jsonschema:"description=Its name - the connector's label when empty"`
}

// PublicationNewOutput follows the action result convention; Open opens the
// publication created.
type PublicationNewOutput struct {
	OK          bool             `json:"ok"`
	Message     string           `json:"message"`
	Publication string           `json:"publication,omitempty" jsonschema:"description=The publication created"`
	Open        *render.Navigate `json:"open,omitempty"        jsonschema:"description=The object to open next - the publication created"`
}
