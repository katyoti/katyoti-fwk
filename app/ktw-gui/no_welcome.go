// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
)

// NoWelcomeEnv turns the welcome screens off for one start, like --no-welcome.
const NoWelcomeEnv = "KTW_NO_WELCOME"

// noWelcomeArg reports whether this start opens no welcome screen - neither a
// universe's first guide nor « Quoi de neuf »: the --no-welcome
// flag, or KTW_NO_WELCOME set to anything but empty, 0 or false. Tests and
// screen captures start this way.
func noWelcomeArg(args []string, env string) bool {
	for _, a := range args {
		if a == "--no-welcome" || a == "--no-welcome=true" {
			return true
		}
	}
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "", "0", "false", "no":
		return false
	}
	return true
}

// NoWelcome reports whether this start of the GUI opens no welcome screen by
// itself; a universe's own setting (Univers.yaml no_welcome) is read beside it
// from the universe list.
//
// Wails-bound: window.go.main.App.NoWelcome().
func (a *App) NoWelcome() bool {
	return a.noWelcome
}

// startNoWelcome is this process's no-welcome option.
func startNoWelcome() bool { return noWelcomeArg(os.Args[1:], os.Getenv(NoWelcomeEnv)) }
