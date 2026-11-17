// SPDX-License-Identifier: Apache-2.0

//go:build windows

package publish

// Windows-only: assign every spawned external process to a process-wide Job
// Object with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE. The kernel auto-terminates
// all job members when this process's last handle to the job is released —
// which happens on ANY process exit, including panic/crash/force-kill.
//
// Without this, sidecars spawned by `exec.CommandContext` become orphans
// when the parent dies abruptly: Windows has no SIGHUP-on-parent-death,
// so the children keep running, ports bound, indefinitely.
//
// Companion to the cross-platform supervisor.go. Linux/macOS get a no-op
// stub in supervisor_other.go (POSIX already cascades via process group +
// ctx cancellation).

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	jobOnce    sync.Once
	jobHandle  windows.Handle
	jobInitErr error
)

// ensureParentJob lazily creates the process-wide Job Object on first use
// and configures KILL_ON_JOB_CLOSE. The handle is intentionally kept open
// for the lifetime of the process — when the process exits, the handle is
// released, triggering the kernel cleanup.
func ensureParentJob() (windows.Handle, error) {
	jobOnce.Do(func() {
		h, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			jobInitErr = fmt.Errorf("create job object: %w", err)
			return
		}
		var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(
			h,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		); err != nil {
			_ = windows.CloseHandle(h)
			jobInitErr = fmt.Errorf("set job info: %w", err)
			return
		}
		jobHandle = h
	})
	return jobHandle, jobInitErr
}

// AttachToParentJob joins the given process to the supervisor's Job Object.
// Best-effort: returns the error so the caller can log it, but the supervisor
// should continue (orphan-prone is better than refusing to spawn). Exported
// so other spawn sites (notably connmgr's NATS-sidecar spawn path) can opt
// into the same kernel-level cleanup.
func AttachToParentJob(p *os.Process) error {
	if p == nil {
		return fmt.Errorf("nil process")
	}
	job, err := ensureParentJob()
	if err != nil {
		return err
	}
	// Open the child process handle with the rights needed for AssignProcessToJobObject.
	procHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(p.Pid),
	)
	if err != nil {
		return fmt.Errorf("open process %d: %w", p.Pid, err)
	}
	defer windows.CloseHandle(procHandle)
	if err := windows.AssignProcessToJobObject(job, procHandle); err != nil {
		return fmt.Errorf("assign to job: %w", err)
	}
	slog.Debug("attached sidecar to parent job", "pid", p.Pid)
	return nil
}
