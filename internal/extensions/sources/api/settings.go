// SPDX-License-Identifier: Apache-2.0

package api

// AN OPENED SOURCE'S « RÉGLAGES »: one form read in rows, a
// pencil per value edited where it is shown (mock-up v6 vSourceSettings) -
// what it reads, the holder's keywords it searches by, its own keywords, its
// proposal bound, what it takes, the source it reads from, its scopes, when
// it is read again, its connection - each change kept at once by
// SettingsSave; « Tester » records its connection by SettingsTest.

// ProposeMaxLimit is the largest proposal bound a source may set.
const ProposeMaxLimit = 100

// ProposeBound is the proposal bound n asks for: ProposeMax when n is 0 or
// out of 1..ProposeMaxLimit.
func ProposeBound(n int) int {
	if n < 1 || n > ProposeMaxLimit {
		return ProposeMax
	}
	return n
}

// SourceSettings is the value an opened source's « Réglages » form binds:
// the rows it reads only - what it reads, the holder's keywords, what it
// takes, the source it reads from, its connection - and those it edits -
// its keywords, its proposal bound, its scopes, the connection it reads
// through, its re-reading mode.
type SourceSettings struct {
	Reads      string   `json:"reads,omitempty"       jsonschema:"title=Reads,description=What it reads - its connector's label"`
	SearchesBy []string `json:"searches_by,omitempty" jsonschema:"title=Searches by,description=Its holder's keywords"`
	Keywords   []string `json:"keywords,omitempty"    jsonschema:"title=Keywords,description=Its own keywords beside its holder's"`
	ProposeMax int      `json:"propose_max,omitempty" jsonschema:"title=Proposals,description=How many items a proposal answers at most"`
	Takes      string   `json:"takes,omitempty"       jsonschema:"title=Takes,description=What it takes - its connector's description"`
	ReadsFrom  string   `json:"reads_from,omitempty"  jsonschema:"title=Reads from,description=The other source of its holder it reads from"`
	Scopes     []string `json:"scopes,omitempty"      jsonschema:"title=Scopes,description=The scopes it reads in"`
	Through    string   `json:"through,omitempty"     jsonschema:"title=Through,description=The connection it reads through - empty for the universe's configuration"`
	// Location is the source's location, its fields the connector's
	// location data-type lets « Réglages » edit drawn as rows of their own
	// (location.<field>) - typed by that data-type on the form.
	Location map[string]any `json:"location,omitempty" jsonschema:"title=Location,description=Where it reads - the fields its connector lets the settings edit"`
	// IDColumns are the « Colonne identifiant » rows, one per item kind its
	// connector types (id_columns.<kind>): an attribute, IDColumnNone or
	// IDColumnUID; a kind with no choice reads IDColumnUID.
	IDColumns  map[string]string `json:"id_columns,omitempty" jsonschema:"title=Id columns,description=By item kind the attribute its read table leads with - none for no id column"`
	Sync       string            `json:"sync,omitempty"        jsonschema:"title=Sync,description=When it is read again,enum=manual|on-open|daily|daily-on-open"`
	Connection *SourceConnection `json:"connection,omitempty" jsonschema:"title=Connection,description=Whether its connection answered when last tested and when"`
}

// SourceConnection is a source's connection as last tested, as « Réglages »
// reads it (mock-up v6): its state in words - drawn as a pill - and when it
// was tested - said from now beside it.
type SourceConnection struct {
	State    string `json:"state"                jsonschema:"title=State,description=Its state in words - reachable or not or not tested yet"`
	TestedAt string `json:"tested_at,omitempty"  jsonschema:"title=Tested at,description=When it was last tested - RFC 3339"`
}

// SettingsSaveInput is an opened source's « Réglages » as its form submits
// them.
type SettingsSaveInput struct {
	ID       string         `json:"id"       cli:"id" jsonschema:"The information source's id; required"`
	Settings SourceSettings `json:"settings"          jsonschema:"The settings as the form holds them - the rows it only reads are ignored"`
}

// SettingsSaveOutput says whether the settings were kept.
type SettingsSaveOutput struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// SettingsTestInput names the source whose connection « Tester » checks; a
// row's button sends its value beside it.
type SettingsTestInput struct {
	ID    string `json:"id"              cli:"id" jsonschema:"The information source's id; required"`
	Value any    `json:"value,omitempty" jsonschema:"The row's value - ignored"`
}

// SettingsTestOutput is the test's outcome in the check grammar a read row
// draws: ok and its text, or ok false and its message.
type SettingsTestOutput struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Text    string `json:"text,omitempty"`
}

// The methods an opened source's « Réglages » form calls.
const (
	SettingsSaveURI = "ktw-agents:sources/settings-save"
	SettingsTestURI = "ktw-agents:sources/settings-test"
	// RemoveURI removes a source - « Supprimer la source »: what it brought
	// leaves the knowledge base.
	RemoveURI = "ktw-agents:sources/remove"
)
