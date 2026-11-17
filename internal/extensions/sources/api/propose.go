// SPDX-License-Identifier: Apache-2.0

package api

// PROPOSING A SOURCE'S ITEMS: a connector's optional propose
// method takes a context - terms and free text - and answers items, each
// with why and a score. Propose runs it for a carrier; ProposalWizard answers
// the page of a wizard showing one step per chosen source, its picker
// pre-ticked with that proposal, where the person adds, removes or presses
// « Suivant ».

// ProposeInput asks one connector's proposal for a carrier.
type ProposeInput struct {
	Connector string   `json:"connector"        cli:"connector" jsonschema:"The connector's name; required"`
	Object    string   `json:"object,omitempty" cli:"object"    jsonschema:"The carrier - its own source for the connector gives the location"`
	Terms     []string `json:"terms,omitempty"  cli:"terms"     jsonschema:"Terms of the context"`
	Text      string   `json:"text,omitempty"   cli:"text"      jsonschema:"The context in free text"`
}

// ProposalWizardInput asks the wizard proposing items from every chosen
// source.
type ProposalWizardInput struct {
	Object     string   `json:"object,omitempty"     cli:"object"     jsonschema:"The carrier the sources are for"`
	Connectors []string `json:"connectors,omitempty" cli:"connectors" jsonschema:"The chosen connectors, in their step order; empty: every connector that can propose"`
	Terms      []string `json:"terms,omitempty"      cli:"terms"      jsonschema:"Terms of the context"`
	Text       string   `json:"text,omitempty"       cli:"text"       jsonschema:"The context in free text"`
}

// ProposalPageInput asks the proposal wizard as a page region:
// the terms as the page holds them - an edited list -, its finish applying
// what the carrier holds.
type ProposalPageInput struct {
	Object        string   `json:"object"                   cli:"object"         jsonschema:"The carrier the sources are for; required"`
	Connectors    []string `json:"connectors,omitempty"     cli:"connectors"     jsonschema:"The chosen connectors, in their step order; empty: every connector that can propose"`
	Terms         []string `json:"terms,omitempty"          cli:"terms"          jsonschema:"Terms of the context"`
	Text          string   `json:"text,omitempty"           cli:"text"           jsonschema:"The context in free text"`
	CompleteLabel string   `json:"complete_label,omitempty" cli:"complete-label" jsonschema:"The finishing button's words; « Appliquer » by default"`
	// Cancel is what leaving the wizard runs: a page
	// action - its label, uri, args and confirm - a consumer's « Abandonner ».
	Cancel map[string]any `json:"cancel,omitempty" jsonschema:"What leaving the wizard runs: a page action (label and uri and args and confirm)"`
	// Complete is what finishing the wizard runs instead of Apply
	// : a page action - a consumer's own finish, such as a
	// run whose first step applies what is held.
	Complete map[string]any `json:"complete,omitempty" jsonschema:"What finishing the wizard runs instead of Apply: a page action (label and uri and args and confirm)"`
}

// HoldURI is the method a proposal step's picker applies: what is ticked is
// held, never read until Apply.
const HoldURI = "ktw-agents:sources/hold"

// ApplyURI is the method a proposal page's finish runs on its carrier.
const ApplyURI = "ktw-agents:sources/apply"

// ProposalWizardOutput is the wizard's page (a page DSL tree): one step per
// chosen source, each a picker pre-ticked with its proposal; a source whose
// proposal failed shows its picker empty, saying why.
type ProposalWizardOutput struct {
	Page   map[string]any `json:"page"`
	Errors []string       `json:"errors,omitempty"`
}
