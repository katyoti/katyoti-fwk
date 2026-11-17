// SPDX-License-Identifier: Apache-2.0

//go:build windows

package projectcmd

import (
	"os/exec"
	"strconv"
	"sync"

	"golang.org/x/sys/windows"
)

// procTree holds a started command's processes in a job object, so a stopped
// command takes every process it started with it — bash, the processes its
// forks start (go test, a build) — and none keeps the output open past
// WaitDelay. Where no job can be made, taskkill /T ends the tree it sees.
type procTree struct {
	mu  sync.Mutex
	job windows.Handle
}

func newProcTree(cmd *exec.Cmd) *procTree {
	t := &procTree{}
	cmd.Cancel = func() error {
		t.mu.Lock()
		job := t.job
		t.mu.Unlock()
		if job != 0 {
			_ = windows.TerminateJobObject(job, 1)
		} else if cmd.Process != nil {
			_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		}
		return cmd.Process.Kill()
	}
	return t
}

// started puts the started command's process in a new job: the processes it
// starts from then on are in it too.
func (t *procTree) started(cmd *exec.Cmd) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	defer windows.CloseHandle(proc)
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)
		return
	}
	t.mu.Lock()
	t.job = job
	t.mu.Unlock()
}

func (t *procTree) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job != 0 {
		_ = windows.CloseHandle(t.job)
		t.job = 0
	}
}
