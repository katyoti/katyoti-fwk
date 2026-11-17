// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// ObjectFilter answers "does this apply to that object" for the declarations
// that select objects: object panels, `hook` contributions and a hook entry's
// `filter`, agent-contexts — and, through ShapeFilter, capability `attach`,
// policy `applies` and a gate requirement's `applies`. Action `target` and a
// task's applicability (`kind`/`types`/`filter`, parsed by the task extension)
// keep their own selectors.
//
// Fields AND together; an absent field does not constrain, so the zero filter
// matches every object. Within a list it is any-of: one listed type, one listed
// capability, one edge of one listed kind is enough.
//
// Incoming / Outgoing select by what points AT the object (or what it points
// at): a challenge's panel shows on the object it challenges. They are answered
// from the edges CWM's index holds both ways — never by a graph read inside a
// `when`. Deliberately NOT here (decided 2026-09-25):
//   - requires — "the object must carry all of" is an include, not a filter;
//   - aspects — an aspect IS a list of capabilities, which Capabilities says;
//   - modules — a runtime name, not a contribution name; when it matters it is
//     one `when` away.
type ObjectFilter struct {
	Types        []string `json:"types,omitempty"        yaml:"types,omitempty"        jsonschema:"description=Object types it applies to (any of); * means every type"`
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty" jsonschema:"description=Capabilities the object must carry at least one of (effective set: type + aspects + instance)"`
	Incoming     []string `json:"incoming,omitempty"     yaml:"incoming,omitempty"     jsonschema:"description=Edge kinds of which the object must have at least one incoming edge (any of)"`
	Outgoing     []string `json:"outgoing,omitempty"     yaml:"outgoing,omitempty"     jsonschema:"description=Edge kinds of which the object must have at least one outgoing edge (any of)"`
	When         string   `json:"when,omitempty"         yaml:"when,omitempty"         jsonschema:"description=el predicate over the object vocabulary (object / current / id / type / parent_id / children / links)"`
}

// FilterSubject is what a filter is asked about. Eval evaluates the `when`
// predicate in the object vocabulary; it is supplied by the caller so this
// package stays free of an expression engine, and so the ONE evaluator the
// core uses for every predicate is the one that runs here too.
//
// Incoming and Outgoing are the kinds of the object's incident edges, each
// side — filled by WithEdges from the edges EdgeList (or CWM's index) returns.
type FilterSubject struct {
	Type         string
	Capabilities []string
	Incoming     []string
	Outgoing     []string
	Eval         func(expr string) (bool, error)
}

// WithEdges fills the subject's edge kinds from the object's incident edges:
// an edge whose target is id is incoming, one whose source is id is outgoing (a
// self-edge is both). The one way every caller turns an object's links into
// what a filter reads.
func (s FilterSubject) WithEdges(id string, edges []Edge) FilterSubject {
	s.Incoming, s.Outgoing = nil, nil
	for _, e := range edges {
		k := string(e.Kind)
		if e.To == id && !slices.Contains(s.Incoming, k) {
			s.Incoming = append(s.Incoming, k)
		}
		if e.From == id && !slices.Contains(s.Outgoing, k) {
			s.Outgoing = append(s.Outgoing, k)
		}
	}
	return s
}

// ReadsEdges reports a filter that selects by edge, so a caller fetches the
// object's edges only when one asks.
func (f ObjectFilter) ReadsEdges() bool { return len(f.Incoming) > 0 || len(f.Outgoing) > 0 }

// Matches reports whether the filter admits the subject, and when it does
// not, which field refused — the answer to "why is this panel missing".
func (f ObjectFilter) Matches(s FilterSubject) (bool, string) {
	if len(f.Types) > 0 && !slices.Contains(f.Types, "*") && !slices.Contains(f.Types, s.Type) {
		return false, fmt.Sprintf("type %q is not one of [%s]", s.Type, strings.Join(f.Types, ", "))
	}
	if len(f.Capabilities) > 0 && !slices.ContainsFunc(f.Capabilities, func(c string) bool { return slices.Contains(s.Capabilities, c) }) {
		return false, fmt.Sprintf("carries none of [%s]", strings.Join(f.Capabilities, ", "))
	}
	if len(f.Incoming) > 0 && !slices.ContainsFunc(f.Incoming, func(k string) bool { return slices.Contains(s.Incoming, k) }) {
		return false, fmt.Sprintf("no incoming edge of kind [%s]", strings.Join(f.Incoming, ", "))
	}
	if len(f.Outgoing) > 0 && !slices.ContainsFunc(f.Outgoing, func(k string) bool { return slices.Contains(s.Outgoing, k) }) {
		return false, fmt.Sprintf("no outgoing edge of kind [%s]", strings.Join(f.Outgoing, ", "))
	}
	if f.When != "" {
		if s.Eval == nil {
			return false, "when: no evaluator"
		}
		ok, err := s.Eval(f.When)
		if err != nil {
			// A predicate that cannot be read does NOT admit: showing a panel
			// whose condition failed would be the same mistake as enabling an
			// action whose guard failed.
			return false, "when: " + err.Error()
		}
		if !ok {
			return false, "when is false: " + f.When
		}
	}
	return true, ""
}

// objectFilterFields is ObjectFilter stripped of its codec.
type objectFilterFields ObjectFilter

// A filter refuses a key no field takes, naming it, like every declaration
// (declaration_keys.go): a misspelt `incomming:` would otherwise decode into
// nothing and admit every object.
func refuseUnknownFilterKeys(tag string, all map[string]any) error {
	bad := unknownDeclarationKeys(reflect.TypeOf(objectFilterFields{}), tag, all)
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("filter: %s is no field of a filter (types, capabilities, incoming, outgoing, when)", strings.Join(bad, ", "))
}

func (f *ObjectFilter) UnmarshalJSON(b []byte) error {
	var v objectFilterFields
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	var all map[string]any
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	if err := refuseUnknownFilterKeys("json", all); err != nil {
		return err
	}
	*f = ObjectFilter(v)
	return nil
}

func (f *ObjectFilter) UnmarshalYAML(n *yaml.Node) error {
	var v objectFilterFields
	if err := n.Decode(&v); err != nil {
		return err
	}
	var all map[string]any
	if err := n.Decode(&all); err != nil {
		return err
	}
	if err := refuseUnknownFilterKeys("yaml", all); err != nil {
		return err
	}
	*f = ObjectFilter(v)
	return nil
}

// ShapeFilter is an ObjectFilter that reads what an object IS — its type and
// the capabilities it carries — and never what it HOLDS. It is the selector of
// the declarations that decide what an object carries or must satisfy:
// capability `attach`, policy `applies`, a gate requirement's `applies`.
//
// THE RULE: an object's shape must not depend on its content. What is
// attached to an object, the rules put on it and what its definition of done
// asks are read before any field, edge or state is — a selector over content
// would make them move under every edit (a capability appearing because a
// field changed, a requirement vanishing so a gate passes) and could not be
// validated without the content it decides. So `when`, `incoming` and
// `outgoing` are refused at decode, naming the field: a condition on content
// belongs in a panel's or an action's `when`, or in a requirement's check.
// Types and capabilities are declarations (the type's, its aspects', the groups
// the object was given), not content.
//
// It decodes through ObjectFilter (one decoder, unknown keys refused) and
// answers through ObjectFilter.Matches (one matcher, one reason).
type ShapeFilter struct {
	Types        []string `json:"types,omitempty"        yaml:"types,omitempty"        jsonschema:"description=Object types it selects (any of); * means every type"`
	Capabilities []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty" jsonschema:"description=Capabilities the object must carry at least one of"`
}

// Filter is the ObjectFilter this shape filter is.
func (f ShapeFilter) Filter() ObjectFilter {
	return ObjectFilter{Types: f.Types, Capabilities: f.Capabilities}
}

// IsZero reports a filter that names nothing.
func (f ShapeFilter) IsZero() bool { return len(f.Types) == 0 && len(f.Capabilities) == 0 }

// Matches is ObjectFilter.Matches: whether the subject is admitted, and when
// not, which field refused.
func (f ShapeFilter) Matches(s FilterSubject) (bool, string) { return f.Filter().Matches(s) }

// shape keeps a decoded ObjectFilter's shape fields, refusing its content ones.
func shape(o ObjectFilter) (ShapeFilter, error) {
	var bad []string
	if len(o.Incoming) > 0 {
		bad = append(bad, "incoming")
	}
	if len(o.Outgoing) > 0 {
		bad = append(bad, "outgoing")
	}
	if o.When != "" {
		bad = append(bad, "when")
	}
	if len(bad) > 0 {
		return ShapeFilter{}, fmt.Errorf("filter: %s refused here: this selector decides what an object carries or must satisfy, "+
			"so it reads the object's shape (types, capabilities), never its content — put a condition on content in a panel's or an action's when, or in a requirement's check",
			strings.Join(bad, ", "))
	}
	return ShapeFilter{Types: o.Types, Capabilities: o.Capabilities}, nil
}

func (f *ShapeFilter) UnmarshalJSON(b []byte) error {
	var o ObjectFilter
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	v, err := shape(o)
	if err == nil {
		*f = v
	}
	return err
}

func (f *ShapeFilter) UnmarshalYAML(n *yaml.Node) error {
	var o ObjectFilter
	if err := n.Decode(&o); err != nil {
		return err
	}
	v, err := shape(o)
	if err == nil {
		*f = v
	}
	return err
}
