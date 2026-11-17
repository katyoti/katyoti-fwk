// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package publish

import "os"

// AttachToParentJob is a no-op on POSIX systems. Process group + ctx
// cancellation already cascade exits down the tree:
//   - exec.CommandContext kills the child on ctx.Done()
//   - process group setup (when needed) propagates SIGHUP on parent exit
//
// so we don't need a Job Object equivalent here.
func AttachToParentJob(*os.Process) error { return nil }
