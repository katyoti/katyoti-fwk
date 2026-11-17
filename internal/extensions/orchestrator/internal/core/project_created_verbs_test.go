// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// createdVerbsService is an orchestrator over a registry in a temporary home,
// with the record of the projects created on this PC beside it.
func createdVerbsService(t *testing.T) (m *Service, home, record string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	isolateDataDir(t)
	registry, _ := catalog.LoadRegistry(filepath.Join(home, "universes.yaml"))
	return &Service{registry: registry}, home, filepath.Join(home, createdProjectsFile)
}

// A UNIVERSE CREATED WITH ITS PROJECT RECORDS ONLY A FOLDER IT MADE
// : universe create with a new project folder
// records it as created on this PC; with a folder that exists already it
// attaches it unrecorded; an attach that fails leaves no record - a clone
// later put at that place would otherwise join beside members.
func TestAUniverseCreatedWithItsProjectRecordsOnlyAFolderItMade(t *testing.T) {
	ctx := context.Background()
	m, home, record := createdVerbsService(t)

	made := filepath.Join(home, "work", "made")
	if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: "made", HomePath: filepath.Join(home, "universes", "made"), ProjectDir: made}); err != nil {
		t.Fatal(err)
	}
	if !createdOnThisPC(record, made) {
		t.Errorf("the project folder universe create made, %s, is not recorded", made)
	}

	existing := filepath.Join(home, "work", "existing")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: "existing", HomePath: filepath.Join(home, "universes", "existing"), ProjectDir: existing}); err != nil {
		t.Fatal(err)
	}
	if createdOnThisPC(record, existing) {
		t.Errorf("a folder that existed before universe create, %s, is recorded as created here", existing)
	}

	// A folder inside a Git repository: the attach is refused.
	if err := os.MkdirAll(filepath.Join(home, "work", "outer", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	refused := filepath.Join(home, "work", "outer", "inner")
	if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: "refused", HomePath: filepath.Join(home, "universes", "refused"), ProjectDir: refused}); err == nil {
		t.Fatalf("universe create attached %s, inside a Git repository", refused)
	}
	if createdOnThisPC(record, refused) {
		t.Errorf("a project folder whose attach failed, %s, is left in the record", refused)
	}
	if n := len(readCreated(record).Projects); n != 1 {
		t.Errorf("the record holds %d folders; want only %s", n, made)
	}
}

// THE SAMPLE PROJECT IS RECORDED IN ITS ORCHESTRATOR'S RECORD BEFORE ITS
// ATTACH: the sample the setup writes is
// recorded as created on this PC in the record of the orchestrator that
// attaches it, before the attach, so it joins a universe that holds a member
// already; an attach that fails leaves neither the record nor the sample, and
// a sample written before (a folder that existed) is reused unrecorded.
func TestTheSampleProjectIsRecordedInItsOrchestratorsRecordBeforeItsAttach(t *testing.T) {
	ctx := context.Background()
	m, home, record := createdVerbsService(t)
	if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: "main", HomePath: filepath.Join(home, "universes", "main")}); err != nil {
		t.Fatal(err)
	}
	s := SetupOver(m, func() (string, error) { return home, nil })

	sample := filepath.Join(home, "work", "sample")
	if r, err := s.CreateSampleProject(ctx, guideapi.CheckInput{Universe: "main", With: map[string]string{"folder": sample}}); err != nil || !r.OK {
		t.Fatalf("sample: %+v %v", r, err)
	}
	if !createdOnThisPC(record, sample) {
		t.Errorf("the sample written and attached, %s, is not in its orchestrator's record %s", sample, record)
	}

	// Beside an existing member: a universe created with its project holds
	// one; the sample written here joins it.
	if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: "withproject", HomePath: filepath.Join(home, "universes", "withproject"), ProjectDir: filepath.Join(home, "work", "own")}); err != nil {
		t.Fatal(err)
	}
	beside := filepath.Join(home, "work", "beside")
	if r, err := s.CreateSampleProject(ctx, guideapi.CheckInput{Universe: "withproject", With: map[string]string{"folder": beside}}); err != nil || !r.OK {
		t.Fatalf("a sample beside an existing member is refused: %+v %v", r, err)
	}
	if !createdOnThisPC(record, beside) {
		t.Errorf("the sample attached beside a member, %s, is not in the record", beside)
	}

	ghost := filepath.Join(home, "work", "ghost")
	if _, err := s.CreateSampleProject(ctx, guideapi.CheckInput{Universe: "nowhere", With: map[string]string{"folder": ghost}}); err == nil {
		t.Fatal("a sample was attached to a universe that is not registered")
	}
	if createdOnThisPC(record, ghost) {
		t.Errorf("a sample whose attach failed, %s, is left in the record", ghost)
	}
	if _, err := os.Stat(ghost); !os.IsNotExist(err) {
		t.Errorf("a sample whose attach failed, %s, is left on disk (%v): a second run would reuse it unrecorded", ghost, err)
	}

	before := filepath.Join(home, "work", "before")
	if err := writeSample(before); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateUniverse(ctx, api.CreateUniverseInput{Name: "other", HomePath: filepath.Join(home, "universes", "other")}); err != nil {
		t.Fatal(err)
	}
	if r, err := s.CreateSampleProject(ctx, guideapi.CheckInput{Universe: "other", With: map[string]string{"folder": before}}); err != nil || !r.OK {
		t.Fatalf("reuse: %+v %v", r, err)
	}
	if createdOnThisPC(record, before) {
		t.Errorf("a sample folder that existed before the setup, %s, is recorded as created by it", before)
	}
}

// FORGET A CREATED PROJECT DROPS ONLY WHAT THE RECORD NAMES:
// the narrow other half of RecordCreatedProject - a relative path is refused,
// a folder the record does not name is left as it is, a recorded one dropped.
func TestForgetCreatedProjectDropsARecordedFolder(t *testing.T) {
	ctx := context.Background()
	m, home, record := createdVerbsService(t)
	laid := filepath.Join(home, "work", "laid")
	if err := writeSample(laid); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ForgetCreatedProject(ctx, api.ForgetCreatedProjectInput{ProjectDir: "work/laid"}); err == nil {
		t.Error("a relative path was accepted")
	}
	if out, err := m.ForgetCreatedProject(ctx, api.ForgetCreatedProjectInput{ProjectDir: laid}); err != nil || out.Forgotten {
		t.Errorf("a folder the record does not name: %+v %v; want nothing forgotten", out, err)
	}
	if _, err := m.RecordCreatedProject(ctx, api.RecordCreatedProjectInput{ProjectDir: laid}); err != nil {
		t.Fatal(err)
	}
	if out, err := m.ForgetCreatedProject(ctx, api.ForgetCreatedProjectInput{ProjectDir: laid}); err != nil || !out.Forgotten {
		t.Errorf("a recorded folder: %+v %v; want it forgotten", out, err)
	}
	if createdOnThisPC(record, laid) {
		t.Errorf("%s is still in the record", laid)
	}
}
