// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// Workspace folder resolution. The `.katyoti` folder name lives in the
// application package (private const) and is composed via
// `application.EnvOf(root).HomeDir()`.
const (
	// EnvVarWorkspaceDir overrides the discovered workspace folder
	// path. When set, the value is treated as the absolute path to
	// the workspace marker folder (e.g. /home/me/proj/.katyoti).
	EnvVarWorkspaceDir = "KTW_WORKSPACE_DIR"

	// envVarWorkspaceFolder is the workspace FOLDER the rest of the runtime
	// resolves config, data and cache from (CLAUDE.md: "KTW_WORKSPACE
	// (workspace folder)"). CWM honours it so a process
	// told where its workspace is does not silently bind to another.
	envVarWorkspaceFolder = "KTW_WORKSPACE"
)

// Source identifies which precedence branch resolved the workspace
// root. Useful for diagnostics and lint.
type Source string

const (
	SourceFlag     Source = "flag"
	SourceEnv      Source = "env"
	SourceKatyoti  Source = "katyoti"
	SourceFirstRun Source = "first-run"
)

// WorkspaceRoot is the resolved workspace folder. Path is always
// absolute. FirstRun signals that the folder does not exist yet —
// the caller (the cwm bootstrap flow) is responsible for creating
// it.
type WorkspaceRoot struct {
	Path     string
	Source   Source
	FirstRun bool
}

// DiscoverOptions configures DiscoverWorkspaceRoot. All fields are
// optional. Tests inject Stat to keep the resolver hermetic.
type DiscoverOptions struct {
	// Explicit is the CLI flag value (highest precedence). Empty
	// means the flag was not set.
	Explicit string

	// Env is a pre-resolved env-var value. Empty means "read
	// os.Getenv(EnvVarWorkspaceDir)" at call time.
	Env string

	// CWD is the directory to start the walk-up from. Empty means
	// "use os.Getwd() at call time".
	CWD string

	// Stat overrides os.Stat for testing. nil means use os.Stat.
	Stat func(string) (os.FileInfo, error)
}

// DiscoverWorkspaceRoot resolves the workspace folder following
// this precedence (highest wins):
//
//  1. opts.Explicit (CLI flag).
//  2. opts.Env (or os.Getenv(EnvVarWorkspaceDir) when opts.Env empty).
//  3. KTW_WORKSPACE — the workspace FOLDER; its workspace dir is used.
//  4. nearest ancestor of CWD containing `.katyoti`.
//  5. first-run: returns `<CWD>/.katyoti` with FirstRun=true.
//
// The resolver has no filesystem side effects beyond reading
// directory entries via Stat. Returned Path is always absolute.
func DiscoverWorkspaceRoot(opts DiscoverOptions) (WorkspaceRoot, error) {
	stat := opts.Stat
	if stat == nil {
		stat = os.Stat
	}

	if opts.Explicit != "" {
		abs, err := filepath.Abs(opts.Explicit)
		if err != nil {
			return WorkspaceRoot{}, fmt.Errorf("workspace flag %q: %w", opts.Explicit, err)
		}
		return WorkspaceRoot{Path: abs, Source: SourceFlag}, nil
	}

	envVal := opts.Env
	if envVal == "" {
		envVal = os.Getenv(EnvVarWorkspaceDir)
	}
	if envVal != "" {
		abs, err := filepath.Abs(envVal)
		if err != nil {
			return WorkspaceRoot{}, fmt.Errorf("%s=%q: %w", EnvVarWorkspaceDir, envVal, err)
		}
		return WorkspaceRoot{Path: abs, Source: SourceEnv}, nil
	}

	// The WORKSPACE FOLDER, when the process was told one. KTW_WORKSPACE names
	// the folder that CONTAINS the workspace dir; KTW_WORKSPACE_DIR above names
	// the dir itself, so the specific one still wins.
	//
	// Without this, a process told KTW_WORKSPACE had its config, data and cache
	// resolved there while CWM walked up from the CURRENT DIRECTORY and bound
	// itself to a different workspace entirely — one process, two workspaces,
	// and the CWM lock taken on whichever one the caller happened to be
	// standing in. Under `go test ./...` that was always the repo's own
	// .katyoti, so every package that builds an application contended for a
	// single lock and the failures read as "held by another process".
	if ws := os.Getenv(envVarWorkspaceFolder); ws != "" {
		abs, err := filepath.Abs(ws)
		if err != nil {
			return WorkspaceRoot{}, fmt.Errorf("%s=%q: %w", envVarWorkspaceFolder, ws, err)
		}
		home := application.EnvOf(abs).HomeDir()
		return WorkspaceRoot{Path: home, Source: SourceEnv}, nil
	}

	cwd := opts.CWD
	if cwd == "" {
		c, err := os.Getwd()
		if err != nil {
			return WorkspaceRoot{}, fmt.Errorf("getwd: %w", err)
		}
		cwd = c
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return WorkspaceRoot{}, fmt.Errorf("abs %q: %w", cwd, err)
	}

	if r, found := walkUp(cwd, stat); found {
		return r, nil
	}
	return WorkspaceRoot{
		Path:     application.EnvOf(cwd).HomeDir(),
		Source:   SourceFirstRun,
		FirstRun: true,
	}, nil
}

// walkUp walks from cwd toward the filesystem root, returning at
// the first ancestor that contains a `.katyoti/` workspace folder
// (an empty one means first-run-needs-init).
func walkUp(cwd string, stat func(string) (os.FileInfo, error)) (WorkspaceRoot, bool) {
	dir := cwd
	for {
		katyoti := application.EnvOf(dir).HomeDir()
		if isDir(stat, katyoti) {
			return WorkspaceRoot{Path: katyoti, Source: SourceKatyoti}, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return WorkspaceRoot{}, false
		}
		dir = parent
	}
}

func isDir(stat func(string) (os.FileInfo, error), path string) bool {
	info, err := stat(path)
	return err == nil && info.IsDir()
}
