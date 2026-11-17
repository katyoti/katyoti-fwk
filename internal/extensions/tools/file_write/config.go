// SPDX-License-Identifier: Apache-2.0

package file_write

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// FileWriteConfig holds the tunable limits applied by the file_write
// tool. Mirrors the FileReadConfig pattern: a typed-config service
// with sane defaults baked in via DefaultConfig(). No YAML file in
// MVP — defaults can be overridden by registering a custom instance
// via application.PutService.
//
// MaxBytes is the upper bound on Content size accepted by Validate.
// Writes larger than this are rejected before any I/O. The default
// (1 MiB) is intentionally larger than file_read's 256 KiB cap
// because writes legitimately produce larger artifacts (generated
// source files, compiled assets, full-rewrite drops). The limit
// exists primarily to keep a runaway agent from accidentally writing
// gigabytes.
type FileWriteConfig struct {
	// MaxBytes caps Content length. Default: 1 MiB.
	MaxBytes int
}

// DefaultConfig returns a *FileWriteConfig populated with the MVP
// defaults. Tests that need a smaller cap can construct a custom
// instance and inject it via the test config-context seam.
func DefaultConfig() *FileWriteConfig {
	return &FileWriteConfig{
		MaxBytes: 1 << 20, // 1 MiB
	}
}

// FileWriteConfigExtension registers *FileWriteConfig as a simple DI
// service backed by DefaultConfig(). Embedding binaries that want to
// override the cap can register a custom instance before this
// extension runs, or replace the service after Run() via
// application.PutService.
var FileWriteConfigExtension = application.DeclareSimpleService[*FileWriteConfig](
	func(_ context.Context) (*FileWriteConfig, error) {
		return DefaultConfig(), nil
	},
)
