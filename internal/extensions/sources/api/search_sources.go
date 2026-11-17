// SPDX-License-Identifier: Apache-2.0

package api

// THE MULTI-SOURCE SEARCH: every source in use searches at
// once, off any page's load - a form or a page starts it, never draws it -,
// each answering as it comes with its state. A connector reading from
// another source (from) waits for that source's answer: it follows what it
// proposed, beside what the carrier kept. With a carrier, what it already
// holds is never proposed again (« Chercher encore »); with none (a creation
// form) nothing is held and nothing is created. Only re-runs
// one source alone, the others' answers kept by whoever shows them
// (« Réessayer »).

// The states of a source in a multi-source search.
const (
	// SearchSearching: its search is running.
	SearchSearching = "searching"
	// SearchWaiting: it reads from another source and waits for its answer
	// (Waiting names it).
	SearchWaiting = "waiting"
	// SearchFound: it answered items (Found of them).
	SearchFound = "found"
	// SearchNothingNew: it answered nothing the carrier does not hold yet.
	SearchNothingNew = "nothing-new"
	// SearchUnreachable: it cannot be reached; Message says why.
	SearchUnreachable = "unreachable"
	// SearchNotConfigured: its settings or keys are not set; Configure says
	// where.
	SearchNotConfigured = "not-configured"
)

// SearchSourcesURI is the method a multi-source search panel starts.
const SearchSourcesURI = "ktw-agents:sources/search-sources"

// SearchSourcesNode is the id of the node a lent panel's per-source answers
// are laid over: its data's sources, each emitted as it comes.
const SearchSourcesNode = "source-search"

// SearchSourcesInput starts a multi-source search.
type SearchSourcesInput struct {
	Object     string   `json:"object,omitempty"     cli:"object"     jsonschema:"The carrier - its own source of each connector gives where it searches and what it already holds; empty: a search before its object exists - nothing held and nothing created"`
	Connectors []string `json:"connectors,omitempty" cli:"connectors" jsonschema:"The sources searched - by connector; empty: every connector that picks"`
	Terms      []string `json:"terms,omitempty"      cli:"terms"      jsonschema:"Terms of the context"`
	Text       string   `json:"text,omitempty"       cli:"text"       jsonschema:"The context in free text"`
	Only       string   `json:"only,omitempty"       cli:"only"       jsonschema:"One connector searched again alone (« Réessayer ») - the others' answers are kept by whoever shows them; the source it reads from is searched again quietly"`
}

// SearchSourcesOutput is every source's answer, in the order they were
// asked.
type SearchSourcesOutput struct {
	Sources []SourceSearch `json:"sources"`
}

// SourceSearch is where one source of a multi-source search stands - what
// is emitted each time it moves.
type SourceSearch struct {
	Connector string `json:"connector"`
	Label     string `json:"label,omitempty"`
	Icon      string `json:"icon,omitempty"`
	Color     string `json:"color,omitempty"`
	State     string `json:"state"             jsonschema:"searching or waiting or found or nothing-new or unreachable or not-configured"`
	// Waiting is the source it waits on: its connector and label.
	Waiting      string `json:"waiting,omitempty"       jsonschema:"The connector of the source it waits on"`
	WaitingLabel string `json:"waiting_label,omitempty" jsonschema:"That source's label"`
	Found        int    `json:"found"                   jsonschema:"How many items it proposes"`
	// Held is how many of its items the carrier already holds: left out.
	Held      int                 `json:"held,omitempty"      jsonschema:"How many items it found that the carrier already holds - not proposed again"`
	Items     []ProposedItem      `json:"items"`
	Message   string              `json:"message,omitempty"   jsonschema:"Why it is unreachable or not configured"`
	Errors    []string            `json:"errors,omitempty"    jsonschema:"The searches of it that failed - the items the others found are kept"`
	Configure *ConnectorConfigure `json:"configure,omitempty" jsonschema:"Where it is configured - when it is not"`
	// Kinds is how its items are drawn: the connector's declared kinds,
	// laid over the panel's own.
	Kinds map[string]ItemKind `json:"kinds,omitempty" jsonschema:"How its item kinds are drawn - each kind's label and slots"`
}

// Done says whether a source has answered: any state but searching and
// waiting.
func (s SourceSearch) Done() bool {
	return s.State != SearchSearching && s.State != SearchWaiting
}
