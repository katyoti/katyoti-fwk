// SPDX-License-Identifier: Apache-2.0

// Package netutil provides small, focused networking helpers shared across
// the codebase. Today: free-port allocation for child-process spawns.
package netutil

import (
	"fmt"
	"net"
)

// AllocFreePort grabs a free ephemeral TCP port from the kernel and returns
// it. The listener is closed before return so the caller can immediately
// hand the port to a spawning child process.
//
// Race window: between Close() and the caller's exec.Cmd.Start(), another
// process could grab the port. Rare in practice on a developer machine. Use
// AllocFreePortWithRetry for an automatic retry loop.
func AllocFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("alloc free port: %w", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// AllocFreePortWithRetry repeatedly allocates a port and invokes spawnFn(port)
// until it succeeds or maxRetries is exhausted.
//
// Use case: between AllocFreePort's listener close and the child process's
// Start, another process can grab the port. spawnFn returns an error in that
// case (typically EADDRINUSE); the helper retries with a fresh port.
//
// Returns the port that was successfully used. Errors after maxRetries
// failures with the last spawn error wrapped.
//
// maxRetries < 1 is treated as 1 (single attempt, no retry).
func AllocFreePortWithRetry(maxRetries int, spawnFn func(port int) error) (int, error) {
	if maxRetries < 1 {
		maxRetries = 1
	}
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		port, err := AllocFreePort()
		if err != nil {
			lastErr = err
			continue
		}
		if err := spawnFn(port); err == nil {
			return port, nil
		} else {
			lastErr = err
		}
	}
	return 0, fmt.Errorf("alloc free port after %d attempts: %w", maxRetries, lastErr)
}
