// SPDX-License-Identifier: Apache-2.0

package api

// RE-READING: an information source is read again by hand
// (manual), when its carrier's « Sources » tab opens (on-open) or every day
// (daily); an item read again with another content raises source.changed on
// its carriers.

// The re-reading modes of a source (its sync field).
const (
	SyncManual = "manual"
	SyncOnOpen = "on-open"
	SyncDaily  = "daily"
	// SyncDailyOnOpen is due in both modes: every day, and when its
	// carrier opens (« Chaque jour, et à l'ouverture »).
	SyncDailyOnOpen = "daily-on-open"
)

// SyncIn says whether a source of re-reading mode sync is due in mode -
// daily or on-open.
func SyncIn(sync, mode string) bool {
	return sync == mode || (sync == SyncDailyOnOpen && (mode == SyncDaily || mode == SyncOnOpen))
}

// EventSourceChanged is raised on a source's carriers when a sync reads
// items with another content than before; PointSourceChange is its point.
const (
	EventSourceChanged = "source.changed"
	PointSourceChange  = "on_source_change"
)

// DailyTimer is the timer a daily source keeps on its carrier (the timed
// capability): its timer.elapsed runs SyncDue daily.
const DailyTimer = "sources-daily"

// SourceChanged is a source.changed event's payload.
type SourceChanged struct {
	Source string   `json:"source"`
	Items  []string `json:"items" jsonschema:"The items read with another content - by title"`
}

// SyncDueInput names the carrier and the mode whose due sources are re-read.
type SyncDueInput struct {
	ID   string `json:"id"   cli:"id"   jsonschema:"The carrier's id; required"`
	Mode string `json:"mode" cli:"mode" jsonschema:"daily or on-open; required"`
}

// SyncDueOutput says which sources were re-read.
type SyncDueOutput struct {
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Synced  []string `json:"synced"`
}

// SYNC PROGRESS: a source's sync publishes where it stands as
// source.sync events raised on the source itself - started, items read n of
// m, then failed (with why) or ended - so a run reading several sources and
// the source's card follow each one. CWM publishes a raised event on the
// object's subject (cwmapi.SubjectToken of its id); a follower subscribes to
// it there.
const (
	EventSourceSync = "source.sync"
	PointSourceSync = "on_source_sync"
)

// The phases of a source.sync event. Every sync publishes started first and
// ends on exactly one of failed or ended.
const (
	SyncStarted = "started"
	SyncItems   = "items"
	SyncFailed  = "failed"
	SyncEnded   = "ended"
)

// SyncErrorMax bounds the why a failed event carries, in bytes: the payload a
// raised event publishes is cut at cwmapi.NoteMax, and a cut one is no JSON.
const SyncErrorMax = 80

// SourceSyncProgress is a source.sync event's payload.
type SourceSyncProgress struct {
	Source string `json:"source"          jsonschema:"The source synced (its id)"`
	Phase  string `json:"phase"           jsonschema:"started, items, failed or ended"`
	Read   int    `json:"read,omitempty"  jsonschema:"The items read and anchored so far"`
	Total  int    `json:"total,omitempty" jsonschema:"The items its connector named; 0 while not known"`
	Error  string `json:"error,omitempty" jsonschema:"Why it failed - cut to SyncErrorMax bytes"`
}
