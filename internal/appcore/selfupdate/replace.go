// SPDX-License-Identifier: Apache-2.0

// Package selfupdate provides the running-exe swap primitive used by katyoti's
// control-plane install/update path (katyoti + ktw-hub + ktw-gui are the only
// binaries with the running-exe problem; ordinary sidecars run under the hub from
// the store and are add-version + repoint).
//
// The hard case is Windows: a running .exe cannot be deleted or overwritten, but
// it CAN be renamed. Replace exploits that with a rename-aside then atomic publish,
// staged entirely within the target's own directory so every rename is same-volume
// (and therefore atomic). The mechanism is identical on Unix, so callers need no
// platform branch.
package selfupdate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// oldSuffix / newSuffix are the staging markers appended (with a nanosecond stamp)
// beside the target during a swap. CleanupOld sweeps the .old-* leftovers.
const (
	oldMarker = ".old-"
	newMarker = ".new-"
)

// Replace swaps the binary at targetPath with the bytes of newPath, tolerating
// targetPath being a CURRENTLY-RUNNING executable.
//
// Steps (all within targetPath's directory → same-volume, atomic renames):
//  1. copy newPath → <target>.new-<ts>, fsync, chmod 0o755
//  2. rename <target> → <target>.old-<ts>  (frees the name; a running process
//     keeps executing from the renamed file)
//  3. rename <target>.new-<ts> → <target>  (atomic publish)
//  4. best-effort remove <target>.old-<ts> (still locked by the running process
//     on Windows — left for CleanupOld to sweep on a later run)
//
// If step 3 fails after step 2, the original is rolled back into place. The staged
// .new file is removed on any error. When targetPath does not exist yet, this
// degrades to a plain staged install (no rename-aside).
func Replace(targetPath, newPath string) (err error) {
	info, err := os.Stat(newPath)
	if err != nil {
		return fmt.Errorf("selfupdate: source %q: %w", newPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("selfupdate: source %q is a directory", newPath)
	}

	// Ensure the target directory exists — a fresh install (e.g. the first
	// install-infra into an empty control-plane location) has no dir yet.
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("selfupdate: create target dir: %w", err)
	}

	ts := time.Now().UnixNano()
	staged := fmt.Sprintf("%s%s%d", targetPath, newMarker, ts)

	if err := copyFile(staged, newPath); err != nil {
		return fmt.Errorf("selfupdate: stage new binary: %w", err)
	}
	// Any early return past this point must not leave the staged file behind.
	defer func() {
		if err != nil {
			_ = os.Remove(staged)
		}
	}()

	// Fresh install: no existing target to move aside.
	if _, statErr := os.Lstat(targetPath); os.IsNotExist(statErr) {
		if err := os.Rename(staged, targetPath); err != nil {
			return fmt.Errorf("selfupdate: publish new binary: %w", err)
		}
		return nil
	} else if statErr != nil {
		return fmt.Errorf("selfupdate: stat target %q: %w", targetPath, statErr)
	}

	aside := fmt.Sprintf("%s%s%d", targetPath, oldMarker, ts)
	if err := os.Rename(targetPath, aside); err != nil {
		return fmt.Errorf("selfupdate: move running binary aside: %w", err)
	}

	if err := os.Rename(staged, targetPath); err != nil {
		// Roll back: restore the original so the target is never left missing.
		if rbErr := os.Rename(aside, targetPath); rbErr != nil {
			return fmt.Errorf("selfupdate: publish failed (%v) AND rollback failed (%v) — original at %q", err, rbErr, aside)
		}
		return fmt.Errorf("selfupdate: publish new binary: %w", err)
	}

	// Best-effort: on Windows the running process still holds the aside file, so
	// this fails and CleanupOld sweeps it later. Not an error.
	_ = os.Remove(aside)
	return nil
}

// CleanupOld removes leftover <target>.old-* files beside targetPath — the aside
// copies a previous Replace could not delete because a running process still held
// them. Best-effort: files still locked are skipped without error. Call at startup
// so a self-updated binary tidies its predecessor once that process has exited.
func CleanupOld(targetPath string) error {
	matches, err := filepath.Glob(targetPath + oldMarker + "*")
	if err != nil {
		return fmt.Errorf("selfupdate: scan for stale binaries: %w", err)
	}
	for _, m := range matches {
		_ = os.Remove(m) // locked files stay; swept on a later run
	}
	return nil
}

// IsLeftover reports whether a file name is a swap's leftover rather than a
// program: a renamed-aside `<binary>.old-<stamp>` (Replace, scripts/install.sh,
// place-binary.sh), a staged `<binary>.new-<stamp>`, or any `*.old`. Such a
// file is never published: a marketplace cut removes it from its tree and
// never copies one into it.
func IsLeftover(name string) bool {
	name = strings.ToLower(filepath.Base(name))
	return strings.Contains(name, oldMarker) || strings.Contains(name, newMarker) || strings.HasSuffix(name, ".old")
}

// copyFile writes src's bytes to a fresh dst, fsyncs, and marks it executable.
// dst is created exclusively so a stale staged file never silently masquerades as
// a fresh copy.
func copyFile(dst, src string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(dst)
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	// Ensure the executable bit even when umask stripped it at create time.
	return os.Chmod(dst, 0o755)
}
