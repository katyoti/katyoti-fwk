// SPDX-License-Identifier: Apache-2.0

package report

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// ReportDef is a YAML-defined multi-step report pipeline.
type ReportDef struct {
	Version     string            `yaml:"version"`
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Variables   map[string]VarDef `yaml:"variables,omitempty"`
	Steps       []StepDef         `yaml:"steps"`
	Output      OutputDef         `yaml:"output"`
}

// VarDef defines a report variable with optional default.
type VarDef struct {
	Desc     string `yaml:"desc,omitempty"`
	Required bool   `yaml:"required,omitempty"`
	Default  string `yaml:"default,omitempty"`
}

// StepDef defines a single step in the report pipeline.
type StepDef struct {
	Name        string            `yaml:"name"`
	Type        string            `yaml:"type"`                  // "query" or "transform"
	Query       string            `yaml:"query,omitempty"`       // for type=query: template name
	Vars        map[string]string `yaml:"vars,omitempty"`        // for type=query: variable overrides
	Transformer string            `yaml:"transformer,omitempty"` // for type=transform: transformer name
	Input       string            `yaml:"input,omitempty"`       // for type=transform: step name reference
	Params      map[string]any    `yaml:"params,omitempty"`      // for type=transform: transformer params
}

// OutputDef defines report output configuration.
type OutputDef struct {
	XLSX *XLSXOutputDef `yaml:"xlsx,omitempty"`
}

// XLSXOutputDef defines multi-sheet Excel output.
type XLSXOutputDef struct {
	Sheets []SheetDef `yaml:"sheets"`
}

// SheetDef defines a single sheet in the Excel output.
type SheetDef struct {
	Name string `yaml:"name"` // Sheet tab name
	Data string `yaml:"data"` // Step name reference (supports dot notation: "step.subOutput")
}

// Column describes a column of a step result: its id (the key in the rows), its label (the header) and the sets it belongs to.
type Column struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Sets  []string `json:"sets,omitempty"`
}

// LoadReportDef loads a report definition from a YAML file.
func LoadReportDef(path string) (*ReportDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read report file: %w", err)
	}
	return ParseReportDef(data)
}

// ParseReportDef parses a report definition from YAML bytes.
func ParseReportDef(data []byte) (*ReportDef, error) {
	var def ReportDef
	if err := config.DecodeBytes(data, marshal.YAML, &def); err != nil {
		return nil, fmt.Errorf("parse report YAML: %w", err)
	}
	if def.Name == "" {
		return nil, fmt.Errorf("report definition missing 'name'")
	}
	if len(def.Steps) == 0 {
		return nil, fmt.Errorf("report definition has no steps")
	}
	for i, step := range def.Steps {
		if step.Name == "" {
			return nil, fmt.Errorf("step %d missing 'name'", i)
		}
		if step.Type == "" {
			return nil, fmt.Errorf("step %q missing 'type'", step.Name)
		}
		switch step.Type {
		case "query":
			if step.Query == "" {
				return nil, fmt.Errorf("step %q (type=query) missing 'query'", step.Name)
			}
		case "transform":
			if step.Transformer == "" {
				return nil, fmt.Errorf("step %q (type=transform) missing 'transformer'", step.Name)
			}
			if step.Input == "" {
				return nil, fmt.Errorf("step %q (type=transform) missing 'input'", step.Name)
			}
		default:
			return nil, fmt.Errorf("step %q has unknown type %q (expected 'query' or 'transform')", step.Name, step.Type)
		}
	}
	return &def, nil
}

// ResolveVariables merges variable defaults with runtime overrides and validates required vars.
func ResolveVariables(def *ReportDef, vars map[string]string) (map[string]string, error) {
	merged := make(map[string]string)

	// Apply defaults
	for name, v := range def.Variables {
		if v.Default != "" {
			merged[name] = v.Default
		}
	}

	// Apply runtime overrides
	for k, v := range vars {
		merged[k] = v
	}

	// Check required
	for name, v := range def.Variables {
		if v.Required {
			if _, ok := merged[name]; !ok || merged[name] == "" {
				return nil, fmt.Errorf("required variable %q not provided", name)
			}
		}
	}

	return merged, nil
}
