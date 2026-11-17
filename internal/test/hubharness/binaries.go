// SPDX-License-Identifier: Apache-2.0

package hubharness

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
)

// A test never runs a program from the checkout's bin/: a running program
// locks its file, so `make build` / `make dev` could not replace it while the
// test runs (and an orphan of a killed test locked it for good). It runs the
// test process's own copy instead, in a run folder of its own:
//
//	<tmp>/ktw-testbin/cache/<name>-<size>-<mtime><ext>  one copy per build of a program
//	<tmp>/ktw-testbin/run-<pid>-<n>/<name><ext>          the test process's programs
//
// The run folder holds hard links to the cached copies (a copy where a link
// cannot be made): the programs keep their names and sit together, so a
// program that finds its siblings beside itself (the hub resolving a host)
// finds the ones the test asked for. The cache keeps one generation per
// program — a rebuilt one replaces it — and an on-access scanner reads each
// copy once. A run folder whose process has ended is removed by the next run.

// Binary answers the test process's own copy of <repo>/bin/<name>[.exe], or
// "" when it is not built (the caller decides: MissingBinary). Every process
// the test process starts from then on ends with it (Tie).
func Binary(t testing.TB, name string) string {
	t.Helper()
	dir := repoBinDir()
	if dir == "" {
		return ""
	}
	p, err := runCopy(dir, name)
	if err != nil {
		t.Fatalf("hubharness: copy of %s: %v", name, err)
	}
	if p != "" {
		if err := tieSelfOnce(); err != nil {
			t.Fatalf("hubharness: tie the test's processes to it: %v", err)
		}
	}
	return p
}

// repoBinDir answers <repo>/bin, the repository found by walking up from the
// working directory to go.mod; "" when there is none.
func repoBinDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for dir := wd; ; {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "bin")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func exeName(name string) string {
	if runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(name), ".exe") {
		return name + ".exe"
	}
	return name
}

var (
	runMu     sync.Mutex
	runDir    string
	runCopies = map[string]string{} // source path → its copy in runDir
)

func testbinRoot() string { return filepath.Join(os.TempDir(), "ktw-testbin") }

// runCopy answers the run folder's copy of binDir/<name>[.exe], made on first
// use; "" (no error) when the source does not exist.
func runCopy(binDir, name string) (string, error) {
	src := filepath.Join(binDir, exeName(name))
	info, err := os.Stat(src)
	if err != nil || info.IsDir() {
		return "", nil
	}
	runMu.Lock()
	defer runMu.Unlock()
	if p, ok := runCopies[src]; ok {
		return p, nil
	}
	if runDir == "" {
		sweepRuns()
		if err := os.MkdirAll(testbinRoot(), 0o755); err != nil {
			return "", err
		}
		dir, err := os.MkdirTemp(testbinRoot(), fmt.Sprintf("run-%d-", os.Getpid()))
		if err != nil {
			return "", err
		}
		runDir = dir
	}
	cached, err := cacheCopy(src, info)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(runDir, filepath.Base(src))
	if err := os.Link(cached, dst); err != nil {
		if err := copyFile(cached, dst, info.Mode()); err != nil {
			return "", err
		}
	}
	runCopies[src] = dst
	return dst, nil
}

// cacheCopy answers the cache's copy of src for its current build (its size
// and modification time), made once; older builds' copies of the same program
// are removed where nothing holds them.
func cacheCopy(src string, info os.FileInfo) (string, error) {
	cache := filepath.Join(testbinRoot(), "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(src)
	base := strings.TrimSuffix(filepath.Base(src), ext)
	stamp := fmt.Sprintf("%s-%d-%d", base, info.Size(), info.ModTime().UnixNano())
	dst := filepath.Join(cache, stamp+ext)
	if _, err := os.Stat(dst); err == nil {
		// A copy an older harness left without its exec bits is fixed in place.
		return dst, keepMode(dst, info.Mode())
	}
	tmp, err := os.CreateTemp(cache, stamp+".*.part")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	_ = tmp.Close()
	if err := copyFile(src, tmpName, info.Mode()); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		if _, statErr := os.Stat(dst); statErr != nil { // another run did not make it either
			return "", err
		}
	}
	// One generation per program: another build's copy goes where it can.
	if entries, err := os.ReadDir(cache); err == nil {
		for _, e := range entries {
			n := e.Name()
			if n != stamp+ext && strings.HasPrefix(n, base+"-") && strings.HasSuffix(n, ext) && isStamp(strings.TrimSuffix(strings.TrimPrefix(n, base+"-"), ext)) {
				_ = os.Remove(filepath.Join(cache, n))
			}
		}
	}
	return dst, nil
}

// isStamp reports whether s is "<size>-<mtime>", so ktw-hub's sweep never
// takes ktw-hub-x's copies.
func isStamp(s string) bool {
	size, mtime, ok := strings.Cut(s, "-")
	if !ok {
		return false
	}
	_, err1 := strconv.ParseInt(size, 10, 64)
	_, err2 := strconv.ParseInt(mtime, 10, 64)
	return err1 == nil && err2 == nil
}

// sweepRuns removes the run folders of processes that have ended.
func sweepRuns() {
	entries, err := os.ReadDir(testbinRoot())
	if err != nil {
		return
	}
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), "run-")
		if !ok || !e.IsDir() {
			continue
		}
		pidText, _, _ := strings.Cut(rest, "-")
		pid, err := strconv.Atoi(pidText)
		if err != nil || pid == os.Getpid() || hubclient.ProcessAlive(pid) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(testbinRoot(), e.Name()))
	}
}

// copyFile copies src to dst with mode's permissions and the owner's
// read-write-exec. OpenFile's mode applies only when it creates dst, so a dst
// that already exists (the cache's CreateTemp file, 0600) is given it after.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return keepMode(dst, mode)
}

// keepMode gives p mode's permissions and the owner's read-write-exec, so a
// program's copy runs on Linux (Windows keeps only the read-only bit, cleared).
func keepMode(p string, mode os.FileMode) error {
	return os.Chmod(p, mode.Perm()|0o700)
}

var (
	tieOnce sync.Once
	tieErr  error
)

func tieSelfOnce() error {
	tieOnce.Do(func() { tieErr = tieSelf() })
	return tieErr
}

// Tie makes cmd's process end with the test process, even when the test
// process is killed (a timeout, Ctrl-C, taskkill /F): on Windows the test
// process joins a job that kills every process in it when its last handle —
// the test process's — closes, and every process started from then on is in
// it, the ones they start included; on Linux the process gets its own group
// and SIGKILL when its parent dies. Call it before cmd.Start.
func Tie(cmd *exec.Cmd) error {
	tieCmd(cmd)
	return tieSelfOnce()
}
