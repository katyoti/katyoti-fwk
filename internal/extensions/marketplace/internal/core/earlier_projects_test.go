// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// : a new installation finds the projects an earlier one serves
// (the registry of installations, its universes, their projects registries)
// and imports one as a copy in its projects folder, migrated on the copy only:
// the original is byte-identical after a success and after a failed
// migration, which leaves no copy behind.
func TestAnEarlierProjectIsImportedAsAMigratedCopyAndTheOriginalIsNeverWritten(t *testing.T) {
	dir := t.TempDir()
	original, current := earlierWorld(t, dir)
	// Its store last changed at a known time: that is its last use.
	used := time.Date(2026, 3, 14, 9, 26, 53, 0, time.UTC)
	if err := os.MkdirAll(application.EnvOf(original).HomeDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = filepath.WalkDir(application.EnvOf(original).HomeDir(), func(p string, _ fs.DirEntry, err error) error {
		if err == nil {
			_ = os.Chtimes(p, used, used)
		}
		return nil
	})
	before := treeHash(t, original)
	ctx := context.Background()
	s := &service{}

	list, err := s.EarlierProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Projects) != 1 || !sameFolder(list.Projects[0].Folder, original) ||
		list.Projects[0].Installation != "old" || list.Projects[0].Universe != "work" ||
		list.Projects[0].DataVersion != 1 || list.Projects[0].ImportedTo != "" {
		t.Fatalf("earlier projects = %+v, want the old installation's one project at data version 1", list)
	}
	if got := list.Projects[0].LastUsed; got != used.Format(time.RFC3339) {
		t.Fatalf("last use = %q, want %s - its store's newest change", got, used.Format(time.RFC3339))
	}

	// A failed migration: no copy, the original untouched.
	old := migrateCopy
	migrateCopy = func(context.Context, string) (int, int, error) { return 1, 0, errors.New("step 3 broke") }
	_, err = s.ImportProject(ctx, mktapi.ImportProjectInput{Folder: original})
	migrateCopy = old
	if err == nil || !strings.Contains(err.Error(), "the original is untouched") {
		t.Fatalf("import with a failing migration = %v, want it refused", err)
	}
	projects, _ := current.ProjectsFolder()
	dst := filepath.Join(projects, "acme")
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("a failed migration left the copy %s (%v)", dst, err)
	}
	if got := treeHash(t, original); got != before {
		t.Fatal("a failed import changed the original")
	}

	// A success: the copy is in the projects folder, at this release's data
	// version; the original is byte-identical.
	out, err := s.ImportProject(ctx, mktapi.ImportProjectInput{Folder: original})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if !sameFolder(out.Project, dst) || out.From != 1 || out.To <= 1 {
		t.Fatalf("import = %+v, want the copy in %s migrated from data version 1", out, dst)
	}
	f, err := projectfile.Read(dst)
	if err != nil || f.DataVersion != out.To {
		t.Fatalf("the copy's data version = %d (%v), want %d", f.DataVersion, err, out.To)
	}
	if _, err := os.Stat(filepath.Join(dst, "modules", "app", "notes.md")); err != nil {
		t.Fatalf("the copy misses a file: %v", err)
	}
	if got := treeHash(t, original); got != before {
		t.Fatal("the import wrote the original")
	}

	// Listed as imported; importing again never overwrites the copy.
	list, _ = s.EarlierProjects(ctx)
	if len(list.Projects) != 1 || !sameFolder(list.Projects[0].ImportedTo, dst) {
		t.Fatalf("after the import: %+v, want it marked imported to %s", list.Projects, dst)
	}
	if _, err := s.ImportProject(ctx, mktapi.ImportProjectInput{Folder: original}); err == nil {
		t.Fatal("a second import overwrote the copy")
	}
}

// earlierWorld lays out an earlier installation serving one project through
// a universe, and the current installation with its projects folder; it
// answers the original project's folder and the current installation.
func earlierWorld(t *testing.T, dir string) (string, *cenv.Installation) {
	t.Helper()
	earlier := cenv.NewInstallation(filepath.Join(dir, "old"), "old", "")
	current := cenv.NewInstallation(filepath.Join(dir, "new"), "new", "")
	current.Settings = map[string]any{cenv.ProjectsFolderSetting: filepath.Join(dir, "Documents", "Katyoti")}
	for _, inst := range []*cenv.Installation{earlier, current} {
		mkdir(t, inst.Dir())
		if err := cenv.SaveInstallation(inst); err != nil {
			t.Fatal(err)
		}
	}
	project := filepath.Join(dir, "old-projects", "acme")
	write(t, filepath.Join(project, projectfile.FileName), "id: acme\ntype: project\nname: acme\n")
	write(t, filepath.Join(project, "modules", "app", "notes.md"), "# notes\n")
	home := filepath.Join(dir, "old-universes", "work")
	write(t, filepath.Join(application.EnvOf(home).HomeDir(), cenv.AttachedProjectsFile), fmt.Sprintf("projects:\n  - %q\n", project))
	write(t, filepath.Join(earlier.ConfigRoot(), "universes.yaml"), fmt.Sprintf("universes:\n  - name: work\n    home_path: %q\n", home))

	reg := filepath.Join(dir, "katyoti-instances.yaml")
	write(t, reg, fmt.Sprintf("instances:\n  - name: old\n    folder: %q\n  - name: new\n    folder: %q\n", earlier.Dir(), current.Dir()))
	t.Setenv(controlplane.InstallationsEnv, reg)
	oldCur := currentInstallation
	currentInstallation = func() *cenv.Installation { return current }
	t.Cleanup(func() { currentInstallation = oldCur })
	return project, current
}

// treeHash is one hash of every path and mode under dir, and each file's
// time and bytes.
func treeHash(t *testing.T, dir string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		line := fmt.Sprintf("%s %v", filepath.ToSlash(rel), info.Mode())
		if !d.IsDir() { // a folder's time is the file system's: NTFS moves it late
			line += " " + info.ModTime().UTC().String()
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			line += fmt.Sprintf(" %x", sha256.Sum256(b))
		}
		lines = append(lines, line)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(lines, "\n"))))
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// : an import's name is one folder name, and its destination one
// the installation accepts - nothing is copied otherwise, inside or outside
// the projects folder.
func TestAnImportRefusesANameThatIsNotOneFolderName(t *testing.T) {
	dir := t.TempDir()
	original, current := earlierWorld(t, dir)
	ctx := context.Background()
	s := &service{}
	outside := filepath.Join(dir, "outside")
	for _, name := range []string{"..", ".", "../outside", `..\outside`, "a/b", outside} {
		if _, err := s.ImportProject(ctx, mktapi.ImportProjectInput{Folder: original, Name: name}); err == nil {
			t.Errorf("import named %q was accepted", name)
		}
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("a refused name left a copy at %s (%v)", outside, err)
	}
	projects, _ := current.ProjectsFolder()
	if entries, _ := os.ReadDir(projects); len(entries) != 0 {
		t.Fatalf("a refused name left %d entries in the projects folder", len(entries))
	}

	// A projects folder inside the installation: the destination is refused.
	current.Settings = map[string]any{cenv.ProjectsFolderSetting: filepath.Join(current.Dir(), "projects")}
	if _, err := s.ImportProject(ctx, mktapi.ImportProjectInput{Folder: original}); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Fatalf("import into the installation's folder = %v, want it refused", err)
	}
	if _, err := os.Stat(filepath.Join(current.Dir(), "projects", "acme")); !os.IsNotExist(err) {
		t.Fatalf("a refused destination was copied to (%v)", err)
	}
}
