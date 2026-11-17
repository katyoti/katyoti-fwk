// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package core

import (
	"os"
	"syscall"
)

// signalProcess sends SIGTERM. The sidecar's `sc ktsp nats` command listens
// for SIGTERM via signal.NotifyContext and drains its NATS subscriptions
// before exiting.
func signalProcess(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
