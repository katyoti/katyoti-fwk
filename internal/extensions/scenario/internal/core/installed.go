// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// STAGES ON AN INSTALLED UNIVERSE — a clean machine's case
// stage runs another case's stages, its steps and checks as a scenario
// universe runs them, on the universe the installer made: its hub reached
// through the installation's data, its project the one it serves, its Inbox
// answered by that case's simulated user. One definition for both.

// InstalledUniverse is the installed universe stages run on.
type InstalledUniverse struct {
	// Name is the universe's name, Home its folder, Data its hub's data
	// folder, Project the project it serves.
	Name, Home, Data, Project string
	// Work keeps the stages' reports and state.
	Work string
}

// RunStagesOn runs stages of the case named name on an installed universe, in
// order, each one's steps then its checks; user is that case's user file (the
// simulated user's persona, or its scripted answers). The error names the
// first stage that failed and why. A stage answering the Inbox gives the
// universe the simulated user's task, as a scenario universe has it.
func RunStagesOn(ctx context.Context, name string, stages []api.Stage, user string, u InstalledUniverse, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if err := os.MkdirAll(u.Work, 0o755); err != nil {
		return err
	}
	c := &api.Case{Name: name, Dir: u.Work, Stages: stages}
	if user != "" {
		if err := os.WriteFile(filepath.Join(u.Work, "user.md"), []byte(user), 0o644); err != nil {
			return err
		}
		c.User = "user.md"
	}
	e := &Engine{Case: c, Out: out, Universe: u.Name,
		RT: Runtime{Dir: u.Work, UniverseDir: u.Home, ProjectDir: u.Project, DataDir: u.Data}}
	if answersInbox(stages) {
		tasks := filepath.Join(e.RT.UniverseHome(), "tasks")
		if err := os.MkdirAll(tasks, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(tasks, SimUserTask+".md"), simUserTaskMD, 0o644); err != nil {
			return err
		}
	}
	var st State
	for _, s := range stages {
		ok, err := e.runStage(ctx, s, &st)
		if err != nil || !ok {
			return fmt.Errorf("stage %s: %s", s.ID, errOr(err, "a check did not hold"))
		}
		fmt.Fprintf(out, "stage %s passed\n", s.ID)
	}
	return nil
}

// answersInbox is whether a stage answers the Inbox.
func answersInbox(stages []api.Stage) bool {
	for _, s := range stages {
		for _, st := range s.Steps {
			if st.AnswerInbox != nil {
				return true
			}
		}
	}
	return false
}
