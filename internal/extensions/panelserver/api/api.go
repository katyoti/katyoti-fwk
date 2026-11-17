// SPDX-License-Identifier: Apache-2.0

// Package api is the panel server's contract: the one bus
// member that keeps each display panel's log and snapshot in the universe's
// data folder, so a panel closed and opened again - even after a restart -
// shows where its work is now.
//
// The server subscribes to every panel subject of its universe
// (subjects.PanelsPattern) and is the one writer of `<DataDir>/panels/`:
// `<id>.jsonl`, the panel's log - each event out and each input in, in the
// order taken - and `<id>.snapshot.json`, what a screen needs to draw the
// panel after its last event. A screen opens a panel by reading its snapshot,
// then follows its events from the snapshot's sequence (
// PanelFollower). Ended panels are pruned after their type's retention (as
// ended runs by default, 7 days); a panel waiting for an answer is kept
// until it is answered. The files are runtime data, never committed
//
// What lasts is the outcome: when a panel whose type keeps it
// (`keep: outcome | full`), or one the person asked to keep, ends, the server
// writes its final state and the person's inputs as one document attached to
// the object its open named - in that object's module, committed like any
// document. A panel that keeps nothing writes nothing in a module.
//
// It carries the panels' own events (internal/core/panel):
// no second event shape.
package api

import (
	"context"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
)

// DefaultRetention is how long an ended panel is kept when its type says
// nothing: as ended runs.
const DefaultRetention = 7 * 24 * time.Hour

// Snapshot is a panel as its events left it, after event Seq: what a screen
// folds to draw it (Events, compacted: the open, then the ops, progress, ask
// and end still in force, each with its own sequence number) and where the
// panel stands.
type Snapshot struct {
	Panel  string      `json:"panel" desc:"The panel's id"`
	Seq    int64       `json:"seq" desc:"The sequence number of the last event the snapshot holds: follow from it"`
	State  panel.State `json:"state,omitempty" desc:"open, asking, complete or closed"`
	Type   string      `json:"type,omitempty" desc:"The panel-type URI its open named"`
	Title  string      `json:"title,omitempty" desc:"The panel's title"`
	Owner  string      `json:"owner,omitempty" desc:"Who owns the panel: the run driving it"`
	Parent string      `json:"parent,omitempty" desc:"The panel it is a child of"`
	Object string      `json:"object,omitempty" desc:"The object its work is on: where a kept outcome is written"`
	// KeepAsked is the person's ask to keep the panel's outcome, whatever its
	// type keeps.
	KeepAsked bool `json:"keepAsked,omitempty" desc:"The person asked to keep its outcome"`
	// Kept is the document its outcome was written to, module-relative in
	// the object's module, once it is written.
	Kept string `json:"kept,omitempty" desc:"The document its outcome was written to: a path in its object's module"`
	// Waiting is an ask no input has answered yet: the panel is kept until
	// it is answered.
	Waiting bool `json:"waiting,omitempty" desc:"An ask no input has answered yet"`
	// Inputs counts the person's inputs taken.
	Inputs  int       `json:"inputs,omitempty" desc:"How many inputs the person sent"`
	Opened  time.Time `json:"opened,omitzero" desc:"When its open was taken"`
	Updated time.Time `json:"updated,omitzero" desc:"When its last event or input was taken"`
	// Ended is when it completed or closed: its retention runs from there.
	Ended  time.Time     `json:"ended,omitzero" desc:"When it completed or closed"`
	Events []panel.Event `json:"events,omitempty" desc:"What a screen folds to draw it, compacted, in order"`
}

// Record is one line of a panel's log: an event out or an input in, when it
// was taken. An input's After is the sequence of the last event before it.
type Record struct {
	At    time.Time    `json:"at" desc:"When the server took it"`
	Event *panel.Event `json:"event,omitempty" desc:"An event out of the panel"`
	Input *panel.Input `json:"input,omitempty" desc:"An input the person sent"`
	After int64        `json:"after,omitempty" desc:"For an input: the sequence of the last event before it"`
}

// Summary is one panel the server keeps.
type Summary struct {
	Panel   string      `json:"panel" desc:"The panel's id"`
	Seq     int64       `json:"seq" desc:"The sequence of its last event"`
	State   panel.State `json:"state,omitempty" desc:"open, asking, complete or closed"`
	Type    string      `json:"type,omitempty" desc:"Its panel-type URI"`
	Title   string      `json:"title,omitempty" desc:"Its title"`
	Owner   string      `json:"owner,omitempty" desc:"The run driving it"`
	Waiting bool        `json:"waiting,omitempty" desc:"An ask no input has answered yet"`
	Updated time.Time   `json:"updated,omitzero" desc:"When it last changed"`
}

// Outcome is what a kept panel leaves on its object when it ends: its final
// snapshot, the person's inputs, and - for a type that keeps it all - its
// whole log.
type Outcome struct {
	Snapshot Snapshot `json:"snapshot" desc:"The panel as it ended"`
	// Keep is what is kept: outcome or full (panel.KeepOutcome, KeepFull).
	Keep   string   `json:"keep" desc:"outcome or full"`
	Inputs []Record `json:"inputs,omitempty" desc:"The inputs the person sent, in order"`
	Log    []Record `json:"log,omitempty" desc:"The whole log, for a type that keeps it all"`
}

// DocumentFolder is the folder of an object's module the kept outcomes are
// written to: <module>/panels/<panel id>.md.
const DocumentFolder = "panels"

// MsgUnknownPanel is the message key (usermsg) of the refusal of a panel the
// server keeps nothing of - a snapshot, a history or a keep of it: a screen
// tells such a panel by this key (KTSP Data.msg) and follows it live from its
// start, never by the error's text. Its param `panel` is the panel's id.
const MsgUnknownPanel = "panel.unknownPanel"

// SnapshotInput names the panel.
type SnapshotInput struct {
	Panel string `json:"panel" cli:"arg" desc:"The panel's id (dp-…)" required:"true"`
}

// HistoryInput names the panel and the sequence to read after.
type HistoryInput struct {
	Panel string `json:"panel" cli:"arg" desc:"The panel's id (dp-…)" required:"true"`
	From  int64  `json:"from,omitempty" desc:"Read the events after this sequence and the inputs taken after it (0: from the open)"`
}

// HistoryOutput is the panel's log after From, in the order taken.
type HistoryOutput struct {
	Records []Record `json:"records" desc:"The log's lines after From"`
}

// KeepInput names the panel the person asks to keep.
type KeepInput struct {
	Panel string `json:"panel" cli:"arg" desc:"The panel's id (dp-…)" required:"true"`
}

// KeepOutput says whether the outcome is written yet: at once for a panel
// that has ended, else when it ends.
type KeepOutput struct {
	Kept string `json:"kept,omitempty" desc:"The document its outcome was written to, once written"`
	// Pending is a panel not ended yet, or one whose object could not take
	// the document yet: it is written when it ends, or tried again.
	Pending bool `json:"pending,omitempty" desc:"Written when the panel ends"`
}

// ListInput filters the panels kept.
type ListInput struct {
	Owner string `json:"owner,omitempty" desc:"Only the panels this run owns"`
}

// ListOutput is every panel kept, the last changed first.
type ListOutput struct {
	Panels []Summary `json:"panels" desc:"The panels kept"`
}

// TemplateInput names a page template and the hash of the
// tree the host holds, if any.
type TemplateInput struct {
	URI  string `json:"uri" cli:"arg" desc:"The page template's URI (<pack>:/page-template/<name>)" required:"true"`
	Hash string `json:"hash,omitempty" desc:"The hash of the tree the host holds: a current one gets the hash alone"`
}

// TemplateOutput is a page template's tree and its hash (render.SpecHash):
// the tree is left out when the host holds it at that hash.
type TemplateOutput struct {
	Tree map[string]any `json:"tree,omitempty" desc:"The template's page tree, unless the host holds it at Hash"`
	Hash string         `json:"hash" desc:"The tree's hash: sha256 of its JSON"`
}

// PanelService reads what the panel server keeps - a panel's snapshot, its
// log, the panels kept - and takes the person's ask to keep a panel's
// outcome.
type PanelService interface {
	Snapshot(ctx context.Context, in SnapshotInput) (Snapshot, error)
	History(ctx context.Context, in HistoryInput) (HistoryOutput, error)
	List(ctx context.Context, in ListInput) (ListOutput, error)
	Keep(ctx context.Context, in KeepInput) (KeepOutput, error)
	// Template answers a page template's tree by its URI:
	// what a panel type's page, or any answer naming a tree by {uri, hash},
	// draws - the hash alone to a host holding the current tree.
	Template(ctx context.Context, in TemplateInput) (TemplateOutput, error)
}

// PanelFollow is the in-process follow of a panel: its events after a
// sequence, then each one as it is taken, coalesced - at most a few
// deliveries a second, each with every event taken since the last.
type PanelFollow interface {
	Follow(id string, from int64, deliver func([]panel.Event)) (stop func(), err error)
}

// PanelServiceDef publishes PanelService on the bus (`ktw-hub:panel-service/<method>`).
var PanelServiceDef = meta.ServiceDef{
	Name:  "PanelService",
	Short: "The display panels the universe keeps: a panel's snapshot, its log, the panels kept",
	Long: `The panel server subscribes to every display panel of the universe and keeps
each one's log and snapshot in the universe's data folder. A
screen opens a panel by reading its snapshot, then follows its events from the
snapshot's sequence; the log is its history.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Snapshot": meta.MethodLong("A panel's snapshot",
			`The panel after its last event: where it stands and the events a screen folds to draw it, compacted, each with its sequence. Follow its events from seq.`),
		"History": meta.MethodLong("A panel's log after a sequence",
			`Each event out of the panel after from, and each input the person sent after it, in the order the server took them.`),
		"List": meta.MethodLong("The panels kept",
			`Every panel whose log the server keeps, the last changed first; owner keeps only one run's.`),
		"Keep": meta.MethodLong("Keep a panel's outcome",
			`The person's ask to keep this panel: when it ends - at once if it has - its final state and the person's inputs are written as a document attached to the object its work is on, whatever its type keeps.`),
		"Template": {Short: "A page template's tree by its URI", ReadOnly: true,
			Long: "The page tree a page-template contribution holds, with its hash (sha256 of its JSON): what an answer naming a tree by {uri, hash} - a panel type's page, a page source's or a section's tree - draws. A host giving the hash it holds gets the hash alone when it is current, so a template it holds costs no tree on the wire."},
	},
}
