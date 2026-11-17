// SPDX-License-Identifier: Apache-2.0

package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// fakeInfo is a minimal fs.FileInfo for our Stat stub. isDir
// distinguishes directory fixtures from file fixtures (the
// workspace marker file is a regular file).
type fakeInfo struct {
	name  string
	isDir bool
}

func (f fakeInfo) Name() string { return f.name }
func (fakeInfo) Size() int64    { return 0 }
func (f fakeInfo) Mode() os.FileMode {
	if f.isDir {
		return os.ModeDir | 0o755
	}
	return 0o644
}
func (fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool      { return f.isDir }
func (fakeInfo) Sys() any           { return nil }

// makeStat returns a Stat fake where the listed paths exist as
// directories and everything else returns os.ErrNotExist.
//
// Paths are absolutized with filepath.Abs so unix-style fixtures
// match the resolver's normalization on Windows (drive letters)
// as well as POSIX.
func makeStat(existingDirs ...string) func(string) (os.FileInfo, error) {
	return makeStatWithFiles(existingDirs, nil)
}

// makeStatWithFiles is the dir+file variant. Dirs return IsDir=true;
// files return IsDir=false. All other paths return os.ErrNotExist.
func makeStatWithFiles(dirs, files []string) func(string) (os.FileInfo, error) {
	dirSet := make(map[string]struct{}, len(dirs))
	for _, p := range dirs {
		dirSet[absOrPanic(p)] = struct{}{}
	}
	fileSet := make(map[string]struct{}, len(files))
	for _, p := range files {
		fileSet[absOrPanic(p)] = struct{}{}
	}
	return func(path string) (os.FileInfo, error) {
		abs := absOrPanic(path)
		if _, ok := dirSet[abs]; ok {
			return fakeInfo{name: filepath.Base(path), isDir: true}, nil
		}
		if _, ok := fileSet[abs]; ok {
			return fakeInfo{name: filepath.Base(path), isDir: false}, nil
		}
		return nil, os.ErrNotExist
	}
}

func absOrPanic(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		panic(err)
	}
	return abs
}

func TestDiscoverWorkspaceRoot_ExplicitFlagWins(t *testing.T) {
	r, err := DiscoverWorkspaceRoot(DiscoverOptions{
		Explicit: application.EnvOf("/some/abs/path").HomeDir(),
		Env:      "/should/be/ignored",
		CWD:      "/should/also/be/ignored",
		Stat:     makeStat(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Source != SourceFlag {
		t.Errorf("Source = %q, want %q", r.Source, SourceFlag)
	}
	if !filepath.IsAbs(r.Path) {
		t.Errorf("Path %q is not absolute", r.Path)
	}
}

func TestDiscoverWorkspaceRoot_EnvWinsWhenNoFlag(t *testing.T) {
	r, err := DiscoverWorkspaceRoot(DiscoverOptions{
		Env:  "/env/path/.katyoti",
		CWD:  "/should/be/ignored",
		Stat: makeStat(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Source != SourceEnv {
		t.Errorf("Source = %q, want %q", r.Source, SourceEnv)
	}
}

func TestDiscoverWorkspaceRoot_KatyotiInCWD(t *testing.T) {
	clearWorkspaceEnv(t)
	cwd := absOrPanic("/proj/repo")
	katyoti := application.EnvOf(cwd).HomeDir()
	r, err := DiscoverWorkspaceRoot(DiscoverOptions{
		CWD:  cwd,
		Stat: makeStat(katyoti),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Source != SourceKatyoti {
		t.Errorf("Source = %q, want %q", r.Source, SourceKatyoti)
	}
	if r.Path != katyoti {
		t.Errorf("Path = %q, want %q", r.Path, katyoti)
	}
}

func TestDiscoverWorkspaceRoot_KatyotiInAncestor(t *testing.T) {
	clearWorkspaceEnv(t)
	cwd := absOrPanic("/proj/repo/sub/deep")
	katyoti := application.EnvOf(absOrPanic("/proj/repo")).HomeDir()
	r, err := DiscoverWorkspaceRoot(DiscoverOptions{
		CWD:  cwd,
		Stat: makeStat(katyoti),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Source != SourceKatyoti {
		t.Errorf("Source = %q, want %q", r.Source, SourceKatyoti)
	}
	if r.Path != katyoti {
		t.Errorf("Path = %q, want %q", r.Path, katyoti)
	}
}

func TestDiscoverWorkspaceRoot_FirstRun(t *testing.T) {
	clearWorkspaceEnv(t)
	cwd := absOrPanic("/empty/project")
	r, err := DiscoverWorkspaceRoot(DiscoverOptions{
		CWD:  cwd,
		Stat: makeStat(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Source != SourceFirstRun {
		t.Errorf("Source = %q, want %q", r.Source, SourceFirstRun)
	}
	if !r.FirstRun {
		t.Errorf("FirstRun = false, want true")
	}
	want := application.EnvOf(cwd).HomeDir()
	if r.Path != want {
		t.Errorf("Path = %q, want %q", r.Path, want)
	}
}

// clearWorkspaceEnv empties the workspace variables a hub or a shell may
// export, so a test of the walk-up reads only its own options.
func clearWorkspaceEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvVarWorkspaceDir, "")
	t.Setenv(envVarWorkspaceFolder, "")
}
