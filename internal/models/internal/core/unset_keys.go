// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

var unexpanded = regexp.MustCompile(`\$\{\{\s*env:([A-Za-z0-9_]+)\s*\}\}`)

// UnsetKeys refuses a model config that still holds a `${{env:KEY}}`: its key
// was not set when the config was read — never imported, or imported while the
// process ran (a process reads its .env when it starts). Said by name, instead
// of the provider failing on the literal (a base URL that does not parse).
func UnsetKeys(cfg any) error {
	seen := map[string]bool{}
	var names []string
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).IsExported() {
					walk(v.Field(i))
				}
			}
		case reflect.Map:
			for _, k := range v.MapKeys() {
				walk(v.MapIndex(k))
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.String:
			for _, m := range unexpanded.FindAllStringSubmatch(v.String(), -1) {
				if !seen[m[1]] {
					seen[m[1]] = true
					names = append(names, m[1])
				}
			}
		}
	}
	walk(reflect.ValueOf(cfg))
	if len(names) == 0 {
		return nil
	}
	return fmt.Errorf("key %s is not set — import it (vault, keys file or config-set), then restart the universe: its programs read their keys when they start",
		strings.Join(names, ", "))
}
