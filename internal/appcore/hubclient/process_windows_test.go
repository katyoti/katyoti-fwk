// SPDX-License-Identifier: Apache-2.0

//go:build windows

package hubclient

// canDetectStalePIDsImpl: Windows now has a real liveness probe via
// OpenProcess + GetExitCodeProcess (see process_windows.go::probeSignal),
// so stale-PID detection works identically to Unix. The test gate that
// previously skipped on Windows is no longer needed.
func canDetectStalePIDsImpl() bool { return true }
