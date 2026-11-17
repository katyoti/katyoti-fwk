// SPDX-License-Identifier: Apache-2.0

package projectcmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// ErrNoBash is why FindBash found no bash: a command then does not run
// (- never through another shell).
var ErrNoBash = func() error {
	if runtime.GOOS == "windows" {
		return errors.New("no bash found: install Git for Windows (https://git-scm.com)")
	}
	return errors.New("no bash found: install bash with the system's package manager")
}()

// FindBash is the bash every command runs through, looked up once. On Windows
// it is Git for Windows' bash — found from git (its exec path, or git.exe on
// PATH), else a bash on PATH, else Git's usual install folders — and never the
// WSL launcher (System32\bash.exe, WindowsApps\bash.exe), which would run the
// command in Linux, away from the project's tools. Of a Git install it takes
// bin\bash.exe, the launcher that puts Git's own tools (grep, sed, head) on the
// PATH, before usr\bin\bash.exe. Elsewhere it is bash on PATH or in /bin,
// /usr/bin, /usr/local/bin. ErrNoBash when there is none.
var FindBash = sync.OnceValues(func() (string, error) {
	if runtime.GOOS == "windows" {
		return findWindowsBash()
	}
	if p, err := exec.LookPath("bash"); err == nil {
		return p, nil
	}
	for _, p := range []string{"/bin/bash", "/usr/bin/bash", "/usr/local/bin/bash", "/opt/homebrew/bin/bash"} {
		if isFile(p) {
			return p, nil
		}
	}
	return "", ErrNoBash
})

func findWindowsBash() (string, error) {
	// Git's own folders first: its exec path (…\mingw64\libexec\git-core) and
	// git.exe on PATH (…\cmd\git.exe) both sit inside its install.
	if out, err := exec.Command("git", "--exec-path").Output(); err == nil {
		if p := gitBashAbove(strings.TrimSpace(string(out))); p != "" {
			return p, nil
		}
	}
	if git, err := exec.LookPath("git"); err == nil {
		if p := gitBashAbove(filepath.Dir(git)); p != "" {
			return p, nil
		}
	}
	// A bash on PATH that is not the WSL launcher: Git's when it is one (its
	// launcher preferred), else that bash (MSYS2, Cygwin).
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, "bash.exe")
		if dir == "" || isWSLLauncher(p) || !isFile(p) {
			continue
		}
		if g := gitBashAbove(dir); g != "" {
			return g, nil
		}
		return p, nil
	}
	var roots []string
	for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if v := os.Getenv(env); v != "" {
			if env == "LOCALAPPDATA" {
				v = filepath.Join(v, "Programs")
			}
			roots = append(roots, filepath.Join(v, "Git"))
		}
	}
	for _, root := range append(roots, `C:\Program Files\Git`) {
		if p := gitBashAbove(root); p != "" {
			return p, nil
		}
	}
	return "", ErrNoBash
}

// gitBashAbove walks up from dir to the Git install holding it — the folder
// with usr\bin\bash.exe — and answers its bin\bash.exe (else usr\bin\bash.exe);
// "" when dir is in no Git install.
func gitBashAbove(dir string) string {
	if dir == "" {
		return ""
	}
	d := filepath.Clean(filepath.FromSlash(dir))
	for i := 0; i < 5; i++ {
		if isFile(filepath.Join(d, "usr", "bin", "bash.exe")) {
			if p := filepath.Join(d, "bin", "bash.exe"); isFile(p) && !isWSLLauncher(p) {
				return p
			}
			return filepath.Join(d, "usr", "bin", "bash.exe")
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return ""
}

// isWSLLauncher says whether p is Windows' bash.exe, which starts WSL: one in
// the Windows folder or in the Store's WindowsApps.
func isWSLLauncher(p string) bool {
	lower := strings.ToLower(filepath.Clean(p))
	if root := os.Getenv("SystemRoot"); root != "" && strings.HasPrefix(lower, strings.ToLower(filepath.Clean(root))+`\`) {
		return true
	}
	return strings.HasPrefix(lower, `c:\windows\`) || strings.Contains(lower, `\windowsapps\`)
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
