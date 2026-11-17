// SPDX-License-Identifier: Apache-2.0

// Package seed is a release's content: the project of the universe the release
// was snapshotted from — its objects, documents and files — and the universe's
// own tasks, packed beside the release so `bootstrap` gives a universe that is
// ready to use, not only wired.
//
// A seed carries what a person authored and nothing a machine made or a
// person must keep: never a secret (a `.env` file is refused, at publish and
// at install), never state that is rebuilt or belongs to one machine (a
// project's `.katyoti/` — its attachment — `.git/`, `node_modules/`, left out
// when packing and refused when found in a published seed). Links are refused:
// a seed is plain files.
//
// Layout of a seed (the folder packed, and the folder unpacked into the store):
//
//	seed.yaml          the marker: a complete seed
//	project/**         the project
//	project-seed.yaml  the project object's seed, when the project has none yet
//	universe/tasks/**  the universe's tasks
package seed

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
)

// The parts of a seed.
const (
	Marker      = "seed.yaml"
	ProjectPart = "project"
	TasksPart   = "universe/tasks"
	// ProjectSeedPart is the seed of the project object (cwmapi.ProjectSeed:
	// its type, title, groups) a seed carries when its project was published
	// before CWM ever opened it - so it has no project object yet, only the
	// seed in its store, which the packing leaves out with the store. At
	// install it goes back into the new project's store, and CWM mints the
	// project object from it at the first opening.
	ProjectSeedPart = "project-seed.yaml"
)

// markerBody is what the marker says: the layout's version.
const markerBody = "format: 1\n"

// left are the folders a project keeps that a seed never carries: derived or
// machine-specific (the project's attachment, its repository, its packages).
var left = []string{".katyoti", ".git", "node_modules"}

// secret is whether a file name is one a seed must never carry.
func secret(name string) bool {
	n := strings.ToLower(name)
	return n == ".env" || strings.HasPrefix(n, ".env.")
}

// Stage writes the seed of a project folder and a tasks folder into dst, as
// it is packed: the derived folders and what the folder's own .gitignore
// ignores left out, a secret or a link refused.
// tasksDir may be empty (a universe without tasks of its own).
func Stage(projectDir, tasksDir, dst string) (int, error) {
	if fi, err := os.Stat(projectDir); err != nil || !fi.IsDir() {
		return 0, fmt.Errorf("seed: project folder %s: not a folder", projectDir)
	}
	n, err := stagePart(projectDir, filepath.Join(dst, filepath.FromSlash(ProjectPart)))
	if err != nil {
		return 0, err
	}
	if tasksDir != "" {
		if _, err := os.Stat(tasksDir); err == nil {
			m, err := stagePart(tasksDir, filepath.Join(dst, filepath.FromSlash(TasksPart)))
			if err != nil {
				return 0, err
			}
			n += m
		}
	}
	return n, os.WriteFile(filepath.Join(dst, Marker), []byte(markerBody), 0o644)
}

// WithProjectSeed puts the seed of the project object (projectSeed, a file in
// the project's store) into a seed staged at dst, when the staged project
// carries no project object: one it carries is the project's, re-minted at
// install (Remint), and wins. No file at projectSeed: nothing to carry. It
// answers whether the seed now carries one.
func WithProjectSeed(dst, projectSeed string) (bool, error) {
	if projectSeed == "" || !exists(projectSeed) {
		return false, nil
	}
	if exists(filepath.Join(dst, filepath.FromSlash(ProjectPart), ProjectObject)) {
		return false, nil
	}
	if err := copyFile(projectSeed, filepath.Join(dst, ProjectSeedPart)); err != nil {
		return false, err
	}
	return true, nil
}

func stagePart(src, dst string) (int, error) {
	n := 0
	ignore := readIgnore(src)
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("seed: %s is a link — a seed is plain files", p)
		}
		if ignore.match(filepath.ToSlash(rel), d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if leftOut(d.Name()) {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if secret(d.Name()) {
			return fmt.Errorf("seed: %s holds secrets — a seed never carries one (remove it, or keep it out of the project)", p)
		}
		n++
		return copyFile(p, filepath.Join(dst, rel))
	})
	return n, err
}

func leftOut(dir string) bool {
	for _, l := range left {
		if strings.EqualFold(dir, l) {
			return true
		}
	}
	return false
}

// Check refuses a seed folder (a published seed, unpacked) that is not
// complete or carries what no seed may: a secret, a left-out folder, a link,
// or anything outside its two parts.
func Check(root string) error {
	if _, err := os.Stat(filepath.Join(root, Marker)); err != nil {
		return fmt.Errorf("seed: %s is not a complete seed (no %s)", root, Marker)
	}
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == Marker || (rel == ProjectSeedPart && !d.IsDir()) {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("seed: %s is a link", rel)
		}
		if !within(rel, ProjectPart) && !within(rel, TasksPart) && rel != "universe" {
			return fmt.Errorf("seed: %s is outside its project and tasks", rel)
		}
		if d.IsDir() && leftOut(d.Name()) {
			return fmt.Errorf("seed: %s is a folder a seed never carries", rel)
		}
		if !d.IsDir() && secret(d.Name()) {
			return fmt.Errorf("seed: %s holds secrets", rel)
		}
		return nil
	})
}

func within(rel, part string) bool { return rel == part || strings.HasPrefix(rel, part+"/") }

// Applied is what Apply did.
type Applied struct {
	// ProjectKept: the project folder already held files; it was left as it was.
	ProjectKept bool
	// Tasks are the task files written into the universe (one already there is
	// kept).
	Tasks int
	// TaskFiles are the paths of those task files: what to take back when the
	// project laid out is refused its universe.
	TaskFiles []string
	// ProjectSeed is the project object's seed the seed carries (a file in
	// the seed's folder), for the new project's store - "" when it carries
	// none, the project was kept, or the project laid out has its project
	// object.
	ProjectSeed string
}

// Apply lays a checked seed folder out: its project into projectDir — unless
// that folder already holds files, which are kept as they are (a second
// bootstrap repairs a universe, it never overwrites a person's project) — and
// its tasks into tasksDir, a task already there kept.
func Apply(root, projectDir, tasksDir string) (Applied, error) {
	var a Applied
	if err := Check(root); err != nil {
		return a, err
	}
	if holdsFiles(projectDir) {
		a.ProjectKept = true
	} else if _, err := copyTree(filepath.Join(root, filepath.FromSlash(ProjectPart)), projectDir, false); err != nil {
		return a, fmt.Errorf("seed: project: %w", err)
	} else if ps := filepath.Join(root, ProjectSeedPart); exists(ps) && !exists(filepath.Join(projectDir, ProjectObject)) {
		a.ProjectSeed = ps
	}
	if src := filepath.Join(root, filepath.FromSlash(TasksPart)); exists(src) {
		written, err := copyTree(src, tasksDir, true)
		if err != nil {
			return a, fmt.Errorf("seed: tasks: %w", err)
		}
		a.Tasks, a.TaskFiles = len(written), written
	}
	return a, nil
}

// holdsFiles is whether a folder exists and holds anything.
func holdsFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// copyTree copies src into dst; keepExisting leaves a file already at its
// destination alone. It answers the files written.
func copyTree(src, dst string, keepExisting bool) ([]string, error) {
	var written []string
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if keepExisting && exists(target) {
			return nil
		}
		written = append(written, target)
		return copyFile(p, target)
	})
	return written, err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// ProjectObject is the project object's file at a project's root (CWM's
// `project.cwm.yaml`).
const ProjectObject = projectfile.FileName

var (
	idLine   = regexp.MustCompile(`(?m)^id:[ \t]*.*$`)
	uuidLine = regexp.MustCompile(`(?m)^([ \t]+)uuid:[ \t]*["']?[0-9a-fA-F-]{36}["']?[ \t]*$`)
)

// Remint gives a laid-out project its own identity: the project object a seed
// carries is the SOURCE's — its uuid is the project's one computer id, minted
// once — so every project applied from one seed would share it. The id becomes
// the project folder's name and the uuid a fresh one; everything else in the
// object is kept as it is. A project object that does not have the shape CWM
// writes is removed instead: CWM gives the project a new one when it opens it.
// It answers the project's id ("" when the seed carried no project object).
func Remint(projectDir string) (string, error) {
	p := filepath.Join(projectDir, ProjectObject)
	raw, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	name := filepath.Base(filepath.Clean(projectDir))
	text := string(raw)
	if len(idLine.FindAllStringIndex(text, -1)) != 1 || len(uuidLine.FindAllStringIndex(text, -1)) != 1 {
		return "", os.Remove(p)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	text = idLine.ReplaceAllLiteralString(text, "id: "+name)
	text = uuidLine.ReplaceAllString(text, "${1}uuid: "+id.String())
	return name, os.WriteFile(p, []byte(text), 0o644)
}
