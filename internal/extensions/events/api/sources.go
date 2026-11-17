// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// EVENTS FROM OUTSIDE THE OBJECTS. What happens outside the
// corpus is raised on the object it concerns through CWM's EventRaise, with
// its cause, and reacted to like any write: a CODE CHANGE (a new git HEAD: its
// changed paths, mapped to the module objects whose `code_packages` hold them),
// a TIMER (recurring, declared on an object by the `timed` capability; or
// delayed, once, as a keyed to-do run at its time), a CALL FROM OUTSIDE through
// the hub (Raise, cause external). The serving process polls; Poll steps it on
// demand (tests, scenario cases). The sources' cursors — the last HEAD seen,
// each timer's last firing — are uncommitted state under the data folder
//

// The event types the events pack declares, and their points.
const (
	EventCodeChanged  = "code.changed"
	EventTimerElapsed = "timer.elapsed"
	PointCodeChange   = cwmapi.HookPoint("on_code_change")
	PointTimer        = cwmapi.HookPoint("on_timer")
)

// The causes the sources set (cause.Cause.Kind).
const (
	CauseCode     = "code"     // id: the new HEAD
	CauseTimer    = "timer"    // id: <object>#<timer>
	CauseExternal = "external" // id: who called
)

// CapTimed is the capability an object declares its recurring timers with.
const CapTimed = "timed"

// WorkDoerDelayed marks a delayed event's to-do (its work packet's doer).
const WorkDoerDelayed = "delayed-event"

// Commit is one commit of a code change.
type Commit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// CodeChanged is a code.changed event's payload: the commits between the HEAD
// last seen and the new one, and the paths they changed that the module's code
// packages hold.
type CodeChanged struct {
	Repo    string   `json:"repo"`
	From    string   `json:"from"`
	To      string   `json:"to"`
	Commits []Commit `json:"commits"`
	Paths   []string `json:"paths"`
}

// TimerElapsed is a timer.elapsed event's payload.
type TimerElapsed struct {
	Timer string `json:"timer"`
	Due   string `json:"due" jsonschema:"When it was due (RFC 3339)"`
}

// Timer is one recurring timer of an object: every a duration, or daily at a
// time of day — not both.
type Timer struct {
	Name  string `json:"name"            jsonschema:"description=The timer's name: unique on its object; the event's payload names it"`
	Every string `json:"every,omitempty"  jsonschema:"description=A period (a Go duration: 30m or 24h); the first firing one period after it is first seen"`
	At    string `json:"at,omitempty"     jsonschema:"description=A time of day (HH:MM local); fires once a day; a firing missed while the universe was down fires once at start"`
}

// TimedFields is the `timed` capability's field group.
type TimedFields struct {
	Timers []Timer `json:"timers" jsonschema:"description=The object's recurring timers"`
}

// PollInput steps the sources once.
type PollInput struct{}

// Raised is one event a source raised.
type Raised struct {
	Type    string `json:"type"`
	Object  string `json:"object"`
	EventID string `json:"event_id"`
	Detail  string `json:"detail,omitempty"`
}

// PollOutput is what one poll raised.
type PollOutput struct {
	Raised []Raised `json:"raised"`
	Head   string   `json:"head,omitempty" jsonschema:"The git HEAD now seen"`
}

// RaiseInput raises a declared event from outside (through the hub).
type RaiseInput struct {
	Type    string          `json:"type"    jsonschema:"description=The event type (declared with a point),required"`
	Object  string          `json:"object"  jsonschema:"description=The object it concerns,required"`
	Payload json.RawMessage `json:"payload,omitempty"`
	By      cwmapi.Actor    `json:"by"      jsonschema:"description=Who raises it: the cause's id,required"`
}

// AfterInput raises a declared event once, later.
type AfterInput struct {
	Type    string          `json:"type"   jsonschema:"description=The event type,required"`
	Object  string          `json:"object" jsonschema:"description=The object it concerns,required"`
	Payload json.RawMessage `json:"payload,omitempty"`
	After   string          `json:"after"  jsonschema:"description=How long from now (a Go duration: 90s or 2h),required"`
	Key     string          `json:"key,omitempty" jsonschema:"description=Delayed once per key: a second After with an open key is the first"`
}

// AfterOutput is the delayed event's to-do.
type AfterOutput struct {
	Todo string `json:"todo"`
	Due  string `json:"due"`
}

// EventSourceService raises events from outside the objects.
type EventSourceService interface {
	Poll(ctx context.Context, in PollInput) (PollOutput, error)
	Raise(ctx context.Context, in RaiseInput) (Raised, error)
	After(ctx context.Context, in AfterInput) (AfterOutput, error)
}

// EventSourceServiceDef describes the service for KTSP / CLI.
var EventSourceServiceDef = meta.ServiceDef{
	Name:    "event-sources",
	Short:   "Events from outside the objects: code changes (git), timers, calls from outside - raised on the object they concern, with their cause",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Poll":  {Short: "Step the sources once now: a new git HEAD raises code.changed on the modules its paths belong to; due timers and delayed events raise theirs", TimeoutS: 300},
		"Raise": meta.Method("Raise a declared event on an object from outside (cause external): what an outside system calls through the hub"),
		"After": meta.Method("Raise a declared event on an object once, later: a keyed to-do run at its time"),
	},
}
