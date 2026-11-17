// SPDX-License-Identifier: Apache-2.0

package schemaapi

// SchemaFormat is the wire format of a DSL schema.
type SchemaFormat string

const (
	FormatAuto SchemaFormat = "" // try JSON first, fall back to YAML
	FormatYAML SchemaFormat = "yaml"
	FormatJSON SchemaFormat = "json"
)

// ParseInput carries a raw schema body plus parsing options.
//
// Lives in core/schema/api alongside the parser so the contribution
// layer can invoke the parser at AppInit without depending on the
// SchemaService wire surface. The SchemaService.Parse method in
// extensions/schema/api accepts the same shape and forwards to the
// core parser.
type ParseInput struct {
	Body     []byte       `json:"body"               desc:"Raw YAML or JSON schema bytes"  required:"true"`
	Format   SchemaFormat `json:"format,omitempty"   desc:"yaml | json (auto-detect if empty)"`
	BasePath string       `json:"basePath,omitempty" desc:"Base directory for resolving imports"`
	Strict   bool         `json:"strict,omitempty"   desc:"Fail on unknown fields and warnings"`
}

// ParseFileInput parses a schema file from disk.
type ParseFileInput struct {
	Path   string       `json:"path"             cli:"arg"          desc:"Schema file path (.yaml, .yml, .json)"  required:"true"`
	Format SchemaFormat `json:"format,omitempty" cli:"format,t"     desc:"yaml | json (default: from extension)"`
	Strict bool         `json:"strict,omitempty" cli:"strict"       desc:"Fail on unknown fields and warnings"`
	Quiet  bool         `json:"quiet,omitempty"  cli:"quiet,q"      desc:"Suppress AST output (validation only)"`
}

// ParseOutput is the canonical AST plus optional non-fatal diagnostics.
type ParseOutput struct {
	Schema      *Schema      `json:"schema"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// ValidateInput re-validates a schema produced earlier.
type ValidateInput struct {
	Schema *Schema `json:"schema" desc:"Schema AST to validate" required:"true"`
}

// ValidateOutput reports validation findings.
type ValidateOutput struct {
	Valid       bool         `json:"valid"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}
