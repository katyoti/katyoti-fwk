// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ProcessConfig walks the struct behind target and applies struct-tag rules
// after unmarshaling. target must be a pointer to a struct.
//
// Supported tags:
//
//	envkey:"ENV_VAR"   — if the field is still zero after unmarshal and the env var is set, fill it (YAML wins over env)
//	default:"value"    — if the field is still zero after unmarshal + env, set this default
//	required:"true"    — if the field is still zero after all resolution, return an error
//
// Processing order per field: envkey override → default fallback → required check.
// Nested structs are walked recursively.
func ProcessConfig(target any) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return fmt.Errorf("ProcessConfig: target must be a non-nil struct pointer")
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("ProcessConfig: target must be a non-nil struct pointer")
	}
	return processStruct(v)
}

func processStruct(v reflect.Value) error {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		ftype := t.Field(i)

		if !field.CanSet() {
			continue
		}

		// Recurse into embedded/nested structs (skip if field has envkey — treat as leaf).
		if field.Kind() == reflect.Struct && ftype.Tag.Get("envkey") == "" {
			if hasConfigTags(ftype) || hasConfigTagsInStruct(field.Type()) {
				if err := processStruct(field); err != nil {
					return err
				}
			}
			continue
		}

		// Step 1: envkey fallback — only fills if field is still zero
		// (YAML values take precedence over env vars; env vars fill missing values)
		if envKey := ftype.Tag.Get("envkey"); envKey != "" {
			if field.IsZero() {
				if val, ok := os.LookupEnv(envKey); ok {
					if err := setField(field, val); err != nil {
						return fmt.Errorf("field %s (envkey %s): %w", ftype.Name, envKey, err)
					}
				}
			}
		}

		// Step 2: default fallback (only if field is zero-value)
		if def := ftype.Tag.Get("default"); def != "" {
			if field.IsZero() {
				if err := setField(field, def); err != nil {
					return fmt.Errorf("field %s (default %q): %w", ftype.Name, def, err)
				}
			}
		}

		// Step 3: required check
		if isTrue(ftype.Tag.Get("required")) {
			if field.IsZero() {
				label := ftype.Name
				if envKey := ftype.Tag.Get("envkey"); envKey != "" {
					label = fmt.Sprintf("%s (envkey %s)", ftype.Name, envKey)
				}
				return fmt.Errorf("required config field %s is missing", label)
			}
		}
	}
	return nil
}

func setField(field reflect.Value, value string) error {
	typ := field.Type()

	if typ.Kind() == reflect.Ptr {
		if field.IsNil() {
			field.Set(reflect.New(typ.Elem()))
		}
		return setField(field.Elem(), value)
	}

	switch typ.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if typ.PkgPath() == "time" && typ.Name() == "Duration" {
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			field.SetInt(int64(d))
		} else {
			n, err := strconv.ParseInt(value, 0, typ.Bits())
			if err != nil {
				return err
			}
			field.SetInt(n)
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(value, 0, typ.Bits())
		if err != nil {
			return err
		}
		field.SetUint(n)

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, typ.Bits())
		if err != nil {
			return err
		}
		field.SetFloat(f)

	case reflect.Slice:
		if typ.Elem().Kind() == reflect.String {
			if strings.TrimSpace(value) == "" {
				field.Set(reflect.MakeSlice(typ, 0, 0))
			} else {
				parts := strings.Split(value, ",")
				for i := range parts {
					parts[i] = strings.TrimSpace(parts[i])
				}
				field.Set(reflect.ValueOf(parts))
			}
		} else {
			return fmt.Errorf("unsupported slice element type %s", typ.Elem().Kind())
		}

	default:
		return fmt.Errorf("unsupported type %s", typ.Kind())
	}

	return nil
}

func hasConfigTags(f reflect.StructField) bool {
	return f.Tag.Get("envkey") != "" || f.Tag.Get("default") != "" || f.Tag.Get("required") != ""
}

func hasConfigTagsInStruct(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		if hasConfigTags(t.Field(i)) {
			return true
		}
		if t.Field(i).Type.Kind() == reflect.Struct {
			if hasConfigTagsInStruct(t.Field(i).Type) {
				return true
			}
		}
	}
	return false
}

func isTrue(s string) bool {
	b, _ := strconv.ParseBool(s)
	return b
}
