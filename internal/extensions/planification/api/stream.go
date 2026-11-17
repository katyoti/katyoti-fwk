// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// STREAMS — a line of releases management recognises by name (the KaTyoti
// packages, ArchiFactory, the docs site): the level above the release. A
// stream numbers its releases by one VERSION SCHEME and makes them by one
// RELEASE MODE. A release is IN exactly one stream, an increment may name the
// one it aims at (both by `in-stream`, N:1); a module serves several streams,
// so it is in none. Stream objects live under a module (`streams/`, SM-n).

// The `stream` object type.
const StreamType = "stream"

// The version schemes of a stream.
const (
	// SchemeSemver numbers releases 1.4.0, or major.minor (1.005); -rcN allowed.
	SchemeSemver = "semver"
	// SchemeCalendar names releases by month: 2026.10, -rcN allowed.
	SchemeCalendar = "calendar"
	// SchemeDate names releases by day: 2026-10-10.
	SchemeDate = "date"
)

// The release modes of a stream.
const (
	// ModeManual: a person plans and makes each release (the default).
	ModeManual = "manual"
	// ModeOnChange: a release is made when an increment is delivered, at most
	// once per min_interval.
	ModeOnChange = "on-change"
	// ModePeriodic: a release is made every `every`, holding the increments
	// delivered since the last.
	ModePeriodic = "periodic"
)

// RolePlatform marks the stream of the platform: drawn below the others, thin.
const RolePlatform = "platform"

// StreamFields are the stream type's own fields. Its data-type, stream, is
// rendered from it.
type StreamFields struct {
	Color         string `json:"color,omitempty"          jsonschema:"title=Color,description=Its color in the drawings (#3651D4)"`
	Order         int    `json:"order,omitempty"          jsonschema:"title=Order,description=Its rank among the streams: the first is drawn on top"`
	Role          string `json:"role,omitempty"           jsonschema:"title=Role,description=platform: the platform the other streams build on; drawn below them as a thin lane with smaller cards,enum=platform"`
	VersionScheme string `json:"version_scheme,omitempty" jsonschema:"title=Version scheme,description=How its releases are numbered: semver (1.4.0 or 1.005) or calendar (2026.10) - both with -rcN - or date (2026-10-10),enum=semver|calendar|date"`
	ReleaseMode   string `json:"release_mode,omitempty"   jsonschema:"title=Release mode,description=How its releases are made: manual (by a person - the default) or on-change (when an increment is delivered - at most once per min_interval) or periodic (every 'every'),enum=manual|on-change|periodic"`
	MinInterval   string `json:"min_interval,omitempty"   jsonschema:"title=Minimum interval,description=on-change: the least time between two releases (1d or 2d)"`
	Every         string `json:"every,omitempty"          jsonschema:"title=Every,description=periodic: the time between two releases (14d)"`
	Review        string `json:"review,omitempty"         jsonschema:"title=Review,description=Who reviews the work its releases ship: agent (the default) or person,enum=agent|person"`
	LastCut       string `json:"last_cut,omitempty"       jsonschema:"title=Last release made,description=When it last made a release by its mode (RFC 3339)"`
}

// CutInput makes a stream's next release.
type CutInput struct {
	ID string `json:"id"           jsonschema:"description=The stream,required"`
	// Timer is the stream's timer that ran it: its daily release-due timer
	// moves what is past its date instead of cutting.
	Timer string       `json:"timer,omitempty" jsonschema:"description=The timer that ran it - release-due moves the increments not done of a release past its date and makes no release"`
	By    cwmapi.Actor `json:"by,omitempty" jsonschema:"description=Who makes it: empty - planification (its timer or a delivered increment)"`
}

// CarryOverOutput is what a stream's releases past their date moved: each
// increment not done, to the stream's next release.
type CarryOverOutput struct {
	Moved []string `json:"moved"`
	Note  string   `json:"note,omitempty"`
}

// CutOutput is the release a cut made, or why none.
type CutOutput struct {
	Release    *Release `json:"release,omitempty"`
	Increments []string `json:"increments"`
	Note       string   `json:"note,omitempty" jsonschema:"description=Why nothing was made or why the release stayed planned"`
}

// Stream is a stream as the service reads it.
type Stream struct {
	ID     string `json:"id"`
	Module string `json:"module"`
	Name   string `json:"name"`
	StreamFields
	// Cut is when it next makes a release (NextCut), read back with it.
	Cut StreamCut `json:"cut" jsonschema:"description=When it next makes a release by its mode"`
}

// CreateStreamInput creates a stream under a module.
type CreateStreamInput struct {
	Module string `json:"module" jsonschema:"description=The module it lives under,required"`
	Name   string `json:"name"   jsonschema:"description=Its name (KaTyoti packages),required"`
	StreamFields
}

// SetStreamInput changes a stream's scheme or mode; an empty field keeps its value.
type SetStreamInput struct {
	ID string `json:"id" jsonschema:"description=The stream,required"`
	StreamFields
}

// StreamInInput places a release or an increment in a stream.
type StreamInInput struct {
	ID     string `json:"id"     jsonschema:"description=The release or the increment,required"`
	Stream string `json:"stream" jsonschema:"description=The stream,required"`
}

// ListStreamsInput lists the streams of a module, or of every module.
type ListStreamsInput struct {
	Module string `json:"module,omitempty"`
}

// StreamOutput answers one stream.
type StreamOutput struct {
	Stream Stream `json:"stream"`
	Path   string `json:"path,omitempty"`
}

// ListStreamsOutput answers streams.
type ListStreamsOutput struct {
	Streams []Stream `json:"streams"`
}

// StreamServiceDef describes the service for KTSP/CLI auto-generation.
var StreamServiceDef = meta.ServiceDef{
	Name:    "streams",
	Short:   "Streams: the lines of releases above the release - each numbering its releases by one version scheme and making them by one release mode; a release is in exactly one",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Create":       meta.Method("Create a stream under a module: its name - its version scheme (semver or calendar or date) - its release mode (manual or on-change with min_interval or periodic with every)"),
		"Get":          {Short: "Read one stream", ReadOnly: true},
		"List":         {Short: "The streams of a module or of every module", ReadOnly: true},
		"Set":          meta.Method("Change a stream's version scheme or release mode (an empty field keeps its value)"),
		"ReleaseIn":    meta.Method("Put a release in a stream (exactly one: it replaces the one it was in); its version must follow the stream's scheme unless it ended"),
		"CarryOver":    meta.Method("Move the increments not done of a stream's releases past their date to its next release - planned for it - each saying why; the release ships without them. Its daily timer runs it"),
		"Cut":          meta.Method("Make a stream's next release: the increments naming it - done and in no release yet - in a release with the scheme's next version and a computed level (major when one is breaking) - frozen; nothing delivered: nothing made. Its timer (periodic) and a delivered increment (on-change) run it"),
		"AimIncrement": meta.Method("Name the stream an increment aims at (one: it replaces the one it named)"),
		"Lint":         meta.Method("The lint hook of a stream: a module in it (a module serves several streams - its releases are in one instead)"),
		"Lanes":        {Short: "Every stream's lane - or one's: its mode and next cut, its releases by date as cards (stage, done of total, a dot per gate, planned for it), its backlog - rows of the stream-lane data type", ReadOnly: true},
		"RunningPage":  {Short: "The Delivery view's Running tab, loaded when it opens: every increment being built with its open requests", ReadOnly: true},
		"DeliveryPage": {Short: "The Delivery view: every stream as a lane - its release mode and next cut with Change mode and Cut now - its releases with their stage and done of total and a dot per gate - its backlog with its held increments; with a stream's id that stream's lane alone (its pane)", ReadOnly: true},
	},
}

// StreamService manages streams.
type StreamService interface {
	Create(ctx context.Context, in CreateStreamInput) (StreamOutput, error)
	Get(ctx context.Context, in GetInput) (StreamOutput, error)
	List(ctx context.Context, in ListStreamsInput) (ListStreamsOutput, error)
	Set(ctx context.Context, in SetStreamInput) (StreamOutput, error)
	ReleaseIn(ctx context.Context, in StreamInInput) (ReleaseOutput, error)
	AimIncrement(ctx context.Context, in StreamInInput) (IncrementOutput, error)
	Cut(ctx context.Context, in CutInput) (CutOutput, error)
	CarryOver(ctx context.Context, in CutInput) (CarryOverOutput, error)
	Lint(ctx context.Context, in HookInput) (LintOutput, error)
	DeliveryPage(ctx context.Context, in DeliveryPageInput) (render.Def, error)
	RunningPage(ctx context.Context, in DeliveryPageInput) (render.Def, error)
	Lanes(ctx context.Context, in LanesInput) (LanesOutput, error)
}

// DeliveryPageInput asks the Delivery view: every stream, or one.
type DeliveryPageInput struct {
	ID     string `json:"id,omitempty" jsonschema:"description=A stream: its lane alone (its pane)"`
	Target string `json:"target,omitempty" jsonschema:"description=The Global strip's target: a stream's id shows its lane alone - anything else every stream"`
	// Language is the person's (en or fr): the Running tab's words.
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
}

// daysRe reads a stream's period: a whole number of days (14d).
var daysRe = regexp.MustCompile(`^([1-9]\d*)d$`)

// Period turns a stream's period ("14d") into a duration.
func Period(s string) (time.Duration, bool) {
	m := daysRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, _ := strconv.Atoi(m[1])
	return time.Duration(n) * 24 * time.Hour, true
}

// NextCut is when a stream next makes a release by its mode
// , the one answer every view reads: periodic, its last
// cut plus its period (from now when it never cut); on change, at the earliest
// its last cut plus its minimum interval (a day when none), once an increment
// is delivered; manual, never by itself - At is then zero.
func NextCut(f StreamFields, now time.Time) StreamCut {
	last, err := time.Parse(time.RFC3339, f.LastCut)
	if err != nil {
		last = time.Time{}
	}
	from := func(d time.Duration) time.Time {
		if last.IsZero() {
			return now.UTC().Add(d)
		}
		return last.Add(d)
	}
	switch f.ReleaseMode {
	case ModePeriodic:
		d, ok := Period(f.Every)
		if !ok {
			return StreamCut{Mode: ModePeriodic, Text: "periodic, with no valid period: it does not cut"}
		}
		at := from(d)
		return StreamCut{Mode: ModePeriodic, At: at.Format("2006-01-02"), Text: "periodic every " + f.Every + " - next cut " + at.Format("2006-01-02")}
	case ModeOnChange:
		gap, ok := Period(f.MinInterval)
		every := f.MinInterval
		if !ok {
			gap, every = 24*time.Hour, "1d"
		}
		at := from(gap)
		if last.IsZero() || at.Before(now) {
			return StreamCut{Mode: ModeOnChange, Text: "on change, at most every " + every + " - its next delivered increment cuts it"}
		}
		return StreamCut{Mode: ModeOnChange, At: at.Format("2006-01-02"), Text: "on change, at most every " + every + " - not before " + at.Format("2006-01-02")}
	}
	return StreamCut{Mode: ModeManual, Text: "manual - a person cuts it with Cut now"}
}

// StreamCut is when a stream next makes a release, and how its mode says it.
type StreamCut struct {
	Mode string `json:"mode" jsonschema:"description=Its release mode: manual or on-change or periodic"`
	// At is the day of its next cut (YYYY-MM-DD); empty when none is due by date.
	At   string `json:"at,omitempty" jsonschema:"description=The day of its next cut (YYYY-MM-DD) - empty for a manual stream or an on-change one whose next delivery cuts it"`
	Text string `json:"text" jsonschema:"description=Its mode and next cut in one line"`
}
