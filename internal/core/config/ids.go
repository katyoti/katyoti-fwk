// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
)

// IdFromPath derives a config ID from a file path.
//
//	"Settings.yaml"     → "Settings"
//	"LLM.yaml"          → "LLM"
//	"LLM/openai.yaml"   → "LLM(openai)"
func IdFromPath(path string) string {
	path = filepath.ToSlash(path)
	ext := filepath.Ext(path)
	path = strings.TrimSuffix(path, ext)
	if dir, name, ok := strings.Cut(path, "/"); ok {
		return dir + "(" + name + ")"
	}
	return path
}

// ServiceName returns the canonical config key for a service instance:
// "ClassName(instanceName)".
func ServiceName(className, instanceName string) string {
	return fmt.Sprintf("%s(%s)", className, instanceName)
}

// TypeName returns the Go type name for T, dereferencing pointers.
// Used for service registration, config file names, and all name derivation.
// The type name IS the config file name (e.g., Settings → Settings.yaml).
// Panics if T has no name (e.g., unnamed struct).
func TypeName[T any]() string {
	return TypeNameOf(reflect.TypeOf((*T)(nil)).Elem())
}

// TypeNameOf returns the Go type name from a reflect.Type.
// Non-generic version of TypeName for dynamic/reflection-based use.
func TypeNameOf(t reflect.Type) string {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	name := t.Name()
	if name == "" {
		panic(fmt.Sprintf("config: type %v has no name — use a named type or interface", t))
	}
	return name
}

// ConfigName derives the config file name from T's type name.
// "Config" and "Settings" suffixes are stripped.
// e.g. PluginsConfig → "Plugins", DocSettings → "Doc", Settings → "Settings"
func ConfigName[T any]() string {
	typeName := TypeName[T]()
	typeName = strings.TrimSuffix(typeName, "Config")
	typeName = strings.TrimSuffix(typeName, "Settings")
	if typeName == "" {
		typeName = TypeName[T]()
	}
	return typeName
}
