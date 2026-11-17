// SPDX-License-Identifier: Apache-2.0

//go:build !unix && !windows

package orchestration

import (
	"io/fs"
	"os"
)

// fileLinkCount: this system tells no link count; one name is assumed.
func fileLinkCount(_ *os.File, _ fs.FileInfo) (uint64, error) { return 1, nil }
