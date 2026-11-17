// SPDX-License-Identifier: Apache-2.0

// Package parser implements the data-DSL pipeline:
//
//	bytes ──> decodeOuter (S2) ──> resolveImports (S3) ──> expandShorthand (S4) ──> validate (S5) ──> *schemaapi.Schema
//
// Each stage operates on the rawSchema intermediate form except the last,
// which produces the typed canonical AST.
package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"gopkg.in/yaml.v3"
)

// rawSchema is the intermediate form produced by the outer decoder. The
// `Types` map values are kept as `any` because each entry may be a shorthand
// string ("string@email?", "Item[]", "->User") or a full block (map).
//
// Shorthand expansion happens in S4; until then the raw form is passed
// through resolveImports unchanged.
type rawSchema struct {
	DSLVersion  string         `yaml:"$schema,omitempty"  json:"$schema,omitempty"`
	Name        string         `yaml:"name"               json:"name"`
	Version     string         `yaml:"version,omitempty"  json:"version,omitempty"`
	Description string         `yaml:"description,omitempty" json:"description,omitempty"`
	Imports     []string       `yaml:"imports,omitempty"  json:"imports,omitempty"`
	Root        string         `yaml:"root,omitempty"     json:"root,omitempty"`
	Types       map[string]any `yaml:"types"              json:"types"`

	// resolver state — not part of the wire format
	basePath   string
	sourcePath string // for error messages and cycle detection
}

// Parse reads a YAML or JSON DSL schema from the input bytes and returns the
// canonical AST. Format is auto-detected unless in.Format is set.
//
// This is the public entry point. It runs the full pipeline:
//
//	decode → imports → shorthand → validate → AST
//
// Stages 3/4/5 are stubbed at S2 and filled in by subsequent subtasks.
func Parse(ctx context.Context, in schemaapi.ParseInput) (schemaapi.ParseOutput, error) {
	raw, err := decodeOuter(in.Body, in.Format)
	if err != nil {
		return schemaapi.ParseOutput{}, err
	}
	raw.basePath = in.BasePath

	return finalize(ctx, raw, in.Strict)
}

// ParseFile reads a schema file from disk and parses it. The file's parent
// directory becomes the import base path, and the format is inferred from
// the extension unless in.Format is set explicitly.
func ParseFile(ctx context.Context, in schemaapi.ParseFileInput) (schemaapi.ParseOutput, error) {
	abs, err := filepath.Abs(in.Path)
	if err != nil {
		return schemaapi.ParseOutput{}, fmt.Errorf("resolve path %q: %w", in.Path, err)
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return schemaapi.ParseOutput{}, fmt.Errorf("read schema file: %w", err)
	}

	format := in.Format
	if format == schemaapi.FormatAuto {
		format = formatFromExt(abs)
	}

	raw, err := decodeOuter(body, format)
	if err != nil {
		return schemaapi.ParseOutput{}, fmt.Errorf("parse %s: %w", in.Path, err)
	}
	raw.basePath = filepath.Dir(abs)
	raw.sourcePath = abs

	return finalize(ctx, raw, in.Strict)
}

// finalize runs stages 3, 4, 5 of the pipeline. At S2 these are stubs;
// later subtasks replace the bodies with real implementations.
func finalize(ctx context.Context, raw *rawSchema, strict bool) (schemaapi.ParseOutput, error) {
	_ = ctx
	_ = strict
	if err := resolveImports(raw); err != nil {
		return schemaapi.ParseOutput{}, err
	}
	schema, diags, err := expandShorthand(raw)
	if err != nil {
		return schemaapi.ParseOutput{}, err
	}
	vDiags := validate(schema)
	diags = append(diags, vDiags...)

	for _, d := range diags {
		if d.Severity == schemaapi.SeverityError {
			return schemaapi.ParseOutput{Schema: schema, Diagnostics: diags},
				fmt.Errorf("schema validation failed: %s", d.Message)
		}
	}
	return schemaapi.ParseOutput{Schema: schema, Diagnostics: diags}, nil
}

// decodeOuter reads the outer schema structure into rawSchema. Auto-detection
// (when format == FormatAuto): try JSON first (cheap, fail-fast on invalid
// JSON), fall back to YAML. JSON inputs always parse as YAML too, but doing
// JSON first preserves number precision for integer literals.
func decodeOuter(body []byte, format schemaapi.SchemaFormat) (*rawSchema, error) {
	body = stripBOM(body)
	if len(body) == 0 {
		return nil, fmt.Errorf("empty schema body")
	}

	switch format {
	case schemaapi.FormatJSON:
		return decodeJSON(body)
	case schemaapi.FormatYAML:
		return decodeYAML(body)
	case schemaapi.FormatAuto:
		// heuristic: leading '{' or '[' → JSON; anything else → YAML.
		// Avoids the cost of a failed JSON parse on long YAML files.
		if looksLikeJSON(body) {
			s, err := decodeJSON(body)
			if err == nil {
				return s, nil
			}
			// fall through to YAML on JSON failure (rare; e.g. YAML flow style
			// happens to start with '{').
		}
		return decodeYAML(body)
	default:
		return nil, fmt.Errorf("unknown schema format %q", string(format))
	}
}

func decodeYAML(body []byte) (*rawSchema, error) {
	var s rawSchema
	if err := yaml.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	if err := s.validateOuter(); err != nil {
		return nil, err
	}
	// yaml.v3 decodes nested maps as map[string]any (good); but inside `Types`
	// values, mappings come back as map[string]any already. No conversion needed.
	return &s, nil
}

func decodeJSON(body []byte) (*rawSchema, error) {
	var s rawSchema
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	if err := s.validateOuter(); err != nil {
		return nil, err
	}
	return &s, nil
}

// validateOuter checks the required outer fields. Type-level validation
// (orphan refs, etc.) lives in the validator stage.
func (s *rawSchema) validateOuter() error {
	if s.Name == "" {
		return fmt.Errorf("schema: %q is required", "name")
	}
	if s.Types == nil {
		return fmt.Errorf("schema %q: %q is required", s.Name, "types")
	}
	return nil
}

// formatFromExt infers the schema format from a file extension. Anything
// other than .json defaults to YAML (since YAML is the canonical surface).
func formatFromExt(path string) schemaapi.SchemaFormat {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return schemaapi.FormatJSON
	default:
		return schemaapi.FormatYAML
	}
}

func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

func looksLikeJSON(b []byte) bool {
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		return c == '{' || c == '['
	}
	return false
}
