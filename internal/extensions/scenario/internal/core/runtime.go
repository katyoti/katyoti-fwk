// SPDX-License-Identifier: Apache-2.0

// Package core runs scenario test cases: it installs a case into its runtime
// folder (a universe, its sidecars, the project created from the case's
// folder), starts and stops that universe headless, drives it through its hub
// as the GUI does, runs the stages with their checks, lets a simulated user
// answer the Inbox, and checkpoints each stage that passes so a run restarts
// just before the one that failed.
package core

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// Runtime is a case's runtime folder, <root>/<case>: everything the case
// changes lives there, so a checkpoint is a copy of it. Stages run on an
// installed universe (a clean machine's case stage) name its folders instead:
// its home, its project, its hub's data.
type Runtime struct {
	Dir string
	// UniverseDir, ProjectDir and DataDir replace the case folder's own.
	UniverseDir, ProjectDir, DataDir string
}

func (r Runtime) Bin() string { return filepath.Join(r.Dir, "bin") }
func (r Runtime) Universe() string {
	if r.UniverseDir != "" {
		return r.UniverseDir
	}
	return filepath.Join(r.Dir, "universe")
}
func (r Runtime) Project() string {
	if r.ProjectDir != "" {
		return r.ProjectDir
	}
	return filepath.Join(r.Dir, "project")
}
func (r Runtime) Data() string {
	if r.DataDir != "" {
		return r.DataDir
	}
	return filepath.Join(r.Dir, "data")
}

// Sessions is where the case's hub keeps its sessions: under its data, the
// KTW_DATA it is started with (procs.go).
func (r Runtime) Sessions() string { return filepath.Join(r.Data(), "sessions") }

func (r Runtime) StateFile() string   { return filepath.Join(r.Dir, "state.json") }
func (r Runtime) Checkpoints() string { return filepath.Join(r.Dir, "checkpoints") }
func (r Runtime) Runs() string        { return filepath.Join(r.Dir, "runs") }

// OtherUniverse is the home of one of the case's other universes; OtherData
// its data.
func (r Runtime) OtherUniverse(name string) string {
	return filepath.Join(r.Dir, "universes", name)
}
func (r Runtime) OtherData(name string) string { return filepath.Join(r.Data(), "universes", name) }

// Repository is the folder of one of the case's other repositories, a member
// of its universe beside its project.
func (r Runtime) Repository(name string) string {
	return filepath.Join(r.Dir, "repositories", name)
}

// UniverseConfig is the universe's config folder.
func (r Runtime) UniverseConfig() string { return application.EnvOf(r.Universe()).ConfigDir() }

// UniverseHome is the universe's config root, where `copy` entries land.
func (r Runtime) UniverseHome() string { return application.EnvOf(r.Universe()).HomeDir() }

// snapshotParts are what a checkpoint copies: the case's whole changing state.
func (r Runtime) snapshotParts() []string {
	return []string{"universe", "universes", "project", "repositories", "data", "state.json"}
}

// State is where a case stands, kept in its runtime folder (and so in its
// checkpoints): the stages passed, the values steps saved, the Inbox requests
// the simulated user answered and the scripted answers it used.
type State struct {
	Installed bool           `json:"installed"`
	Passed    []string       `json:"passed,omitempty"`
	Saved     map[string]any `json:"saved,omitempty"`
	Answered  []string       `json:"answered,omitempty"`
	// Scripted are the scripted answers given (their indexes in the case's).
	Scripted  []int  `json:"scripted,omitempty"`
	Failure   string `json:"failure,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func loadState(r Runtime) (State, error) {
	var s State
	raw, err := os.ReadFile(r.StateFile())
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(raw, &s)
	return s, err
}

func saveState(r Runtime, s State) error {
	s.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.StateFile(), raw, 0o644)
}

// Engine runs one case.
type Engine struct {
	Case *api.Case
	RT   Runtime
	// BinSrc is the folder the sidecars' binaries are copied from at install.
	BinSrc string
	// CasesDir is the folder holding the case folders (what the GUI launcher
	// passes back to `scenario start`).
	CasesDir string
	Out      io.Writer
	// InstalledBinary answers the installed path of a control-plane binary
	// (the launcher's last place to find ktw-gui). Wired by the caller
	// (controlplane.TargetPath): this package reaches no framework itself.
	InstalledBinary func(name string) (string, error)
	// Universe replaces the case's universe name (an installed universe a
	// clean machine's case stage runs on).
	Universe string
}

// universeName is the universe the engine works on.
func (e *Engine) universeName() string {
	if e.Universe != "" {
		return e.Universe
	}
	return e.Case.UniverseName()
}

// NewEngine binds a case to its runtime folder under root.
func NewEngine(c *api.Case, root, binSrc string, out io.Writer) *Engine {
	if out == nil {
		out = os.Stdout
	}
	return &Engine{Case: c, RT: Runtime{Dir: filepath.Join(root, c.Name)}, BinSrc: binSrc, Out: out}
}

func (e *Engine) logf(format string, a ...any) {
	fmt.Fprintf(e.Out, format+"\n", a...)
}
