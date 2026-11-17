// SPDX-License-Identifier: Apache-2.0

package api

// TEAM PRIORITIES - one view for the weekly team
// meeting: the increments of the streams picked, in the order they will be
// done - delivered in the current releases (unless hidden), then one group
// per open release of the streams picked by due date across the streams, no
// release and the increments in no stream - each with its priority and its
// release changed on its line through the existing verbs.

// The kinds of Team priorities' groups: delivered first, then a group per
// open release (current, next or release for its place in its stream), then
// none, then no-stream.
const (
	TeamGroupDelivered = "delivered"
	TeamGroupCurrent   = "current"
	TeamGroupNext      = "next"
	// TeamGroupRelease is a stream's third or later open release: a group
	// of its own, like the current and the next.
	TeamGroupRelease = "release"
	TeamGroupNone    = "none"
	// TeamGroupNoStream holds the open increments in no stream: listed last,
	// whatever streams are picked, so none is hidden.
	TeamGroupNoStream = "no-stream"
)

// TeamPrioritiesInput asks Team priorities: the streams switched off and
// whether the delivered increments are hidden - every stream shown and the
// delivered shown by default.
type TeamPrioritiesInput struct {
	HiddenStreams []string `json:"hidden_streams,omitempty" jsonschema:"description=The streams switched off - every other stream is shown"`
	HideDelivered bool     `json:"hide_delivered,omitempty" jsonschema:"description=Hide the increments delivered in the current release"`
	// CurrentOnly keeps each stream's current release only - the planning
	// deck's Overview: no other group's lines are read.
	CurrentOnly bool `json:"current_only,omitempty" jsonschema:"description=Keep only each stream's current release - its open increments"`
	// Language is the person's (en or fr): the page's words.
	Language string `json:"language,omitempty" jsonschema:"description=The person's language (en or fr); empty = English"`
	// ID and Target are what the host hands a panel's source: unused.
	ID     string `json:"id,omitempty" jsonschema:"description=The object the panel is open on - unused"`
	Target string `json:"target,omitempty" jsonschema:"description=The Global strip's target - unused"`
}

// TeamPrioritiesOutput is Team priorities' data: its stream toggles and its
// groups in order.
type TeamPrioritiesOutput struct {
	Streams       []TeamStream `json:"streams" jsonschema:"description=Every stream with whether it is shown"`
	HideDelivered bool         `json:"hide_delivered,omitempty" jsonschema:"description=The delivered increments are hidden"`
	Groups        []TeamGroup  `json:"groups" jsonschema:"description=The groups in the order the work is done - delivered then one group per open release by due date across the streams then none then no-stream - only those holding a line"`
}

// TeamStream is one stream's toggle.
type TeamStream struct {
	ID    string `json:"id" jsonschema:"description=The stream,content=object-id"`
	Name  string `json:"name" jsonschema:"description=Its name"`
	Color string `json:"color,omitempty" jsonschema:"description=Its colour"`
	On    bool   `json:"on" jsonschema:"description=Its increments are shown"`
	// Releases are its releases still to ship by date: its current one,
	// its next, then the others.
	Releases []TeamRelease `json:"releases,omitempty" jsonschema:"description=Its releases still to ship by date - the current then the next then the others"`
}

// TeamGroup is one group of Team priorities: its separator and its lines.
type TeamGroup struct {
	Kind string `json:"kind" jsonschema:"description=delivered or current or next or release or none or no-stream"`
	// Stream is the stream a release group's release belongs to.
	Stream   string        `json:"stream,omitempty" jsonschema:"description=The stream of a release group's release,content=object-id"`
	Releases []TeamRelease `json:"releases,omitempty" jsonschema:"description=The release of a release group - none for the other groups"`
	Done     int           `json:"done" jsonschema:"description=How much of it is done - the mean of its lines in percent"`
	Lines    []TeamLine    `json:"lines" jsonschema:"description=Its increments by priority then rank"`
}

// TeamRelease is a release named on a separator.
type TeamRelease struct {
	ID       string `json:"id" jsonschema:"description=The release,content=object-id"`
	Label    string `json:"label" jsonschema:"description=How every view names it"`
	Date     string `json:"date,omitempty" jsonschema:"description=Its due date (YYYY-MM-DD)"`
	DaysLeft *int   `json:"days_left,omitempty" jsonschema:"description=Days left to its date - negative once late"`
	// Frozen takes no more increments: offered only on the lines in it.
	Frozen bool `json:"frozen,omitempty" jsonschema:"description=Frozen - it takes no more increments"`
}

// TeamLine is one increment's line.
type TeamLine struct {
	ID          string `json:"id" jsonschema:"description=The increment,content=object-id"`
	Title       string `json:"title" jsonschema:"description=Its short title"`
	Description string `json:"description,omitempty" jsonschema:"description=Its intent - shown on hover"`
	Stream      string `json:"stream" jsonschema:"description=Its stream,content=object-id"`
	Priority    string `json:"priority" jsonschema:"description=P0 to P3,content=priority"`
	// Release is the release it ships in or is planned for; empty: none.
	Release   string `json:"release,omitempty" jsonschema:"description=The release it ships in or is planned for,content=object-id"`
	Delivered bool   `json:"delivered,omitempty" jsonschema:"description=Done in its stream's current release"`
	Done      int    `json:"done" jsonschema:"description=Its requests done in percent"`
	Running   int    `json:"running" jsonschema:"description=Its requests running in percent"`
	NeedsYou  bool   `json:"needs_you,omitempty" jsonschema:"description=A step of it waits on a person"`
	Blocks    string `json:"blocks,omitempty" jsonschema:"description=What blocks it - one line each"`
	// Options are what its release drop-down offers: its stream's planned
	// releases - a frozen one only to the lines in it - then none (out).
	Options []RowOption `json:"options,omitempty" jsonschema:"description=The releases its drop-down offers - a frozen one only on the lines in it - out is no release"`
}
