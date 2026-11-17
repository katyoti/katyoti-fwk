// SPDX-License-Identifier: Apache-2.0

//go:build windows

package core

import (
	"syscall"
	"testing"
)

// otherPathForm is dir's 8.3 short name; the test is skipped on a volume
// that keeps no short names.
func otherPathForm(t *testing.T, dir string) string {
	t.Helper()
	long, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, 1024)
	n, err := syscall.GetShortPathName(long, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		t.Skipf("no short name for %s: %v", dir, err)
	}
	short := syscall.UTF16ToString(buf[:n])
	if short == dir {
		t.Skipf("the volume keeps no 8.3 short name for %s", dir)
	}
	return short
}
