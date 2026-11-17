// SPDX-License-Identifier: Apache-2.0

// In-memory data-type store. Holds parsed Schema ASTs keyed by URI,
// with a lazy-compiled JSON Schema cache. Thread-safe.
//
// Initialised at hub bootstrap (Phase 1 of (modules/contributions/decisions/OD-006-in-what-order-does-the-hub-build-its-catalog-and.cwm.md))
// from `data-type` Contributions in the catalog; subsequent additions
// arrive via RegisterDataType from the bootstrap factory + the refresh
// path.

package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	"github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// dataTypeStore is the in-memory data-type registry.
type dataTypeStore struct {
	mu sync.RWMutex

	// schemas holds the parsed AST per URI. The Schema is the parser's
	// canonical AST (the same shape Parse returns), so consumers get
	// the full AST + extensions when fetching.
	schemas map[string]*schemaapi.Schema

	// jsonSchemaCache memoises ToJSONSchema() output. Invalidated when
	// the URI is re-registered.
	jsonSchemaCache map[string]json.RawMessage

	// validators memoises what ValidateAgainst needs per data-type: the
	// registered AST's own diagnostics and its compiled JSON Schema. Both are
	// functions of the registration alone, and compiling a JSON Schema (which
	// validates it against the 2020-12 meta-schema first) is by far the
	// costliest step: done per body, it made a catalog reindex — one
	// ValidateAgainst per contribution, twice at every hub boot — cost
	// seconds. Invalidated when the URI is re-registered.
	validators map[string]*bodyValidator
}

// bodyValidator is a data-type's compiled validation state: the AST's
// diagnostics (a schema with an error validates nothing) and, when the AST is
// sound, its compiled JSON Schema — or the error compiling it. A compiled
// jsonschema.Schema is read-only once built, so one is shared by every
// concurrent ValidateAgainst.
type bodyValidator struct {
	diags      []schemaapi.Diagnostic
	astValid   bool
	compiled   *jsonschema.Schema
	compileErr error
}

func newDataTypeStore() *dataTypeStore {
	return &dataTypeStore{
		schemas:         make(map[string]*schemaapi.Schema),
		jsonSchemaCache: make(map[string]json.RawMessage),
		validators:      make(map[string]*bodyValidator),
	}
}

// register parses the body as a DSL schema and inserts/updates the
// store entry. Overwrites the previous registration if URI already
// exists; invalidates the JSON Schema cache for that URI.
func (s *dataTypeStore) register(uri string, parsed *schemaapi.Schema) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.schemas[uri] = parsed
	delete(s.jsonSchemaCache, uri)
	delete(s.validators, uri)
}

// get returns the parsed AST for a URI, or (nil, ErrDataTypeNotFound).
func (s *dataTypeStore) get(uri string) (*schemaapi.Schema, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	schema, ok := s.schemas[uri]
	if !ok {
		return nil, api.ErrDataTypeNotFound
	}
	return schema, nil
}

// getJSONSchema returns the compiled JSON Schema for a URI. Lazy-compiles
// on first request, caches the result.
func (s *dataTypeStore) getJSONSchema(uri string) (json.RawMessage, error) {
	s.mu.RLock()
	if cached, ok := s.jsonSchemaCache[uri]; ok {
		s.mu.RUnlock()
		return cached, nil
	}
	schema, ok := s.schemas[uri]
	s.mu.RUnlock()
	if !ok {
		return nil, api.ErrDataTypeNotFound
	}
	compiled, err := schema.ToJSONSchema()
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", uri, err)
	}
	s.mu.Lock()
	s.jsonSchemaCache[uri] = compiled
	s.mu.Unlock()
	return compiled, nil
}

// getValidator returns the data-type's compiled validation state, building it
// on first request and caching it until the URI is re-registered. Two callers
// racing on a cold entry may both build it; the result is the same, so the
// second write is harmless.
func (s *dataTypeStore) getValidator(uri string) (*bodyValidator, error) {
	s.mu.RLock()
	if v, ok := s.validators[uri]; ok {
		s.mu.RUnlock()
		return v, nil
	}
	schema, ok := s.schemas[uri]
	s.mu.RUnlock()
	if !ok {
		return nil, api.ErrDataTypeNotFound
	}
	v := &bodyValidator{diags: parser.RunValidator(schema), astValid: true}
	for _, d := range v.diags {
		if d.Severity == schemaapi.SeverityError {
			v.astValid = false
		}
	}
	if v.astValid {
		v.compiled, v.compileErr = s.compileJSONSchema(uri)
	}
	s.mu.Lock()
	// Keep the entry only if the registration it was built from is still the
	// current one: a re-register meanwhile must not be shadowed by a stale build.
	if s.schemas[uri] == schema {
		s.validators[uri] = v
	}
	s.mu.Unlock()
	return v, nil
}

// compileJSONSchema compiles the data-type's JSON Schema (2020-12) projection.
func (s *dataTypeStore) compileJSONSchema(uri string) (*jsonschema.Schema, error) {
	js, err := s.getJSONSchema(uri)
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", bytes.NewReader(js)); err != nil {
		return nil, err
	}
	return compiler.Compile("schema.json")
}

// list returns the URIs of every registered data-type.
func (s *dataTypeStore) list() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	uris := make([]string, 0, len(s.schemas))
	for uri := range s.schemas {
		uris = append(uris, uri)
	}
	return uris
}
