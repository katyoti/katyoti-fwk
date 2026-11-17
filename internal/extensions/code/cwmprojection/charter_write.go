// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteCharterMD renders the markdown via BuildCharter then writes
// it atomically to path (write-to-`.tmp` + rename). Creates the
// parent directory if needed. Returns the number of bytes written.
func WriteCharterMD(in CharterInputs, path string) (int, error) {
	md := BuildCharter(in)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, fmt.Errorf("create charter dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(md), 0o644); err != nil {
		return 0, fmt.Errorf("write charter tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("rename charter into place: %w", err)
	}
	return len(md), nil
}
