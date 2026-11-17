// SPDX-License-Identifier: Apache-2.0

package api

// DECLARATION KEYS — a declaration refuses a key no field takes.
//
// Every CWM declaration kind (object type, capability, edge type, aspect,
// workflow, policy, hook, hook type) is decoded through its own codec, which
// checks the top-level keys of the declaration against the fields of its Go
// type. A key no field takes is refused, naming it: a removed key left in a
// pack, or a misspelt one, would otherwise be decoded into nothing and silently
// do nothing. The codec is the ONE strict path — the pack loader reaches it
// through YAML (config.DecodeBytes), the catalog reader through JSON — so the
// two readers never disagree on what a declaration may say.
//
// An object type is the one kind whose unknown MAP keys are legal (a
// capability's section, type_sections.go); every other kind has no sections.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// declKeyCache holds the key set of each declaration type, per codec tag.
var declKeyCache sync.Map // declKeyID -> map[string]bool

type declKeyID struct {
	t   reflect.Type
	tag string
}

// declarationKeys is the set of top-level keys the fields of t take under the
// codec tag ("json" or "yaml"); a field tagged "-" takes none.
func declarationKeys(t reflect.Type, tag string) map[string]bool {
	id := declKeyID{t, tag}
	if v, ok := declKeyCache.Load(id); ok {
		return v.(map[string]bool)
	}
	out := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get(tag), ",")
		if name != "" && name != "-" {
			out[name] = true
		}
	}
	declKeyCache.Store(id, out)
	return out
}

// unknownDeclarationKeys is every key of all no field of t takes, sorted.
func unknownDeclarationKeys(t reflect.Type, tag string, all map[string]any) []string {
	known := declarationKeys(t, tag)
	var bad []string
	for k := range all {
		if !known[k] {
			bad = append(bad, k)
		}
	}
	sort.Strings(bad)
	return bad
}

// refuseUnknownKeys names the keys a declaration of kind carries that no field
// takes, or returns nil.
func refuseUnknownKeys[F any](kind, name, tag string, all map[string]any) error {
	bad := unknownDeclarationKeys(reflect.TypeOf((*F)(nil)).Elem(), tag, all)
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("%s %q: %s is no field of a %s", kind, name, strings.Join(bad, ", "), kind)
}

// decodeDeclarationJSON decodes b into F (a declaration type stripped of its
// codec), refusing a top-level key no field of F takes.
func decodeDeclarationJSON[F any](kind string, b []byte, name func(*F) string) (F, error) {
	var f F
	if err := json.Unmarshal(b, &f); err != nil {
		return f, err
	}
	var all map[string]any
	if err := json.Unmarshal(b, &all); err != nil {
		return f, err
	}
	return f, refuseUnknownKeys[F](kind, name(&f), "json", all)
}

// decodeDeclarationYAML is decodeDeclarationJSON for a YAML node.
func decodeDeclarationYAML[F any](kind string, n *yaml.Node, name func(*F) string) (F, error) {
	var f F
	if err := n.Decode(&f); err != nil {
		return f, err
	}
	var all map[string]any
	if err := n.Decode(&all); err != nil {
		return f, err
	}
	return f, refuseUnknownKeys[F](kind, name(&f), "yaml", all)
}

// DecodeDeclarationJSON is the strict codec for a declaration kind another
// extension owns (a gate requirement): b decoded into F — the declaration type
// stripped of its codec — refusing a top-level key no field takes, naming it.
func DecodeDeclarationJSON[F any](kind string, b []byte, name func(*F) string) (F, error) {
	return decodeDeclarationJSON(kind, b, name)
}

// DecodeDeclarationYAML is DecodeDeclarationJSON for a YAML node.
func DecodeDeclarationYAML[F any](kind string, n *yaml.Node, name func(*F) string) (F, error) {
	return decodeDeclarationYAML(kind, n, name)
}

// The declaration types stripped of their codec — what the fields decode into.
type (
	capabilityFields Capability
	edgeTypeFields   EdgeTypeRegistration
	aspectFields     Aspect
	workflowFields   Workflow
	policyFields     Policy
	hookFields       Hook
	hookTypeFields   HookType
	eventTypeFields  EventType
)

func (c *Capability) UnmarshalJSON(b []byte) error {
	f, err := decodeDeclarationJSON("capability", b, func(f *capabilityFields) string { return f.Name })
	if err == nil {
		*c = Capability(f)
	}
	return err
}

func (c *Capability) UnmarshalYAML(n *yaml.Node) error {
	f, err := decodeDeclarationYAML("capability", n, func(f *capabilityFields) string { return f.Name })
	if err == nil {
		*c = Capability(f)
	}
	return err
}

func (e *EdgeTypeRegistration) UnmarshalJSON(b []byte) error {
	f, err := decodeDeclarationJSON("edge type", b, func(f *edgeTypeFields) string { return string(f.Name) })
	if err == nil {
		*e = EdgeTypeRegistration(f)
	}
	return err
}

func (e *EdgeTypeRegistration) UnmarshalYAML(n *yaml.Node) error {
	f, err := decodeDeclarationYAML("edge type", n, func(f *edgeTypeFields) string { return string(f.Name) })
	if err == nil {
		*e = EdgeTypeRegistration(f)
	}
	return err
}

func (a *Aspect) UnmarshalJSON(b []byte) error {
	f, err := decodeDeclarationJSON("aspect", b, func(f *aspectFields) string { return f.Name })
	if err == nil {
		*a = Aspect(f)
	}
	return err
}

func (a *Aspect) UnmarshalYAML(n *yaml.Node) error {
	f, err := decodeDeclarationYAML("aspect", n, func(f *aspectFields) string { return f.Name })
	if err == nil {
		*a = Aspect(f)
	}
	return err
}

func (w *Workflow) UnmarshalJSON(b []byte) error {
	f, err := decodeDeclarationJSON("workflow", b, func(f *workflowFields) string { return f.Name })
	if err == nil {
		*w = Workflow(f)
	}
	return err
}

func (w *Workflow) UnmarshalYAML(n *yaml.Node) error {
	f, err := decodeDeclarationYAML("workflow", n, func(f *workflowFields) string { return f.Name })
	if err == nil {
		*w = Workflow(f)
	}
	return err
}

func (p *Policy) UnmarshalJSON(b []byte) error {
	f, err := decodeDeclarationJSON("policy", b, func(f *policyFields) string { return f.Name })
	if err == nil {
		*p = Policy(f)
	}
	return err
}

func (p *Policy) UnmarshalYAML(n *yaml.Node) error {
	f, err := decodeDeclarationYAML("policy", n, func(f *policyFields) string { return f.Name })
	if err == nil {
		*p = Policy(f)
	}
	return err
}

func (h *Hook) UnmarshalJSON(b []byte) error {
	f, err := decodeDeclarationJSON("hook", b, func(f *hookFields) string { return f.Name })
	if err == nil {
		*h = Hook(f)
	}
	return err
}

func (h *Hook) UnmarshalYAML(n *yaml.Node) error {
	f, err := decodeDeclarationYAML("hook", n, func(f *hookFields) string { return f.Name })
	if err == nil {
		*h = Hook(f)
	}
	return err
}

func (t *HookType) UnmarshalJSON(b []byte) error {
	f, err := decodeDeclarationJSON("hook type", b, func(f *hookTypeFields) string { return string(f.Name) })
	if err == nil {
		*t = HookType(f)
	}
	return err
}

func (t *HookType) UnmarshalYAML(n *yaml.Node) error {
	f, err := decodeDeclarationYAML("hook type", n, func(f *hookTypeFields) string { return string(f.Name) })
	if err == nil {
		*t = HookType(f)
	}
	return err
}

func (t *EventType) UnmarshalJSON(b []byte) error {
	f, err := decodeDeclarationJSON("event type", b, func(f *eventTypeFields) string { return f.Name })
	if err == nil {
		*t = EventType(f)
	}
	return err
}

func (t *EventType) UnmarshalYAML(n *yaml.Node) error {
	f, err := decodeDeclarationYAML("event type", n, func(f *eventTypeFields) string { return f.Name })
	if err == nil {
		*t = EventType(f)
	}
	return err
}

// declarationFields is the type each catalog kind's fields decode into.
var declarationFields = map[string]reflect.Type{
	"capability":  reflect.TypeOf(capabilityFields{}),
	"edge-type":   reflect.TypeOf(edgeTypeFields{}),
	"aspect":      reflect.TypeOf(aspectFields{}),
	"workflow":    reflect.TypeOf(workflowFields{}),
	"policy":      reflect.TypeOf(policyFields{}),
	"hook":        reflect.TypeOf(hookFields{}),
	"hook-type":   reflect.TypeOf(hookTypeFields{}),
	"event-type":  reflect.TypeOf(eventTypeFields{}),
	"object-type": reflect.TypeOf(objectTypeFields{}),
}

// LenientDeclarationJSON is how a declaration written in a NEWER MINOR of its
// kind's format is read (contribapi.JudgeFormat): the top-level keys no field
// of the kind takes are dropped — for an object type, the ones that are not a
// section (a section is an object) — and named, sorted, so the reader reports
// what it ignored. The stripped body then decodes through the strict codec as
// any other. A kind CWM does not read comes back as it is.
func LenientDeclarationJSON(kind string, b []byte) ([]byte, []string, error) {
	t, ok := declarationFields[kind]
	if !ok {
		return b, nil, nil
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, nil, err
	}
	known := declarationKeys(t, "json")
	var dropped []string
	for k, v := range all {
		if known[k] {
			continue
		}
		if kind == "object-type" && len(v) > 0 && v[0] == '{' {
			continue // a section: the grammar takes it
		}
		dropped = append(dropped, k)
		delete(all, k)
	}
	if len(dropped) == 0 {
		return b, nil, nil
	}
	sort.Strings(dropped)
	out, err := json.Marshal(all)
	return out, dropped, err
}
