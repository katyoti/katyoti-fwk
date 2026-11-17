// SPDX-License-Identifier: Apache-2.0

package concurrency

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/proclease"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// lockfileBaseName is the filename inside the workspace root.
const lockfileBaseName = ".lock"

// ProcessLockfile is the CWM workspace lock: a process lease
// (core/proclease) on <root>/.lock. Only the refusal is CWM's own:
// another live holder is api.ErrLockHeldByOtherProcess.
type ProcessLockfile struct {
	*proclease.Lease
}

// NewProcessLockfile constructs the workspace lock anchored at root.
// The file itself is not created until Acquire is called.
func NewProcessLockfile(root string) *ProcessLockfile {
	return &ProcessLockfile{Lease: proclease.New(filepath.Join(root, lockfileBaseName))}
}

// Acquire obtains the workspace lock; api.ErrLockHeldByOtherProcess, naming
// the holder's pid, when another live process holds it freshly.
func (l *ProcessLockfile) Acquire(ctx context.Context) error {
	if err := l.Lease.Acquire(ctx); err != nil {
		// The refusal names the holder ("...: pid N"): a member that cannot
		// open the workspace says which process keeps it.
		var held *proclease.HeldError
		if errors.As(err, &held) {
			return fmt.Errorf("%w: pid %d", api.ErrLockHeldByOtherProcess, held.PID)
		}
		if errors.Is(err, proclease.ErrHeld) {
			return api.ErrLockHeldByOtherProcess
		}
		return err
	}
	return nil
}

// Check refuses a write once this process lost the workspace lock to
// another: the other one writes the store now.
func (l *ProcessLockfile) Check() error {
	if err := l.Lease.Check(); err != nil {
		return fmt.Errorf("%w: this process lost the workspace lock to another process and writes no more (%v)", api.ErrLockHeldByOtherProcess, err)
	}
	return nil
}
