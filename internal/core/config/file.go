// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// LoadFromDir reads `<dir>/<name>.yaml` into target through the standard
// decode pipeline (variable expansion + ProcessConfig) — one directory, no
// workspace overlay. A missing file leaves target as the caller prepared it
// (defaults), with `default:`/`envkey` tags still applied — not an error.
//
// For typed files kept in one fixed layer, e.g. the cross-universe
// `<GlobalHomeDir>/config/` (Katyoti.yaml, GuiSettings.yaml, Profile.yaml),
// which the active workspace's application.Config() would overlay. The read
// is atomicio.ReadAtomic, the reading half of SaveAtomic's protocol
// (modules/framework/rules/R-003-files-shared-across-goroutines-or-processes-go.cwm.md).
func LoadFromDir(dir, name string, target any) error {
	data, err := atomicio.ReadAtomic(filepath.Join(dir, name+".yaml"))
	switch {
	case errors.Is(err, os.ErrNotExist):
		if t := reflect.TypeOf(target); t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct {
			return ProcessConfig(target)
		}
		return nil
	case err != nil:
		return fmt.Errorf("read %s.yaml: %w", name, err)
	}
	if err := DecodeBytes(data, marshal.YAML, target); err != nil {
		return fmt.Errorf("config %q: %w", name, err)
	}
	return nil
}

// SaveAtomic writes v as YAML to `<dir>/<name>.yaml` through
// atomicio.WriteAtomic (unique temp file, fsync, Windows-aware rename),
// creating dir if needed. The counterpart of LoadFromDir: ConfigService itself
// has no save.
func SaveAtomic(dir, name string, v any) error {
	data, err := marshal.Marshal(v, marshal.YAML)
	if err != nil {
		return fmt.Errorf("encode %s.yaml: %w", name, err)
	}
	if err := atomicio.WriteAtomic(filepath.Join(dir, name+".yaml"), data, 0o644); err != nil {
		return fmt.Errorf("save %s.yaml: %w", name, err)
	}
	return nil
}
