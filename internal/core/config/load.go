// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/variables"
)

// ── Core pipeline ────────────────────────────────────────────────────────

// DecodeBytes is the single entry point for bytes → struct decoding.
// Pipeline: variable expansion → unmarshal → ProcessConfig (for structs).
// target must be a pointer.
func DecodeBytes(data []byte, format marshal.Format, target any) error {
	data = variables.ExpandVarsBytes(data)
	if err := marshal.Unmarshal(data, format, target); err != nil {
		return err
	}
	if reflect.TypeOf(target).Elem().Kind() == reflect.Struct {
		return ProcessConfig(target)
	}
	return nil
}

// DecodeMap converts a map[string]any into a typed struct via the full pipeline.
// Marshals to YAML bytes, then applies DecodeBytes (variable expansion +
// unmarshal + ProcessConfig). target must be a pointer to a struct.
func DecodeMap(m map[string]any, target any) error {
	data, err := marshal.Marshal(m, marshal.YAML)
	if err != nil {
		return fmt.Errorf("config: encode map: %w", err)
	}
	return DecodeBytes(data, marshal.YAML, target)
}

// Decode wraps DecodeBytes for generic return-value callers.
func Decode[T any](data []byte, format marshal.Format, label string) (T, error) {
	var target T
	if err := DecodeBytes(data, format, &target); err != nil {
		return target, fmt.Errorf("config %q: %w", label, err)
	}
	return target, nil
}

// LoadMap decodes raw bytes into map[string]any using the given format.
// Applies variable expansion but not ProcessConfig (maps have no struct tags).
func LoadMap(data []byte, format marshal.Format) (map[string]any, error) {
	data = variables.ExpandVarsBytes(data)
	return marshal.UnmarshalMap(data, format)
}
