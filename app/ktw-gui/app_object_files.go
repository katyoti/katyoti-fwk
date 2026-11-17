// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// WorkspaceObjectFileOpen opens one of an object's module files with the
// application the system associates with it — the Files tab's « open the
// file », for a file no display engine shows well. The path is
// module-relative and goes through CWM's module file API (rooted at the
// module, `..` refused), which answers it project-relative; it is anchored on
// the universe's attached project, as `${node.module_dir}` is. Answers an
// error text, empty on success.
func (a *App) WorkspaceObjectFileOpen(universeName, objectID, path string) string {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return fmt.Sprintf("cannot reach universe %q: %v", universeName, err)
	}
	abs, err := objectFileAbs(a.ctx, sender, a.projectRootsOf(universeName), universeName, objectID, path)
	if err != nil {
		return err.Error()
	}
	if _, err := os.Stat(abs); err != nil {
		return err.Error()
	}
	if err := openWithSystem(abs); err != nil {
		return err.Error()
	}
	return ""
}

// objectFileAbs is where one of an object's module files is on disk: CWM's
// module file API answers its project-relative path (`..` refused), anchored
// on the universe's project roots. A folder, or a file under no root, is an
// error.
func objectFileAbs(ctx context.Context, sender ktsp.Sender, roots []string, universeName, objectID, path string) (string, error) {
	raw, err := sender.Call(ctx, "ktw-agents:cwm/object-file-stat", cwmapi.ObjectFileStatInput{ID: objectID, Path: path})
	if err != nil {
		return "", err
	}
	var st cwmapi.ObjectFileStatOutput
	if err := json.Unmarshal(raw, &st); err != nil {
		return "", fmt.Errorf("decode ObjectFileStat: %v", err)
	}
	if st.Entry.IsDir {
		return "", fmt.Errorf("%s is a folder", path)
	}
	abs := resolveProjectRelative(filepath.FromSlash(st.ProjectPath), roots)
	if !filepath.IsAbs(abs) {
		return "", fmt.Errorf("%s is under no project root of universe %q", st.ProjectPath, universeName)
	}
	return abs, nil
}

// openWithSystem hands a file to the application the system associates with
// its type.
func openWithSystem(abs string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", abs).Start()
	case "darwin":
		return exec.Command("open", abs).Start()
	default:
		return exec.Command("xdg-open", abs).Start()
	}
}
