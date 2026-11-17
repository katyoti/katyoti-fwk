// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
)

// MethodOpts holds optional metadata for a method definition.
//
// InputSchema/OutputSchema are stored as `any` (the decoded JSON Schema tree)
// so downstream encoders can serialize them as structured output for both
// JSON and YAML. See MethodInfo for the full rationale.
type MethodOpts struct {
	Desc         string
	Tags         MethodTags
	InputSchema  any
	OutputSchema any
	CLIMeta      *CLIMeta
	// RenderTemplate is a Go text/template for rich CLI output (empty = JSON
	// or ui-cli rendering, depending on OutputAST). Kept for the existing
	// hand-coded-template path until all callers migrate to `ui-cli`.
	RenderTemplate string
	// OutputAST is the canonical Type AST for the method's output type,
	// stored opaquely as `any` to keep this package free of an import on
	// `internal/extensions/schema/api`. AutoKTSP populates it via
	// `reflect.TypeAST[OutputType]()` at slot-build time; the CLI
	// renderer in `internal/extensions/tui/render/` type-asserts back to
	// `*schemaapi.Type` at command-run time and dispatches on the
	// attached `ui-cli` extension. Nil = no AST captured (manually
	// declared slot methods) → renderer falls back to JSON.
	OutputAST any
	// TimeoutS is the per-method request timeout in seconds advertised to
	// callers. Zero = transport default. See meta.MethodDef.TimeoutS.
	TimeoutS int
	// Live marks a method that emits progress snapshots; the CLI installs a
	// live terminal renderer for it (rich + TTY). See meta.MethodDef.Live.
	Live bool
}

// MethodDef is the rich method descriptor. It bundles a handler with
// all its metadata: schemas, tags, CLI mapping. Generators consume this.
type MethodDef struct {
	name    string
	handler Handler
	opts    MethodOpts
}

// Name returns the method name.
func (m *MethodDef) Name() string { return m.name }

// Handler returns the underlying handler function.
func (m *MethodDef) Handler() Handler { return m.handler }

// Opts returns the method options.
func (m *MethodDef) Opts() MethodOpts { return m.opts }

// Info returns a MethodInfo populated from the definition.
func (m *MethodDef) Info() MethodInfo {
	return MethodInfo{
		Name:         m.name,
		Description:  m.opts.Desc,
		InputSchema:  m.opts.InputSchema,
		OutputSchema: m.opts.OutputSchema,
		Tags:         m.opts.Tags,
		TimeoutS:     m.opts.TimeoutS,
	}
}

// GetCLIMeta returns the CLI metadata, or nil if not available.
func (m *MethodDef) GetCLIMeta() *CLIMeta { return m.opts.CLIMeta }

// Method creates a typed MethodDef. The handler function receives a typed
// input struct and returns a typed output struct. Schemas are provided
// externally (from generated code), NOT via reflection.
func Method[In, Out any](name string, opts MethodOpts, fn func(ctx context.Context, in In) (Out, error)) *MethodDef {
	h := func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
		var in In
		if params != nil {
			if err := json.Unmarshal(params, &in); err != nil {
				return nil, ErrInvalidParams(err.Error())
			}
		}
		out, err := fn(ctx, in)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}
	return &MethodDef{name: name, handler: h, opts: opts}
}

// RawMethod creates a MethodDef from an untyped handler with provided schemas.
func RawMethod(name string, opts MethodOpts, handler Handler) *MethodDef {
	return &MethodDef{name: name, handler: handler, opts: opts}
}
