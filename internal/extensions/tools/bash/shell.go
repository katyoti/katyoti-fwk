// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/projectcmd"
)

// bashResult caches the outcome of FindBash so the search runs at most once.
var (
	bashOnce sync.Once
	bashPath string
	bashErr  error
)

// FindBash locates a bash executable. The result is memoized — the search
// runs only on the first call; subsequent calls return the cached path.
//
// Search order:
//  1. CLAUDE_CODE_SHELL / KTW_SHELL env override (if set and points to bash)
//  2. SHELL env (Unix only — if it contains "bash")
//  3. projectcmd.FindBash — the bash a project command runs through (on
//     Windows Git for Windows' bash, never the WSL launcher)
//
// If bash is not found, returns an error with installation instructions.
func FindBash() (string, error) {
	bashOnce.Do(func() {
		bashPath, bashErr = findBashImpl()
	})
	return bashPath, bashErr
}

// ResetBashCache clears the memoized bash path. Test-only.
func ResetBashCache() {
	bashOnce = sync.Once{}
	bashPath = ""
	bashErr = nil
}

func findBashImpl() (string, error) {
	// 1. Explicit override via env.
	for _, envKey := range []string{"KTW_SHELL", "CLAUDE_CODE_SHELL"} {
		if v := os.Getenv(envKey); v != "" {
			if isBashExecutable(v) {
				return v, nil
			}
		}
	}

	// 2. SHELL env (Unix convention).
	if shell := os.Getenv("SHELL"); shell != "" && isBashPath(shell) {
		if isExecutable(shell) {
			return shell, nil
		}
	}

	// 3. The bash project commands run through.
	if p, err := projectcmd.FindBash(); err == nil {
		return p, nil
	}

	return "", fmt.Errorf("bash not found — bash is required for the bash tool.\n%s",
		installHint(),
	)
}

// isBashPath checks if a path string looks like it points to bash.
func isBashPath(path string) bool {
	base := filepath.Base(path)
	return base == "bash" || base == "bash.exe"
}

// isBashExecutable checks if the path looks like bash AND is executable.
func isBashExecutable(path string) bool {
	return isBashPath(path) && isExecutable(path)
}

// isExecutable checks if a file exists and is executable.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}
	// On Windows, existence + .exe is enough.
	if runtime.GOOS == "windows" {
		return true
	}
	// On Unix, check executable bit.
	return info.Mode()&0o111 != 0
}

func installHint() string {
	if runtime.GOOS == "windows" {
		return "Install Git for Windows (https://git-scm.com) which provides both bash and git."
	}
	return "Install bash via your system package manager (e.g. apt install bash)."
}
