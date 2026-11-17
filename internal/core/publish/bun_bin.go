// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"os"
	"path/filepath"
	"strings"
)

// BunGlobalBin is where `bun add --global` puts a package's commands: the bin
// folder of $BUN_INSTALL, else of ~/.bun - bun's own layout. "" when neither
// is known.
func BunGlobalBin() string {
	if root := strings.TrimSpace(os.Getenv("BUN_INSTALL")); root != "" {
		return filepath.Join(root, "bin")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".bun", "bin")
}
