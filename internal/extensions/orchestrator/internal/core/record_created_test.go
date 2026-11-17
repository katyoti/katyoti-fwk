// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A CREATION PATH OUTSIDE THE ORCHESTRATOR RECORDS ITS FOLDER THROUGH IT
// : RecordCreatedProject adds an existing folder holding a
// project laid out to the record of the projects created on this PC, once -
// so it joins a universe beside other members - and refuses
// a relative path, a missing folder and an empty one, recording nothing.
func TestRecordCreatedProjectRecordsALaidOutFolderOnce(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	isolateDataDir(t)
	registry, _ := catalog.LoadRegistry(filepath.Join(home, "universes.yaml"))
	m := &Service{registry: registry}
	record := filepath.Join(home, createdProjectsFile)

	laid := filepath.Join(home, "projects", "seeded")
	if err := os.MkdirAll(laid, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(laid, "README.md"), []byte("# seeded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		out, err := m.RecordCreatedProject(ctx, api.RecordCreatedProjectInput{ProjectDir: laid})
		if err != nil || !out.Recorded {
			t.Fatalf("a laid-out folder is not recorded: %+v %v", out, err)
		}
	}
	if !createdOnThisPC(record, laid) {
		t.Fatalf("%s is not in the record %s", laid, record)
	}
	if n := len(readCreated(record).Projects); n != 1 {
		t.Errorf("the folder recorded twice: %d entries", n)
	}

	empty := filepath.Join(home, "projects", "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"projects/seeded", filepath.Join(home, "projects", "missing"), empty} {
		out, err := m.RecordCreatedProject(ctx, api.RecordCreatedProjectInput{ProjectDir: dir})
		if err == nil || out.Recorded {
			t.Errorf("%s was recorded: %+v", dir, out)
		} else if !strings.Contains(err.Error(), "record a created project") {
			t.Errorf("the refusal %q does not say what was refused", err)
		}
	}
	if n := len(readCreated(record).Projects); n != 1 {
		t.Errorf("a refused folder was recorded: %d entries", n)
	}
}
