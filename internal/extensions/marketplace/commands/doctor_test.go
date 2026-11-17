// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// doctor says what it did to each problem - repaired, or left with what to
// do - and where the backup is; a dry run says nothing changed.
func TestWriteRepair(t *testing.T) {
	actions := []mktapi.RepairAction{
		{Problem: mktapi.InstallationProblem{Kind: mktapi.ProblemVanishedPath, Entry: "old", Detail: "universe \"old\" names a path that is gone"}, Done: true, Did: "removed the row from universes.yaml (backed up first)"},
		{Problem: mktapi.InstallationProblem{Kind: mktapi.ProblemProgramReplaced, Entry: "ktw-b", Detail: "program \"ktw-b\" 1.0 was replaced"}, Did: "left: install it again: katyoti install ktw-b"},
	}
	var b bytes.Buffer
	writeRepair(&b, mktapi.InstallationRepair{Backup: "/root/backups/doctor-1", Actions: actions})
	out := b.String()
	for _, want := range []string{"repaired vanished-path", "removed the row from universes.yaml", "left program-replaced", "katyoti install ktw-b", "1 of 2 repaired", "backup of every file changed: /root/backups/doctor-1"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out)
		}
	}
	var dry bytes.Buffer
	writeRepair(&dry, mktapi.InstallationRepair{DryRun: true, Actions: actions})
	if !strings.Contains(dry.String(), "would repair vanished-path") || !strings.Contains(dry.String(), "nothing changed") {
		t.Errorf("a dry run's output:\n%s", dry.String())
	}
	var sound bytes.Buffer
	writeRepair(&sound, mktapi.InstallationRepair{})
	if !strings.Contains(sound.String(), "nothing to repair") {
		t.Errorf("a sound installation's output: %q", sound.String())
	}
}
