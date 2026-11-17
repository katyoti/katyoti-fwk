// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// CAPABILITY GROUPS AGAINST THEIR SCHEMA. CWM guarantees every stored object is
// valid, and a capability's group is valid when it conforms to the schema the
// capability declares (`schema:`, a data-type). The host compiles the schemas
// from the catalog (CatalogTypeSet.CapabilitySchemas); CWM checks every group
// of an object on create and update — a transition writes through update — so
// the check never calls out.

// mergeCapabilitySchemas compiles newly-appeared and changed schemas: a
// changed schema is in force at the refresh that brings it. A schema that does not compile is logged and left out; the
// groups it would check are then reported unresolved, never silently passed.
func (s *Service) mergeCapabilitySchemas(schemas map[string]json.RawMessage) {
	if len(schemas) == 0 {
		return
	}
	s.capSchemasMu.Lock()
	defer s.capSchemasMu.Unlock()
	if s.capSchemas == nil {
		s.capSchemas = map[string]*jsonschema.Schema{}
		s.capSchemaRaw = map[string]json.RawMessage{}
	}
	for uri, raw := range schemas {
		// A schema already compiled from the same source stays; a changed one
		// is compiled again and replaces it.
		if _, done := s.capSchemas[uri]; (done && bytes.Equal(s.capSchemaRaw[uri], raw)) || len(raw) == 0 {
			continue
		}
		compiled, err := compileSchema(uri, raw)
		if err != nil {
			slog.Warn("cwm: a capability schema does not compile; groups using it cannot be checked", "schema", uri, "error", err)
			continue
		}
		s.capSchemas[uri] = compiled
		s.capSchemaRaw[uri] = raw
	}
}

// schemaDefaults returns the top-level `default` of every property the schema
// declares one for — what giving a capability fills in when the caller does
// not.
func (s *Service) schemaDefaults(uri string) map[string]any {
	s.capSchemasMu.RLock()
	raw := s.capSchemaRaw[uri]
	s.capSchemasMu.RUnlock()
	type node struct {
		Ref        string `json:"$ref"`
		Properties map[string]struct {
			Default any `json:"default"`
		} `json:"properties"`
		Defs map[string]node `json:"$defs"`
	}
	var sch node
	if len(raw) == 0 || json.Unmarshal(raw, &sch) != nil {
		return nil
	}
	// A compiled data-type roots at `$ref: #/$defs/<Root>`.
	root := sch
	if name, ok := strings.CutPrefix(sch.Ref, "#/$defs/"); ok {
		root = sch.Defs[name]
	}
	out := map[string]any{}
	for name, p := range root.Properties {
		if p.Default != nil {
			out[name] = p.Default
		}
	}
	return out
}

func compileSchema(uri string, raw json.RawMessage) (*jsonschema.Schema, error) {
	c := jsonschema.NewCompiler()
	res := "cwm-capability:" + uri
	if err := c.AddResource(res, bytes.NewReader(raw)); err != nil {
		return nil, err
	}
	return c.Compile(res)
}

// capabilitySchema returns the compiled schema for a data-type URI.
func (s *Service) capabilitySchema(uri string) (*jsonschema.Schema, bool) {
	s.capSchemasMu.RLock()
	defer s.capSchemasMu.RUnlock()
	sch, ok := s.capSchemas[uri]
	return sch, ok
}

// GroupViolation is one capability group that does not conform to its schema.
type GroupViolation struct {
	Capability string
	Schema     string
	Problems   []string
}

func (v GroupViolation) String() string {
	return fmt.Sprintf("capability %q (%s): %s", v.Capability, v.Schema, strings.Join(v.Problems, "; "))
}

// groupViolations checks every group of obj whose capability declares a
// schema. unresolved lists the schema URIs no compiled schema answers for.
func (s *Service) groupViolations(obj api.Object) (violations []GroupViolation, unresolved []string) {
	declared := s.capabilitiesByName()
	ids := make([]string, 0, len(obj.Capabilities))
	for id := range obj.Capabilities {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c, ok := declared[id]
		if !ok || c.Schema == "" {
			continue // an undeclared group is checkInstanceCapabilities' to report
		}
		sch, ok := s.capabilitySchema(c.Schema)
		if !ok {
			unresolved = append(unresolved, c.Schema)
			continue
		}
		// Normalise through JSON: a group read from YAML holds Go values the
		// validator does not know (int, map[string]any nested in interfaces).
		raw, err := json.Marshal(obj.Capabilities[id])
		if err != nil {
			violations = append(violations, GroupViolation{Capability: id, Schema: c.Schema, Problems: []string{err.Error()}})
			continue
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			violations = append(violations, GroupViolation{Capability: id, Schema: c.Schema, Problems: []string{err.Error()}})
			continue
		}
		if err := sch.Validate(v); err != nil {
			violations = append(violations, GroupViolation{Capability: id, Schema: c.Schema, Problems: validationProblems(err)})
		}
	}
	// The type's own fields, against the type's `fields:` data-type.
	if t, ok := s.lookupObjectType(obj.Type); ok && t.Fields != "" && len(obj.Fields) > 0 {
		sch, ok := s.capabilitySchema(t.Fields)
		if !ok {
			return violations, append(unresolved, t.Fields)
		}
		raw, err := json.Marshal(obj.Fields)
		var v any
		if err == nil {
			err = json.Unmarshal(raw, &v)
		}
		if err == nil {
			err = sch.Validate(v)
		}
		if err != nil {
			violations = append(violations, GroupViolation{Capability: api.FieldsPanel, Schema: t.Fields, Problems: validationProblems(err)})
		}
	}
	return violations, unresolved
}

// validationProblems flattens a validation error into "<where>: <what>" lines.
func validationProblems(err error) []string {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []string{err.Error()}
	}
	var out []string
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			where := e.InstanceLocation
			if where == "" {
				where = "/"
			}
			out = append(out, where+": "+e.Message)
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	if len(out) == 0 {
		out = append(out, ve.Message)
	}
	return out
}

// checkCapabilityGroups refuses (error mode) or logs (warn mode) an object
// whose capability groups do not conform to their schemas. A schema no
// compiled schema answers for is logged once per URI: the group cannot be
// checked, and saying so is the difference between "valid" and "unchecked".
func (s *Service) checkCapabilityGroups(obj api.Object, mode, verb string) error {
	violations, unresolved := s.groupViolations(obj)
	for _, uri := range unresolved {
		if _, seen := s.unresolvedSchemas.LoadOrStore(uri, true); !seen {
			slog.Warn("cwm: a capability schema is not resolved; its groups are not checked", "schema", uri)
		}
	}
	if len(violations) == 0 {
		return nil
	}
	parts := make([]string, len(violations))
	for i, v := range violations {
		parts[i] = v.String()
	}
	if mode == api.TypeValidationError {
		return fmt.Errorf("%w: %s: object %q is not valid: %s", api.ErrInvalidInput, verb, obj.ID, strings.Join(parts, " | "))
	}
	slog.Warn("cwm: object is not valid", "verb", verb, "id", obj.ID, "problems", strings.Join(parts, " | "),
		"hint", "this write is refused once validation runs in error mode")
	return nil
}
