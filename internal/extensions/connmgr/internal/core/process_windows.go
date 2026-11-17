// SPDX-License-Identifier: Apache-2.0

//go:build windows

package core

import "os"

// signalProcess on Windows: SIGTERM doesn't exist for child processes
// in the Unix sense, and CTRL_BREAK_EVENT requires creating the process
// in a new process group at spawn time (which we don't today). For the
// first impl pass we Kill outright — the NATS server cleans up the
// sidecar's queue-group subscription when the connection drops, so the
// bus state stays consistent even without a clean drain on the sidecar.
//
// Follow-up: spawn with `SysProcAttr{CreationFlags: CREATE_NEW_PROCESS_GROUP}`
// and send CTRL_BREAK_EVENT for graceful shutdown on Windows.
func signalProcess(p *os.Process) error {
	return p.Kill()
}
