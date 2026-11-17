// SPDX-License-Identifier: Apache-2.0

package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// IsProcessAlive checks whether a process with the given PID is still running.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// On Windows, never os.FindProcess: it asks for more rights than a probe
	// needs (PROCESS_QUERY_INFORMATION), and a live process this one may not
	// open that way read DEAD - a live hub's lockfile taken for a stale one,
	// removed, and a second hub started over it. processRunning
	// asks the least right and calls dead only a pid no process has, or one
	// that has exited (a handle still open keeps its exit code readable).
	if runtime.GOOS == "windows" {
		return processRunning(pid)
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Send signal 0 to probe: no error,
	// or not ours to signal (another user's process) — it is alive.
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, os.ErrPermission)
}

// TCPCheck tries to connect to host:port within the given timeout.
// Returns true if the connection succeeds (server is listening).
func TCPCheck(host string, port int, timeout time.Duration) bool {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// StartBackground re-executes the current binary with the given args in a
// detached background process. Stdout and stderr are redirected to logPath.
// Returns the child PID on success.
func StartBackground(args []string, logPath string) (int, error) {
	exePath, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("find executable: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.Command(exePath, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	applyDetach(cmd)
	publish.NoConsoleWindow(cmd) // after applyDetach, which sets SysProcAttr: a hidden console of its own

	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, fmt.Errorf("start background process: %w", err)
	}

	pid := cmd.Process.Pid
	cmd.Process.Release()
	logFile.Close()

	return pid, nil
}

// CleanStaleInfo checks if the server's PID is alive. If not, removes the
// info file and returns true (was stale). Returns false if alive or no file.
func CleanStaleInfo(name string) bool {
	info, err := ReadInfo(name)
	if err != nil {
		return false
	}
	if IsProcessAlive(info.PID) {
		return false
	}
	_ = RemoveInfo(name)
	return true
}
