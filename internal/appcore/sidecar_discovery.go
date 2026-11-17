// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	sidecarOnce       sync.Once
	sidecarDiscovered []SidecarBinaryDef
)

// DiscoverSidecars returns all ktw-* sidecar binaries in the same directory as the
// running executable. The result is cached — subsequent calls return the
// same slice without rescanning.
func DiscoverSidecars() []SidecarBinaryDef {
	sidecarOnce.Do(func() {
		sidecarDiscovered = scanSidecars()
	})
	return sidecarDiscovered
}

func scanSidecars() []SidecarBinaryDef {
	selfPath, err := os.Executable()
	if err != nil {
		return nil
	}
	selfPath, _ = filepath.EvalSymlinks(selfPath)
	selfDir := filepath.Dir(selfPath)
	selfExt := filepath.Ext(selfPath)
	selfBase := strings.TrimSuffix(filepath.Base(selfPath), selfExt)

	entries, err := os.ReadDir(selfDir)
	if err != nil {
		return nil
	}

	matchExt := selfExt
	if runtime.GOOS != "windows" {
		matchExt = ""
	}

	seen := make(map[string]bool)
	var result []SidecarBinaryDef
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if !strings.EqualFold(ext, matchExt) {
			continue
		}
		base := strings.TrimSuffix(name, ext)
		if !strings.HasPrefix(base, "ktw-") {
			continue
		}
		if strings.EqualFold(base, selfBase) {
			continue
		}
		if seen[base] {
			continue
		}
		seen[base] = true
		result = append(result, SidecarBinaryDef{
			Name: base,
			Path: filepath.Join(selfDir, name),
		})
	}
	return result
}
