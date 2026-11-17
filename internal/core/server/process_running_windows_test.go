// SPDX-License-Identifier: Apache-2.0

//go:build windows

package server

import (
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

// A process that has exited is not alive, even while a handle to it is still
// open somewhere — what kept a dead command's workspace lock "held".
func TestAnExitedProcessIsNotAliveWhileAHandleIsOpen(t *testing.T) {
	cmd := exec.Command(os.Getenv("ComSpec"), "/c", "exit", "0")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start cmd.exe:", err)
	}
	pid := cmd.Process.Pid
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		t.Fatalf("open the child: %v", err)
	}
	defer windows.CloseHandle(h)
	_ = cmd.Wait()
	if IsProcessAlive(pid) {
		t.Fatalf("pid %d exited and still reads alive", pid)
	}
	if !IsProcessAlive(os.Getpid()) {
		t.Fatal("this process reads dead")
	}
}

// A live process this one may not open with full query rights is alive: the
// System process (pid 4) refuses os.FindProcess to a non-elevated caller, and
// reading it dead is how a live hub's lockfile was taken for a stale one and
// a second hub started over it.
func TestALiveProcessThatRefusesFullQueryRightsIsAlive(t *testing.T) {
	if !IsProcessAlive(4) {
		t.Fatal("the System process (pid 4) reads dead")
	}
	if IsProcessAlive(0x7FFFFFF0) {
		t.Fatal("a pid no process has reads alive")
	}
}
