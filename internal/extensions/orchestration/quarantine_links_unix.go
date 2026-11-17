// SPDX-License-Identifier: Apache-2.0

//go:build unix

package orchestration

import (
	"io/fs"
	"os"
	"syscall"
)

// fileLinkCount is how many names the opened file has.
func fileLinkCount(_ *os.File, info fs.FileInfo) (uint64, error) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink), nil
	}
	return 1, nil
}
