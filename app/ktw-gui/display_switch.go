// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"regexp"
	"strings"
)

// switchEnvs are the increments' display switches the GUI reads, each with
// the variable that turns it on. Every approved or
// closed increment's switch is retired, its screens shown to every user
// (IN-045's, retired with the increment's close
// IN-1158's, a domain module's feature, with its delivery: the create dialog draws a
// type's create page, IN-1157's, with its delivery:
// every holder shows the object card, its children as cards with the
// « + Nouveau » shortcut and « Retirer » archiving; IN-035's,
// , with its delivery: the contribution system review's
// screens show for everyone). No increment's switch is read today.
var switchEnvs = map[string]string{}

// displaySwitches are the increments' display switches this start of the GUI
// has on, by increment: each off unless its variable says 1, true, yes or on.
func displaySwitches(getenv func(string) string) map[string]bool {
	on := make(map[string]bool, len(switchEnvs))
	for name, env := range switchEnvs {
		on[name] = switchOn(getenv(env))
	}
	return on
}

// switchOn reads a switch's variable: on only when it says so.
func switchOn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// DisplaySwitches answers the increments' display switches of this start.
//
// Wails-bound: window.go.main.App.DisplaySwitches().
func (a *App) DisplaySwitches() map[string]bool { return displaySwitches(os.Getenv) }

// incrementNumber is an increment id's number part: IN-045 in
var incrementNumber = regexp.MustCompile(`(?:^|\.)IN-(\d+)$`)

// switchVariable is the variable that turns an increment's display switch on,
// derived from its id - KTW_IN045_DISPLAY for - the
// one rule every switch's reader follows; empty for an id that
// names no increment.
func switchVariable(increment string) string {
	m := incrementNumber.FindStringSubmatch(strings.TrimSpace(increment))
	if m == nil {
		return ""
	}
	return "KTW_IN" + m[1] + "_DISPLAY"
}

// DisplaySwitchState is one increment's display switch as this start of the
// GUI reads it: the variable that turns it on and whether it says so.
type DisplaySwitchState struct {
	Increment string `json:"increment"`
	Variable  string `json:"variable"`
	On        bool   `json:"on"`
}

// displaySwitchStates answers each increment's switch variable and whether it
// is on; an id naming no increment answers no variable, off.
func displaySwitchStates(increments []string, getenv func(string) string) []DisplaySwitchState {
	out := make([]DisplaySwitchState, 0, len(increments))
	for _, inc := range increments {
		v := switchVariable(inc)
		out = append(out, DisplaySwitchState{Increment: inc, Variable: v, On: v != "" && switchOn(getenv(v))})
	}
	return out
}

// DisplaySwitchStates answers, for the increments given, the variable of each
// one's display switch and whether it is on in this GUI process's environment
// - read-only: the debug view's « Interrupteurs d'affichage ».
//
// Wails-bound: window.go.main.App.DisplaySwitchStates(increments).
func (a *App) DisplaySwitchStates(increments []string) []DisplaySwitchState {
	return displaySwitchStates(increments, os.Getenv)
}
