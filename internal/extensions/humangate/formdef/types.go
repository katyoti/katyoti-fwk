// SPDX-License-Identifier: Apache-2.0

package formdef

// FormDefinition is the top-level custom YAML form definition.
// If Groups are defined, the form renders as tabs (JSON Forms Categorization).
// If only Fields are defined (no Groups), the form renders as a single vertical layout.
type FormDefinition struct {
	Screen Screen  `yaml:"screen"`
	Fields []Field `yaml:"fields,omitempty"` // flat field list (no tabs)
	Groups []Group `yaml:"groups,omitempty"` // tabbed layout — each group becomes a tab
}

// Screen holds form window metadata.
type Screen struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description,omitempty"`
	Width       int    `yaml:"width,omitempty"`
	Height      int    `yaml:"height,omitempty"`
}

// Field represents a single form field.
// Type must be one of: string, text, markdown, integer, number, boolean,
// enum, date, time, datetime, object, array, table.
type Field struct {
	Key         string      `yaml:"key"`
	Type        string      `yaml:"type"`
	Label       string      `yaml:"label,omitempty"`
	Description string      `yaml:"description,omitempty"`
	Required    bool        `yaml:"required,omitempty"`
	Default     any         `yaml:"default,omitempty"`
	Placeholder string      `yaml:"placeholder,omitempty"`
	ReadOnly    bool        `yaml:"readOnly,omitempty"`
	Display     string      `yaml:"display,omitempty"` // enum display: "radio" or "dropdown" (default)
	Validation  *Validation `yaml:"validation,omitempty"`

	// enum type: list of allowed values
	Options []Option `yaml:"options,omitempty"`

	// object type: recursive sub-fields
	Fields []Field `yaml:"fields,omitempty"`

	// array type: item template (defines what each array element looks like)
	Items *Field `yaml:"items,omitempty"`

	// table type: column definitions (array-of-objects rendered as a grid)
	Columns []Field `yaml:"columns,omitempty"`
}

// Validation defines per-field validation constraints.
type Validation struct {
	MinLength *int     `yaml:"minLength,omitempty"`
	MaxLength *int     `yaml:"maxLength,omitempty"`
	Minimum   *float64 `yaml:"minimum,omitempty"`
	Maximum   *float64 `yaml:"maximum,omitempty"`
	Pattern   string   `yaml:"pattern,omitempty"`
	Format    string   `yaml:"format,omitempty"` // email, uri, hostname, ipv4, etc.
}

// Option is a value/label pair for enum fields.
type Option struct {
	Value       string `yaml:"value"`
	Label       string `yaml:"label"`
	Description string `yaml:"description,omitempty"` // shown as helper text in radio mode
}

// Group defines a tab or section. Fields within a group are rendered together.
type Group struct {
	Key    string  `yaml:"key"`
	Label  string  `yaml:"label"`
	Fields []Field `yaml:"fields"`
}

// AllFields returns all fields from the definition, whether they are
// at the top level (flat) or inside groups (tabbed).
func (d *FormDefinition) AllFields() []Field {
	if len(d.Groups) > 0 {
		var all []Field
		for _, g := range d.Groups {
			all = append(all, g.Fields...)
		}
		return all
	}
	return d.Fields
}

// ValidFieldTypes is the set of all supported field types.
var ValidFieldTypes = map[string]bool{
	"string":   true,
	"text":     true,
	"markdown": true,
	"integer":  true,
	"number":   true,
	"boolean":  true,
	"enum":     true,
	"date":     true,
	"time":     true,
	"datetime": true,
	"object":   true,
	"array":    true,
	"table":    true,
}
