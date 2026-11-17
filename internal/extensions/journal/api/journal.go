// SPDX-License-Identifier: Apache-2.0

// Package api is the journal service contract: what happened to every object —
// created, updated, moved, deleted, transitioned, an action run on it — kept as an append-only
// stream in the system module `sys-journal`, one `journal-day` object per day
// whose `entries` named file holds one JSON record per line. History is a
// query over it, never a copy. The service keeps no storage of its own: it
// writes and reads through the CWM Object API.
package api

import (
	"context"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The journal's module, type and file.
const (
	Module      = cwmapi.SystemModulePrefix + "journal"
	DayType     = "journal-day"
	EntriesFile = "entries"
)

// Actions a record carries.
const (
	ActionCreated      = "created"
	ActionUpdated      = "updated"
	ActionDeleted      = "deleted"
	ActionMoved        = "moved"
	ActionSplit        = "split"
	ActionArchived     = "archived" // the object ended (cwm ObjectArchive): Note is where its file went
	ActionTransitioned = "transitioned"
	ActionInvoked      = "invoked" // an action ran on the object (the actions service reports it)
	ActionEvent        = "event"   // an event raised on the object (cwm EventRaise): Event names its type
)

// DayID is the journal-day object of a date (UTC): `sys-journal.2026-09-27`.
func DayID(t time.Time) string {
	return cwmapi.QualifyObjectID(Module, t.UTC().Format("2006-01-02"))
}

// Record is one line of the journal: one thing that happened to one object.
type Record struct {
	// At is the write's time (the event's), UTC.
	At     time.Time `json:"at"`
	Action string    `json:"action" jsonschema:"created | updated | deleted | moved | split | archived | transitioned | invoked | event"`
	ID     string    `json:"id"`
	Module string    `json:"module"`
	Type   string    `json:"type,omitempty"`
	Parent string    `json:"parent,omitempty"`
	// EventID is the id of the event the record comes from (a UUID v7); an
	// invoked record has none.
	EventID string `json:"event_id,omitempty"`
	// Revision is the object's revision the write left (a delete: the one
	// removed).
	Revision string `json:"revision,omitempty"`
	// Cause is why it happened — the person, agent, hook or action the write
	// was made under, and how many reactions deep; absent when the write
	// carried none.
	Cause *cause.Cause `json:"cause,omitempty"`
	// A move: where the object came from, and why.
	FromID     string `json:"from_id,omitempty"`
	FromModule string `json:"from_module,omitempty"`
	FromParent string `json:"from_parent,omitempty"`
	Reason     string `json:"reason,omitempty"`
	// A split: what replaced the object (ID), and whether it was kept.
	Successors []string `json:"successors,omitempty"`
	SourceKept *bool    `json:"source_kept,omitempty"`
	// A transition: the states, the transition, who made it — and which
	// lifecycle moved (Capability: empty = the object's own), whether a
	// failing guard was forced, and what the guard's functions answered then.
	// These records ARE the object's lifecycle history (Transitions).
	FromState  string                  `json:"from_state,omitempty"`
	ToState    string                  `json:"to_state,omitempty"`
	Transition string                  `json:"transition,omitempty"`
	Capability string                  `json:"capability,omitempty"`
	Forced     bool                    `json:"forced,omitempty"`
	Calls      []cwmapi.FunctionAnswer `json:"calls,omitempty"`
	By         *cwmapi.Actor           `json:"by,omitempty"`
	Note       string                  `json:"note,omitempty"`
	// An event raised on it: the event type (code.changed); the note is its
	// payload, cut.
	Event string `json:"event,omitempty"`
	// An action run on it: its URI and how it ended (ok, launched, failed);
	// the note says what came of it.
	URI    string `json:"uri,omitempty"`
	Status string `json:"status,omitempty"`
}

// JournalServiceDef describes the service for KTSP/CLI auto-generation.
var JournalServiceDef = meta.ServiceDef{
	Name:    "journal",
	Short:   "What happened to every object: created, updated, moved, deleted, transitioned, an action run on it, an event raised on it",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Query": {Short: "Records newest first, by object, module, time window and action",
			RenderTemplate: recordsTemplate},
		"History": {Short: "Everything that happened to one object, newest first — its former ids and the object it was split from included",
			RenderTemplate: recordsTemplate},
		"Transitions": {Short: "One object's lifecycle moves, oldest first: the journal's transitioned records, and before the journal's oldest day what git's log of the object's file shows",
			RenderTemplate: transitionsTemplate},
		"Times": {Short: "When one object was created and last changed, derived: its oldest created record and its newest record in the journal (followed through its moves and splits), else git's log of its file",
			RenderTemplate: timesTemplate},
		"Invoked":     meta.Method("Record that an action ran on an object (its URI, how it ended, what came of it) - the actions service reports each run"),
		"HistoryPage": {Short: "The History panel of an object: everything that happened to it, newest first - each change with who made it, when and why", ReadOnly: true},
	},
}

// recordsTemplate renders Query's and History's records in the CLI's rich and
// raw modes: one line per record — when, what, to which object, the detail
// the action carries, and why (`kind:id@depth`).
const recordsTemplate = `{{ if .records -}}
{{ range .records -}}
{{ .at }}  {{ .action }}  {{ .id }}{{ if .from_id }}  from {{ .from_id }}{{ end }}{{ if .from_state }}  {{ .from_state }} -> {{ .to_state }}{{ end }}{{ if .successors }}  -> {{ join .successors ", " }}{{ end }}{{ if .uri }}  {{ .uri }} {{ .status }}{{ end }}{{ if .event }}  {{ .event }}{{ end }}{{ with .cause }}  {{ dim (printf "%s:%s@%.0f" .kind .id .depth) }}{{ end }}
{{ end -}}
{{ if .truncated }}{{ dim "(more: raise the limit)" }}
{{ end -}}
{{- else -}}
{{ dim "No record." }}
{{ end -}}`

// transitionsTemplate renders Transitions: one line per move, oldest first.
const transitionsTemplate = `{{ if .entries -}}
{{ range .entries -}}
{{ .at }}  {{ if .from }}{{ .from }} -> {{ end }}{{ .to }}{{ if .transition }}  {{ .transition }}{{ end }}{{ with .by }}  {{ .id }}{{ end }}{{ if .forced }}  {{ dim "forced" }}{{ end }}{{ if .note }}  {{ dim .note }}{{ end }}
{{ end -}}
{{ if .truncated }}{{ dim "(more: raise the limit)" }}
{{ end -}}
{{- else -}}
{{ dim "No move." }}
{{ end -}}`

// timesTemplate renders Times: created and changed, each when known.
const timesTemplate = `{{ if .created_at }}created  {{ .created_at }}
{{ end -}}
{{ if .updated_at }}changed  {{ .updated_at }}
{{ end -}}
{{ if not (or .created_at .updated_at) }}{{ dim "Not known." }}
{{ end -}}`

// JournalService answers what happened. It is written by the CWM event
// observer, in the process that holds the workspace — and, for the actions run
// on an object, by the actions service (Invoked): what CWM's events cannot see.
type JournalService interface {
	Query(ctx context.Context, in QueryInput) (QueryOutput, error)
	History(ctx context.Context, in HistoryInput) (QueryOutput, error)
	// Transitions is one object's lifecycle history, derived: the object
	// stores only its state.
	Transitions(ctx context.Context, in TransitionsInput) (TransitionsOutput, error)
	// Times is when one object was created and last changed, derived: no
	// object stores its times.
	Times(ctx context.Context, in TimesInput) (TimesOutput, error)
	Invoked(ctx context.Context, in InvokedInput) (InvokedOutput, error)
	// HistoryPage is History drawn: the History panel of every object.
	HistoryPage(ctx context.Context, in HistoryPageInput) (render.Def, error)
}

// HistoryPageInput names the object the panel shows, and the person's
// language.
type HistoryPageInput struct {
	ID       string `json:"id"                 jsonschema:"The object; required"`
	Language string `json:"language,omitempty" jsonschema:"The person's language (en or fr); empty = English"`
}

// TimesInput names the object.
type TimesInput struct {
	ID string `json:"id" jsonschema:"The object; required"`
}

// TimesOutput is when the object was created and last changed, UTC; a zero
// time is not known (no record and no git history). CreatedAt is the
// journal's created record of the object (followed through its moves and
// splits), else its file's first commit; UpdatedAt is the journal's newest
// record of it (any action), else its file's last commit.
type TimesOutput struct {
	CreatedAt time.Time `json:"created_at,omitzero" jsonschema:"When the object was created; absent when not known"`
	UpdatedAt time.Time `json:"updated_at,omitzero" jsonschema:"When the object last changed; absent when not known"`
}

// InvokedInput is an action run on an object.
type InvokedInput struct {
	ID     string        `json:"id"               jsonschema:"The object the action ran on; required"`
	URI    string        `json:"uri"              jsonschema:"The action; required"`
	Status string        `json:"status"           jsonschema:"ok or launched or failed; required"`
	Note   string        `json:"note,omitempty"   jsonschema:"What came of it: the task started or the error (200 characters at most)"`
	By     *cwmapi.Actor `json:"by,omitempty"     jsonschema:"Who ran it"`
}

// InvokedOutput is the record written.
type InvokedOutput struct {
	Record Record `json:"record"`
}

// QueryInput filters the journal; every field narrows, none is required.
type QueryInput struct {
	ObjectID string    `json:"object_id,omitempty" jsonschema:"Records about this object (as its id or as the id it moved from)"`
	Module   string    `json:"module,omitempty"    jsonschema:"Records about objects of this module"`
	Since    time.Time `json:"since,omitempty"     jsonschema:"From this time (inclusive); zero = the oldest day kept"`
	Until    time.Time `json:"until,omitempty"     jsonschema:"Up to this time (exclusive); zero = now"`
	Actions  []string  `json:"actions,omitempty"   jsonschema:"Only these actions"`
	Limit    int       `json:"limit,omitempty"     jsonschema:"At most this many records; 0 = 200"`
}

// HistoryInput names the object.
type HistoryInput struct {
	ID    string `json:"id"              jsonschema:"The object; required"`
	Limit int    `json:"limit,omitempty" jsonschema:"At most this many records; 0 = 200"`
}

// TransitionsInput names the object and the lifecycle.
type TransitionsInput struct {
	ID         string `json:"id"                   jsonschema:"The object; required"`
	Capability string `json:"capability,omitempty" jsonschema:"The capability whose own workflow moved; empty = the object's lifecycle"`
	Limit      int    `json:"limit,omitempty"      jsonschema:"At most this many moves - the latest; 0 = 200"`
}

// TransitionsOutput is the moves, oldest first, in the shape a lifecycle
// history reads (from, to, transition, by, at, note, forced, calls).
// Truncated says the limit cut the oldest.
type TransitionsOutput struct {
	Entries   []cwmapi.LifecycleEntry `json:"entries"`
	Truncated bool                    `json:"truncated,omitempty"`
}

// QueryOutput is the records, newest first. Truncated says the limit cut it.
type QueryOutput struct {
	Records   []Record `json:"records"`
	Truncated bool     `json:"truncated,omitempty"`
}

// DefaultLimit is a query's limit when none is given.
const DefaultLimit = 200

// DefaultRetentionDays is how many days stay in the tree when Journal.yaml
// says nothing; older days are pruned (git keeps them).
const DefaultRetentionDays = 90
