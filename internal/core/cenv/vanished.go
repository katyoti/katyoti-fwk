// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"strings"
)

// Vanished is whether path is surely gone: an absolute path that does not
// exist while its volume does. A relative or bare name (a program found on
// PATH), a path that cannot be read for another reason, or one on a drive or
// share that is not there (unplugged, not mounted) is not vanished - a
// registry keeps its entry (RQ-062).
func Vanished(path string) bool {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return false
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return false
	}
	if vol := filepath.VolumeName(path); vol != "" {
		if _, err := os.Stat(vol + string(filepath.Separator)); err != nil {
			return false
		}
	}
	return true
}
