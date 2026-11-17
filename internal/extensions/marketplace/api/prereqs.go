// SPDX-License-Identifier: Apache-2.0

package api

import "github.com/katyoti/katyoti-fwk/internal/core/publish"

// The state of a planned program.
const (
	// PrereqOK: every sidecar's check finds the program.
	PrereqOK = "ok"
	// PrereqInstall: missing, and a recipe for this platform can install it.
	PrereqInstall = "install"
	// PrereqManual: missing, and no recipe can run here — the install skill
	// (the prose fallback) says how, for an LLM or a person.
	PrereqManual = "manual"
)

// The scope a step installs in.
const (
	ScopeUser  = "user"  // the person's own profile: no prompt
	ScopeAdmin = "admin" // administrator rights: grouped behind ONE prompt
)

// PrereqsPlanInput names the universes whose sidecars' programs are planned.
type PrereqsPlanInput struct {
	Universes []string `json:"universes,omitempty" cli:"universe" desc:"The universes whose sidecars' programs are planned: registered names or folders (default: the default universe)"`
}

// PrereqsPlan is what the programs of a set of universes need on this machine:
// one row per program, deduplicated across the sidecars that declare it.
type PrereqsPlan struct {
	// Platform is the runtime.GOOS the plan is for.
	Platform string `json:"platform"`
	// Programs are the programs, by name.
	Programs []PlannedProgram `json:"programs"`
	// Elevation names the selected programs whose step needs administrator
	// rights: installed together, after one prompt.
	Elevation []string `json:"elevation,omitempty"`
	// Sidecars are the sidecars whose declarations were read.
	Sidecars []string `json:"sidecars,omitempty"`
	// Unchecked are the sidecars whose check could not be read, with why.
	Unchecked []string `json:"unchecked,omitempty"`
	// NotInstalled are the sidecars a universe is wired to whose program is
	// gone and that no installed item replaces: never checked, to reinstall
	//
	NotInstalled []string `json:"notInstalled,omitempty"`
	// Rewired are the universe members moved off a program that is gone onto
	// the installed version, as "<universe>: <sidecar>".
	Rewired []string `json:"rewired,omitempty"`
}

// PlannedProgram is one program a universe's sidecars need.
type PlannedProgram struct {
	Name string `json:"name"`
	// Required: at least one sidecar cannot run without it. An optional
	// program is offered, never preselected.
	Required bool `json:"required"`
	// Version is the highest version a sidecar declares it is validated with.
	Version string `json:"version,omitempty"`
	// NeededBy are the sidecars that declare it.
	NeededBy []string `json:"neededBy"`
	// Commands are the executable names the check looks up on PATH.
	Commands []string `json:"commands,omitempty"`
	// PythonPackage is the Python package the program is: its check looks
	// for that package in the Python pip installs with, never on PATH.
	PythonPackage string `json:"pythonPackage,omitempty"`
	// State is ok, install or manual.
	State string `json:"state"`
	// Found is the version the check found, when it found one.
	Found string `json:"found,omitempty"`
	// Recipe is the step chosen for this platform, in the declared order of
	// preference: the first whose installer is on this machine and that
	// satisfies Version.
	Recipe *publish.InstallRecipe `json:"recipe,omitempty"`
	// ScriptPath is a script recipe's script, resolved in its sidecar's folder.
	ScriptPath string `json:"scriptPath,omitempty"`
	// After is the program of the same plan that provides the chosen
	// recipe's installer (python for a pip recipe, bun for a bun one) when
	// this machine lacks it: that program is installed first.
	After string `json:"after,omitempty"`
	// Scope is where the chosen step installs: user, or admin.
	Scope string `json:"scope,omitempty"`
	// Selected: installed by "install all" — required, missing, with a recipe.
	Selected bool `json:"selected"`
	// InstallSkill is the prose install skill a sidecar names.
	InstallSkill string `json:"installSkill,omitempty"`
	// Fallback says, in prose, why no recipe runs here and what to follow.
	Fallback string `json:"fallback,omitempty"`
}

// PrereqsInstallInput installs programs of a plan.
type PrereqsInstallInput struct {
	Universes []string `json:"universes,omitempty" cli:"universe" desc:"The universes whose sidecars' programs are installed (default: the default universe)"`
	// Programs are the programs to install by name; empty installs the
	// selected ones (every required program missing). Naming an optional
	// program is how it is installed.
	Programs []string `json:"programs,omitempty" desc:"The programs to install (default: every required program missing)"`
	// DryRun returns the steps without running any.
	DryRun bool `json:"dryRun,omitempty" cli:"dry-run" desc:"Say which steps would run; run none"`
}

// PrereqsInstallOutput is what an install did.
type PrereqsInstallOutput struct {
	// Plan is the plan the install started from.
	Plan PrereqsPlan `json:"plan"`
	// Steps are the steps, in the order they ran.
	Steps []PrereqStep `json:"steps,omitempty"`
	// ElevationPrompts is how many times administrator rights were asked:
	// 0 or 1.
	ElevationPrompts int `json:"elevationPrompts"`
	// PathRefreshed: PATH was read again after the steps ran.
	PathRefreshed bool `json:"pathRefreshed,omitempty"`
	// After is the plan again, from the sidecars' checks run after the steps.
	After *PrereqsPlan `json:"after,omitempty"`
	// Missing are the required programs still missing at the end - in After
	// when the checks ran again, else in Plan (one no recipe installs here
	// among them): the install did not make the universe ready.
	Missing []string `json:"missing,omitempty"`
}

// The status of a step.
const (
	StepPlanned   = "planned"   // a dry run
	StepInstalled = "installed" // the step ran
	StepFailed    = "failed"    // the step ran and failed, or was refused
	StepSkipped   = "skipped"   // nothing to run: present, or no recipe here
)

// PrereqStep is one program's step.
type PrereqStep struct {
	Program string `json:"program"`
	Kind    string `json:"kind,omitempty"`
	Scope   string `json:"scope,omitempty"`
	Status  string `json:"status"`
	// Detail is what the step installs, or - failed - why, in one line.
	Detail string `json:"detail,omitempty"`
	// Report is a failed step's full report, when it ran a command.
	Report *StepReport `json:"report,omitempty"`
}

// StepReport is everything known of a failed step's command: what ran, how
// it ended and all it printed - what a person reads, or pastes to an
// assistant, to understand the failure.
type StepReport struct {
	// Command is the command line that ran.
	Command string `json:"command"`
	// ExitCode is its exit code; -1 when it did not exit on its own.
	ExitCode int `json:"exitCode"`
	// Code is the exit code as its installer names it (winget's 0x8A150010).
	Code string `json:"code,omitempty"`
	// Meaning is what that code means, when it is known.
	Meaning string `json:"meaning,omitempty"`
	// Output is all the command wrote, its output and errors interleaved.
	Output string `json:"output,omitempty"`
	// Logs is the folder holding the installer's own logs, when it keeps any.
	Logs string `json:"logs,omitempty"`
}
