// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"encoding/json"
	"fmt"
	"time"
)

// WaitMode defines how the executor evaluates multiple wait conditions.
// Its zero value is no mode: a WaitSpec built without one is refused when the
// run waits (ErrWaitWithoutMode), never taken for a wait-for-all. A mode is
// saved by its name; a run saved when modes were numbers (all 0, any 1,
// custom 2) is read with its meaning kept.
type WaitMode int

const (
	// waitUnset is the zero value: no mode chosen, refused.
	waitUnset WaitMode = iota

	// WaitAny resumes the machine on the FIRST matching event.
	// Remaining subscriptions are cancelled.
	WaitAny

	// WaitAll resumes the machine when ALL conditions have matched.
	// The machine receives all collected events at once. The conditions met
	// so far live in memory only: they are not kept across a restart.
	WaitAll

	// WaitCustom resumes the machine on EVERY matching event.
	// The machine decides whether to re-suspend or transition.
	// Enables BPMN event gateway patterns: timeout, error catch, unwanted event handling.
	WaitCustom
)

// ErrWaitWithoutMode refuses a wait whose WaitSpec names no mode.
var ErrWaitWithoutMode = fmt.Errorf("engine: a wait names no mode (WaitAny, WaitAll or WaitCustom)")

func (m WaitMode) String() string {
	switch m {
	case WaitAll:
		return "wait_all"
	case WaitAny:
		return "wait_any"
	case WaitCustom:
		return "wait_custom"
	default:
		return "unknown"
	}
}

// Valid reports whether m is a chosen mode (not the zero value).
func (m WaitMode) Valid() bool { return m == WaitAny || m == WaitAll || m == WaitCustom }

// legacyWaitModes are the numbers a mode was saved as before it was saved by
// its name: WaitAll was the zero value then.
var legacyWaitModes = map[int]WaitMode{0: WaitAll, 1: WaitAny, 2: WaitCustom}

// MarshalJSON saves a mode by its name; no mode is saved as "".
func (m WaitMode) MarshalJSON() ([]byte, error) {
	if !m.Valid() {
		return json.Marshal("")
	}
	return json.Marshal(m.String())
}

// UnmarshalJSON reads a mode by its name, or a number saved before modes were
// saved by name (with the meaning it had then).
func (m *WaitMode) UnmarshalJSON(b []byte) error {
	var name string
	if err := json.Unmarshal(b, &name); err == nil {
		switch name {
		case "":
			*m = waitUnset
		case "wait_any":
			*m = WaitAny
		case "wait_all":
			*m = WaitAll
		case "wait_custom":
			*m = WaitCustom
		default:
			return fmt.Errorf("engine: unknown wait mode %q", name)
		}
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("engine: wait mode %s is neither a name nor a number", b)
	}
	legacy, ok := legacyWaitModes[n]
	if !ok {
		return fmt.Errorf("engine: unknown wait mode %d", n)
	}
	*m = legacy
	return nil
}

// WaitCondition defines a single event to wait for.
type WaitCondition struct {
	// Topic is the EventBus topic pattern (supports glob: "run.*.complete").
	Topic string `json:"topic"`

	// Filter is an optional property-based filter applied after topic match.
	// All entries must match for the condition to be satisfied.
	Filter map[string]string `json:"filter,omitempty"`

	// Label identifies which condition fired when the machine is resumed.
	Label string `json:"label"`

	// EventURI, when set, is an event of another process (an EventURI the
	// EventService resolves, e.g. an inbox request updated): while the run
	// waits, the task service forwards it onto Topic (ExternalEventTopic).
	EventURI string `json:"eventUri,omitempty"`
}

// WaitSpec defines what the executor waits for between steps.
// Replaces the old ResumeOn string with multi-condition, multi-mode waiting.
type WaitSpec struct {
	// Conditions lists the events to wait for.
	Conditions []WaitCondition `json:"conditions"`

	// Mode controls how conditions are evaluated. It has no default: a
	// WaitSpec without one is refused (ErrWaitWithoutMode).
	Mode WaitMode `json:"mode"`

	// Timeout is the maximum time to wait. Zero means no timeout.
	Timeout time.Duration `json:"timeout,omitempty"`

	// TimeoutLabel is the label used when the timeout fires.
	// The machine receives an event with this label and nil payload.
	TimeoutLabel string `json:"timeoutLabel,omitempty"`

	// RecheckOnResume: a run restored after a restart does not wait for the
	// event again — it may have come while no process listened — but steps at
	// once with a TickEvent; the machine checks what it waited for and waits
	// again if it has not happened.
	RecheckOnResume bool `json:"recheckOnResume,omitempty"`
}

// ResumeOnTopic creates a WaitSpec for a single topic (WaitAny with one condition).
// Backward-compat helper for simple single-event waits.
func ResumeOnTopic(topic string) *WaitSpec {
	return &WaitSpec{
		Conditions: []WaitCondition{{Topic: topic, Label: topic}},
		Mode:       WaitAny,
	}
}
