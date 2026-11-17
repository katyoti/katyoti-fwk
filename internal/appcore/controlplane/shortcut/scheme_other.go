// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package shortcut

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// RegisterScheme makes scheme open target for the person: on Linux a desktop
// entry handling x-scheme-handler/<scheme> (Exec `"<target>" gui --open "%u"`)
// in the applications folder (KTW_SHORTCUT_DIR, else XDG_DATA_HOME, else
// ~/.local/share/applications), made the default with xdg-mime when it is
// there. macOS takes it from an app bundle: nothing is written. Answers what
// it wrote.
func RegisterScheme(scheme, target string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", nil
	}
	dir := os.Getenv(DirEnv)
	if dir == "" {
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".local", "share")
		}
		dir = filepath.Join(base, "applications")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := scheme + "-link.desktop"
	body := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=KaTyoti link\nNoDisplay=true\nExec=%s\nMimeType=x-scheme-handler/%s;\n",
		linkCommand(target, "%u"), scheme)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	if os.Getenv(DirEnv) == "" {
		if xdg, err := exec.LookPath("xdg-mime"); err == nil {
			xc := exec.Command(xdg, "default", name, "x-scheme-handler/"+scheme)
			publish.NoConsoleWindow(xc)
			_ = xc.Run()
		}
	}
	return path, nil
}
