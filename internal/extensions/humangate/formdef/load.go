// SPDX-License-Identifier: Apache-2.0

package formdef

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads a YAML form definition from the given path.
func Load(path string) (*FormDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read form definition: %w", err)
	}
	return Parse(data)
}

// Parse parses a YAML form definition from raw bytes.
func Parse(data []byte) (*FormDefinition, error) {
	var def FormDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse form definition: %w", err)
	}
	if err := Validate(&def); err != nil {
		return nil, err
	}
	return &def, nil
}

// Validate checks the form definition for structural errors.
func Validate(def *FormDefinition) error {
	if def.Screen.Title == "" {
		return fmt.Errorf("screen.title is required")
	}
	if len(def.Fields) == 0 && len(def.Groups) == 0 {
		return fmt.Errorf("form must have at least one field or group")
	}
	if len(def.Fields) > 0 && len(def.Groups) > 0 {
		return fmt.Errorf("form must use either top-level fields or groups, not both")
	}

	seen := map[string]bool{}
	for i, f := range def.Fields {
		if err := validateField(f, seen, fmt.Sprintf("fields[%d]", i)); err != nil {
			return err
		}
	}
	groupKeys := map[string]bool{}
	for i, g := range def.Groups {
		if g.Key == "" {
			return fmt.Errorf("groups[%d].key is required", i)
		}
		if g.Label == "" {
			return fmt.Errorf("groups[%d].label is required", i)
		}
		if groupKeys[g.Key] {
			return fmt.Errorf("duplicate group key %q", g.Key)
		}
		groupKeys[g.Key] = true
		if len(g.Fields) == 0 {
			return fmt.Errorf("group %q must have at least one field", g.Key)
		}
		for j, f := range g.Fields {
			if err := validateField(f, seen, fmt.Sprintf("groups[%d].fields[%d]", i, j)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateField(f Field, seen map[string]bool, path string) error {
	if f.Key == "" {
		return fmt.Errorf("%s: key is required", path)
	}
	if seen[f.Key] {
		return fmt.Errorf("%s: duplicate field key %q", path, f.Key)
	}
	seen[f.Key] = true

	if f.Type == "" {
		return fmt.Errorf("%s (%s): type is required", path, f.Key)
	}
	if !ValidFieldTypes[f.Type] {
		return fmt.Errorf("%s (%s): unknown type %q", path, f.Key, f.Type)
	}

	switch f.Type {
	case "enum":
		if len(f.Options) == 0 {
			return fmt.Errorf("%s (%s): enum field must have options", path, f.Key)
		}
	case "object":
		if len(f.Fields) == 0 {
			return fmt.Errorf("%s (%s): object field must have sub-fields", path, f.Key)
		}
		childSeen := map[string]bool{}
		for i, child := range f.Fields {
			if err := validateField(child, childSeen, fmt.Sprintf("%s.fields[%d]", path, i)); err != nil {
				return err
			}
		}
	case "array":
		if f.Items == nil {
			return fmt.Errorf("%s (%s): array field must have items", path, f.Key)
		}
	case "table":
		if len(f.Columns) == 0 {
			return fmt.Errorf("%s (%s): table field must have columns", path, f.Key)
		}
	}
	return nil
}
