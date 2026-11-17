// SPDX-License-Identifier: Apache-2.0

package api

import (
	"time"

	"errors"
	"fmt"
)

// TASK GROUPS — a run is processed in a group that limits how many of its runs
// RUN at once. A run holds a slot of its group from the moment it starts or is
// woken until its machine waits (an input, an event, a child) or ends: waiting
// holds none. Groups are declared per universe in TaskGroups.yaml and do not
// see each other.

// DefaultGroup is the group of a run that names none and has no parent run:
// a person's (the GUI, the CLI).
const DefaultGroup = "user"

// DefaultMaxRunning is a group's limit when TaskGroups.yaml does not set one.
const DefaultMaxRunning = 8

// TaskGroupsConfig is TaskGroups.yaml.
type TaskGroupsConfig struct {
	Groups map[string]TaskGroup `yaml:"groups,omitempty" json:"groups,omitempty"`
	// DefaultGroup is the group of a run that names none and has no parent;
	// empty = DefaultGroup (user).
	DefaultGroup string `yaml:"default_group,omitempty" json:"default_group,omitempty"`
	// MaxRunning is how many runs of the universe's process run at once, all
	// groups together; 0 = DefaultUniverseMaxRunning. At the ceiling the
	// default group's runs (a person's) start first.
	MaxRunning int `yaml:"max_running,omitempty" json:"max_running,omitempty"`
	// PriorityAgingS: a queued run's priority rises by one every this many
	// seconds it waits, so a steady stream of higher-priority runs cannot
	// starve it; 0 = DefaultPriorityAgingS, negative = no aging.
	PriorityAgingS int `yaml:"priority_aging_s,omitempty" json:"priority_aging_s,omitempty"`
	// KeepAlive are the tasks the universe keeps running: started by the
	// serving process, after every restart, again after they end (with a
	// backoff), until they end too often.
	KeepAlive []KeepAliveTask `yaml:"keep_alive,omitempty" json:"keep_alive,omitempty"`
}

// KeepAliveTask is one task the universe keeps running.
type KeepAliveTask struct {
	Task  string         `yaml:"task" json:"task"`
	Group string         `yaml:"group,omitempty" json:"group,omitempty"`
	Input map[string]any `yaml:"input,omitempty" json:"input,omitempty"`
}

// KeepAliveInfo is a kept-alive task's state, as Groups lists it.
type KeepAliveInfo struct {
	Task string `json:"task"`
	// State: running · backing-off (it ended; started again at NextAt) ·
	// stopped (Reason; started again when a person runs it) · waiting-to-serve
	// (no serving process has started it yet).
	State    string     `json:"state"`
	RunID    string     `json:"runId,omitempty"`
	Reason   string     `json:"reason,omitempty"`
	NextAt   *time.Time `json:"nextAt,omitempty"`
	Restarts int        `json:"restarts"`
}

// DefaultUniverseMaxRunning is the ceiling across groups when TaskGroups.yaml
// sets none.
const DefaultUniverseMaxRunning = 32

// DefaultPriorityAgingS is the queued-priority aging period when
// TaskGroups.yaml sets none.
const DefaultPriorityAgingS = 60

// TaskGroup is one group's limits.
type TaskGroup struct {
	// MaxRunning is how many of its runs may run at once; 0 = DefaultMaxRunning.
	MaxRunning int `yaml:"max_running,omitempty" json:"max_running,omitempty"`
	// Priority is the group's rank among groups (for a global limit, later).
	Priority int `yaml:"priority,omitempty" json:"priority,omitempty"`
	// QueueLimit is how many runs may wait for a slot before a new run is
	// refused (ErrGroupFull); 0 = no limit.
	QueueLimit int `yaml:"queue_limit,omitempty" json:"queue_limit,omitempty"`
	// On: false pauses the group — its runs queue, none starts. Default true.
	On *bool `yaml:"on,omitempty" json:"on,omitempty"`
}

// ErrGroupFull: a run was refused because its group's queue is full.
var ErrGroupFull = errors.New("task group full")

// GroupFullError is ErrGroupFull with the group's load — what a submitter
// reads to back off.
type GroupFullError struct {
	Group      string
	Running    int
	Queued     int
	MaxRunning int
	QueueLimit int
}

func (e *GroupFullError) Error() string {
	return fmt.Sprintf("%v: %s runs %d of %d, %d queued (limit %d)", ErrGroupFull, e.Group, e.Running, e.MaxRunning, e.Queued, e.QueueLimit)
}

func (e *GroupFullError) Unwrap() error { return ErrGroupFull }

// GroupLoad is a group's load and limits.
type GroupLoad struct {
	Name       string `json:"name"`
	Running    int    `json:"running"`
	Waiting    int    `json:"waiting"`
	Queued     int    `json:"queued"`
	MaxRunning int    `json:"maxRunning"`
	QueueLimit int    `json:"queueLimit,omitempty"`
	Priority   int    `json:"priority,omitempty"`
	On         bool   `json:"on"`
}

// GroupsRequest asks for every group's load.
type GroupsRequest struct{}

// GroupsResponse lists the groups: the declared ones, and any a run used.
type GroupsResponse struct {
	Groups []GroupLoad `json:"groups"`
	// Warnings are what went wrong around the task service (its runs
	// folder, its TaskGroups.yaml, a run manifest it could not read).
	Warnings []string `json:"warnings,omitempty"`
	// KeepAlive are the tasks the universe keeps running, and how they are.
	KeepAlive []KeepAliveInfo `json:"keepAlive,omitempty"`
}

// SetGroupRequest changes a group live; unset fields stay as they are.
type SetGroupRequest struct {
	Group      string `json:"group"                desc:"The group; required"`
	MaxRunning *int   `json:"maxRunning,omitempty" desc:"Runs at once"`
	QueueLimit *int   `json:"queueLimit,omitempty" desc:"Queued runs before a new one is refused; 0 = no limit"`
	On         *bool  `json:"on,omitempty"         desc:"false pauses the group: its runs queue, none starts"`
}

// ClearGroupRequest removes queued runs of a group.
type ClearGroupRequest struct {
	Group string `json:"group" desc:"The group; required"`
	// OriginID, when set, removes only the runs started by this origin (the
	// planner clearing what it submitted).
	OriginID string `json:"originId,omitempty" desc:"Only the runs of this origin id"`
}

// ClearGroupResponse names the runs removed.
type ClearGroupResponse struct {
	Cleared []string `json:"cleared"`
}
