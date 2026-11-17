// SPDX-License-Identifier: Apache-2.0

package api

// HOLDING A CHOICE: the items proposed or picked for a
// carrier are kept without being read - a wizard reads nothing before the
// person decides - until the carrier applies them all.

// HoldInput keeps a connector's chosen items for a carrier.
type HoldInput struct {
	ID string `json:"id"        cli:"id"        jsonschema:"The carrier's id - a module carrying the knowledge capability; required"`
	// Object names the carrier too - what a picker passes.
	Object    string         `json:"object,omitempty"          jsonschema:"The carrier's id - when id is empty (a picker's object)"`
	Connector string         `json:"connector" cli:"connector" jsonschema:"The connector's name; required"`
	Items     []SelectedItem `json:"items"                     jsonschema:"The items chosen - the whole choice: an item left out is no longer held"`
	// By is who holds them: the items it newly keeps
	// record them; an item already kept keeps its keeper.
	By *Keeper `json:"by,omitempty" jsonschema:"Who holds them - a person or an agent"`
	// Read reads what is kept at once: the source takes
	// the choice as its selection and is synced, the knowledge base
	// reindexed - a picker's « Enregistrer et lire ».
	Read bool `json:"read,omitempty" jsonschema:"Read what is kept at once: the source applies the choice and is synced and the knowledge base reindexed"`
}

// HoldOutput says what is held now, in the {ok, message} shape a pane shows.
type HoldOutput struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Source  string `json:"source" jsonschema:"The carrier's source holding the choice"`
	Held    int    `json:"held"`
	// Result says what reading the kept items read and failed, when read
	//
	Result *ApplyResult `json:"result,omitempty"`
}

// ApplyInput names the carrier whose held choices are applied.
type ApplyInput struct {
	ID string `json:"id" cli:"id" jsonschema:"The carrier's id; required"`
}

// ApplyOutput says which sources took their held choice.
type ApplyOutput struct {
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Sources []string `json:"sources" jsonschema:"The sources whose selection is now what they held"`
	// Results says what each source applied read, removed or failed
	//
	Results []ApplyResult `json:"results,omitempty"`
}

// ApplyResult is one source applied: what it read, removed and failed.
type ApplyResult struct {
	Source  string   `json:"source"`
	Read    []string `json:"read,omitempty"`
	Removed []string `json:"removed,omitempty"`
	Failed  []string `json:"failed,omitempty"`
	// Why says why the failed items were not read.
	Why     string `json:"why,omitempty"`
	Message string `json:"message,omitempty"`
}
