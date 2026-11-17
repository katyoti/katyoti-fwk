// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// CREATED ON THIS PC. Until IN-1419 (securing third-party
// sidecars) is delivered, a universe attaches beside its other members only a
// repository its person created on this PC: a cloned repository's hooks,
// skills and tasks would otherwise be served beside the person's own,
// unchecked. The creation verbs of the orchestrator record the folder they
// made - project create, a universe created with its own project, the sample
// project - in the installation's creation record, beside the universes
// registry: machine-local state, never in the project (it is committed and
// cloned with it) nor in a universe's projects.yaml (a project created in one
// universe is attached to another by detach and attach). The record outlives
// the universe it was created in.

// createdProjectsFile is the creation record, beside universes.yaml.
const createdProjectsFile = "created-projects.yaml"

// createdProject is one folder a creation verb made on this PC.
type createdProject struct {
	Path string    `yaml:"path"`
	At   time.Time `yaml:"at"`
}

type createdProjects struct {
	Projects []createdProject `yaml:"projects"`
}

// createdMu serialises the record's read-modify-write within the process
// (the hub's orchestrator is its one writer).
var createdMu sync.Mutex

// createdRecordPath is the record beside the registry at regPath; beside the
// default registry when regPath is empty.
func createdRecordPath(regPath string) (string, error) {
	if regPath == "" {
		p, err := catalog.DefaultRegistryPath()
		if err != nil {
			return "", err
		}
		regPath = p
	}
	return filepath.Join(filepath.Dir(regPath), createdProjectsFile), nil
}

// createdPath is the record of this service's registry.
func (s *Service) createdPath() (string, error) {
	reg, err := s.registryOrLoad()
	if err != nil {
		return "", err
	}
	return createdRecordPath(reg.Path())
}

func readCreated(path string) createdProjects {
	var cp createdProjects
	data, err := os.ReadFile(path)
	if err != nil {
		return cp
	}
	_ = config.DecodeBytes(data, marshal.YAML, &cp)
	return cp
}

// recordCreated adds dir to the creation record at path; a folder recorded
// already is left as it is.
func recordCreated(path, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	createdMu.Lock()
	defer createdMu.Unlock()
	cp := readCreated(path)
	for _, c := range cp.Projects {
		if samePath(c.Path, abs) {
			return nil
		}
	}
	cp.Projects = append(cp.Projects, createdProject{Path: filepath.Clean(abs), At: time.Now().UTC()})
	data, err := yaml.Marshal(cp)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// recordCreatedHere records dir as created on this PC in this service's
// record. A failure to record is said, never fatal to the creation: the
// folder exists and is attached; only a later attach beside other members
// would be refused.
func (s *Service) recordCreatedHere(dir string) error {
	path, err := s.createdPath()
	if err != nil {
		return err
	}
	return recordCreated(path, dir)
}

// forgetCreatedHere drops dir from this service's record: a creation that
// removed the folder it made leaves no record a later clone at that place
// would pass by.
func (s *Service) forgetCreatedHere(dir string) {
	path, err := s.createdPath()
	if err != nil {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	createdMu.Lock()
	defer createdMu.Unlock()
	cp := readCreated(path)
	kept := cp.Projects[:0]
	for _, c := range cp.Projects {
		if !samePath(c.Path, abs) {
			kept = append(kept, c)
		}
	}
	if len(kept) == len(cp.Projects) {
		return
	}
	cp.Projects = kept
	if data, err := yaml.Marshal(cp); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
}

// createdOnThisPC reports whether the record at path lists dir.
func createdOnThisPC(path, dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	createdMu.Lock()
	defer createdMu.Unlock()
	for _, c := range readCreated(path).Projects {
		if samePath(c.Path, abs) {
			return true
		}
	}
	return false
}

// notCreatedHere says why the repository at abs cannot join the members the
// universe attaches already: no creation verb of this PC made it. Nil: it
// may join.
func (s *Service) notCreatedHere(universe, abs string) error {
	path, err := s.createdPath()
	if err != nil {
		return fmt.Errorf("attach: read the record of the projects created on this PC: %w", err)
	}
	if createdOnThisPC(path, abs) {
		return nil
	}
	return fmt.Errorf("attach: %s was not created on this PC by KaTyoti (project create, a universe created with its project, the sample project): until securing third-party sidecars is delivered, universe %q attaches beside its other members only a repository its person created here - a cloned repository's hooks, skills and tasks would be served unchecked; attach it alone in a universe of its own, or create the project here (katyoti project create) and bring its content into it",
		abs, universe)
}

// RecordCreatedProject records as created on this PC a project folder a
// creation path outside the orchestrator laid out - the marketplace's seed
// : the orchestrator stays the record's one writer. Narrow
// on purpose: an absolute path to an existing folder that holds files (the
// project just laid out), recorded only; nothing is attached. A folder
// recorded already is left as it is.
func (s *Service) RecordCreatedProject(ctx context.Context, in api.RecordCreatedProjectInput) (api.RecordCreatedProjectOutput, error) {
	out := api.RecordCreatedProjectOutput{ProjectDir: in.ProjectDir}
	dir := strings.TrimSpace(in.ProjectDir)
	if dir == "" || !filepath.IsAbs(dir) {
		return out, fmt.Errorf("record a created project: name its folder by an absolute path")
	}
	dir = filepath.Clean(dir)
	out.ProjectDir = dir
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return out, fmt.Errorf("record a created project: %s is not a folder of this PC", dir)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) == 0 {
		return out, fmt.Errorf("record a created project: %s holds no project laid out", dir)
	}
	if err := s.recordCreatedHere(dir); err != nil {
		return out, fmt.Errorf("record a created project: %w", err)
	}
	out.Recorded = true
	return out, nil
}

// ForgetCreatedProject drops a folder from this service's record - the other
// half of RecordCreatedProject for a creation path outside the orchestrator
// that recorded the folder before its attach (the setup's sample project,
// ) and whose attach failed. Narrow: an absolute path, dropped
// only; a folder the record does not name is left as it is.
func (s *Service) ForgetCreatedProject(ctx context.Context, in api.ForgetCreatedProjectInput) (api.ForgetCreatedProjectOutput, error) {
	out := api.ForgetCreatedProjectOutput{ProjectDir: in.ProjectDir}
	dir := strings.TrimSpace(in.ProjectDir)
	if dir == "" || !filepath.IsAbs(dir) {
		return out, fmt.Errorf("forget a created project: name its folder by an absolute path")
	}
	dir = filepath.Clean(dir)
	out.ProjectDir = dir
	path, err := s.createdPath()
	if err != nil {
		return out, fmt.Errorf("forget a created project: %w", err)
	}
	if !createdOnThisPC(path, dir) {
		return out, nil
	}
	s.forgetCreatedHere(dir)
	out.Forgotten = !createdOnThisPC(path, dir)
	return out, nil
}
