// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package atomicio

import "os"

// readShared is os.ReadFile: a POSIX rename never waits for readers.
func readShared(path string) ([]byte, error) { return os.ReadFile(path) }

// replaceFile is os.Rename: atomic over an open destination on POSIX.
func replaceFile(tmp, path string) error { return os.Rename(tmp, path) }
