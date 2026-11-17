// SPDX-License-Identifier: Apache-2.0

package api

// ReviewableFields are the `reviewable` capability's field-group on the
// object reviewed: the compact history of its reviews. A module keeps its
// current review and the one before it of each review type as objects; an
// older review is folded into one entry here, then removed.
type ReviewableFields struct {
	History []HistoryEntry `json:"history,omitempty" jsonschema:"title=History,description=The reviews folded away: one entry each with its type and day and rating and finding counts; oldest first"`
}

// HistoryEntry is one folded review: what the trend and the timeline read.
type HistoryEntry struct {
	Review     string `json:"review"                jsonschema:"description=The id the review had"`
	Type       string `json:"type,omitempty"        jsonschema:"description=Its review type"`
	ReviewedAt string `json:"reviewed_at,omitempty" jsonschema:"description=When it was held"`
	Commit     string `json:"commit,omitempty"      jsonschema:"description=The commit it read"`
	Rating     string `json:"rating,omitempty"      jsonschema:"description=Its rating,enum=good|needs-attention|critical"`
	Score      *int   `json:"score,omitempty"       jsonschema:"description=Its score when its type gives one"`
	Findings   int    `json:"findings,omitempty"    jsonschema:"description=How many findings it listed"`
	// Open counts the findings still open when it was folded, by severity.
	Open map[string]int `json:"open,omitempty" jsonschema:"description=Its findings still open when it was folded by severity"`
}

// SupersedeInput names the review just filed.
type SupersedeInput struct {
	ID string `json:"id" jsonschema:"description=The review just filed; required"`
}

// SupersedeOutput says what is kept and what was folded, and names every
// object the fold changed and their files: a caller that commits by path
// commits the fold whole, never a removed review without the links it cut.
type SupersedeOutput struct {
	Current  string   `json:"current"            jsonschema:"description=The current review of the type"`
	Previous string   `json:"previous,omitempty" jsonschema:"description=The review before it; empty when it is the first"`
	Folded   []string `json:"folded,omitempty"   jsonschema:"description=The reviews folded into the history and removed"`
	Removed  []string `json:"removed,omitempty"  jsonschema:"description=Every object removed: the folded reviews and their children"`
	Detached []string `json:"detached,omitempty" jsonschema:"description=The objects whose links to a removed object were cut: findings that pointed at a folded review"`
	History  string   `json:"history,omitempty"  jsonschema:"description=The reviewed object whose history received the folded reviews"`
	Files    []string `json:"files,omitempty"    jsonschema:"description=The project-relative files the fold wrote or removed; sorted"`
}
