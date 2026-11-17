// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// CHECKPOINTS — a copy of everything the case changes (its universe, project,
// data and state), taken with the universe stopped so nothing is half
// written. `installed` is taken at install; each stage that passes takes one
// named after it. Restoring the checkpoint before a stage puts the case back
// exactly where that stage starts: a fix is tested by running that stage
// again, not the whole scenario.

// CheckpointInstalled is the checkpoint install takes.
const CheckpointInstalled = "installed"

// Checkpoint copies the case's state under name (replacing one of that name).
func (e *Engine) Checkpoint(ctx context.Context, name string) error {
	was := e.Running(ctx)
	if was {
		if err := e.Stop(ctx); err != nil {
			return err
		}
	}
	dst := filepath.Join(e.RT.Checkpoints(), name)
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	for _, part := range e.RT.snapshotParts() {
		src := filepath.Join(e.RT.Dir, part)
		info, err := os.Stat(src)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = copyTree(src, filepath.Join(dst, part))
		} else {
			err = copyFile(src, filepath.Join(dst, part))
		}
		if err != nil {
			return fmt.Errorf("checkpoint %s: %s: %w", name, part, err)
		}
	}
	e.logf("checkpoint %s taken", name)
	if was {
		return e.Start(ctx)
	}
	return nil
}

// Restore puts the case back to a checkpoint; the universe is left stopped.
func (e *Engine) Restore(ctx context.Context, name string) error {
	src := filepath.Join(e.RT.Checkpoints(), name)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("no checkpoint %q (have %v)", name, e.Checkpoints())
	}
	if err := e.Stop(ctx); err != nil {
		return err
	}
	for _, part := range e.RT.snapshotParts() {
		if err := os.RemoveAll(filepath.Join(e.RT.Dir, part)); err != nil {
			return fmt.Errorf("restore %s: clear %s: %w", name, part, err)
		}
		from := filepath.Join(src, part)
		info, err := os.Stat(from)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			err = copyTree(from, filepath.Join(e.RT.Dir, part))
		} else {
			err = copyFile(from, filepath.Join(e.RT.Dir, part))
		}
		if err != nil {
			return fmt.Errorf("restore %s: %s: %w", name, part, err)
		}
	}
	e.logf("restored checkpoint %s", name)
	return nil
}

// Checkpoints lists the checkpoints taken.
func (e *Engine) Checkpoints() []string {
	entries, _ := os.ReadDir(e.RT.Checkpoints())
	var out []string
	for _, d := range entries {
		if d.IsDir() {
			out = append(out, d.Name())
		}
	}
	sort.Strings(out)
	return out
}

// checkpointBefore is the checkpoint a stage starts from: the previous
// stage's, or `installed` for the first.
func (e *Engine) checkpointBefore(stage string) string {
	i := e.Case.StageIndex(stage)
	if i <= 0 {
		return CheckpointInstalled
	}
	return e.Case.Stages[i-1].ID
}
