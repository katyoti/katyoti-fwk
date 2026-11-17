// SPDX-License-Identifier: Apache-2.0

//go:build !windows && !linux

package hubharness

import "os/exec"

// Elsewhere nothing ties a process to its parent's death: the process stays in
// the test's group, so a Ctrl-C reaches it, and the harness's cleanup ends it.
func tieSelf() error { return nil }

func tieCmd(*exec.Cmd) {}
