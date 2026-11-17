// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// scopeDecl: ktw-a owns the type `doc` and the capability `signed`; ktw-b
// owns the type `note`.
func scopeDecl() Declarations {
	return Declarations{
		Objects: []ObjectType{
			{Name: "doc", Provider: "ktw-a"},
			{Name: "note", Provider: "ktw-b"},
		},
		Capabilities: []Capability{{Name: "signed", Provider: "ktw-a"}},
	}
}

func TestScopeApplies(t *testing.T) {
	sc := NewScope(scopeDecl(), Grants{"ktw-g": cenv.GateGrant{Types: []string{"note"}}, "ktw-c": cenv.GateGrant{Capabilities: []string{"signed"}}})
	for _, row := range []struct {
		name, provider, typ string
		caps                []CapabilityRef
		want                bool
	}{
		{"its own type", "ktw-a", "doc", nil, true},
		{"a foreign type", "ktw-a", "note", nil, false},
		{"a foreign type carrying its capability by choice (type)", "ktw-a", "note", []CapabilityRef{{ID: "signed", Source: "type"}}, true},
		{"by choice (aspect)", "ktw-a", "note", []CapabilityRef{{ID: "signed", Source: "aspect:isa"}}, true},
		{"by choice (instance)", "ktw-a", "note", []CapabilityRef{{ID: "signed", Source: "object"}}, true},
		{"its own attach does not count", "ktw-a", "note", []CapabilityRef{{ID: "signed", Source: "attached"}}, false},
		{"a granted type", "ktw-g", "note", nil, true},
		{"a type not granted", "ktw-g", "doc", nil, false},
		{"a granted capability, however carried", "ktw-c", "note", []CapabilityRef{{ID: "signed", Source: "attached"}}, true},
		{"no provider (the object's own)", "", "note", nil, true},
	} {
		got, why := sc.Applies(row.provider, row.typ, row.caps)
		if got != row.want {
			t.Errorf("%s: Applies = %v (%s), want %v", row.name, got, why, row.want)
		}
		if !got && !strings.Contains(why, "grants: {gate:") {
			t.Errorf("%s: the reason must name the grant to add: %s", row.name, why)
		}
	}
}

func TestScopeOf(t *testing.T) {
	gate := Hooks{HookBeforeUpdate: {{URI: "x:y/z"}}}
	after := Hooks{HookOnUpdate: {{URI: "x:y/z"}}}
	for _, row := range []struct {
		name   string
		add    func(*Declarations)
		grants Grants
		want   []string // kind/name of each finding
	}{
		{"a hook on its own type", func(d *Declarations) {
			d.Hooks = append(d.Hooks, Hook{Name: "h", Type: HookBeforeUpdate, URI: "x:y/z", Provider: "ktw-a", Filter: ObjectFilter{Types: []string{"doc"}}})
		}, nil, nil},
		{"a hook on a foreign type", func(d *Declarations) {
			d.Hooks = append(d.Hooks, Hook{Name: "h", Type: HookBeforeUpdate, URI: "x:y/z", Provider: "ktw-a", Filter: ObjectFilter{Types: []string{"note"}}})
		}, nil, []string{"hook/h"}},
		{"a hook on a foreign type, granted", func(d *Declarations) {
			d.Hooks = append(d.Hooks, Hook{Name: "h", Type: HookBeforeUpdate, URI: "x:y/z", Provider: "ktw-a", Filter: ObjectFilter{Types: []string{"note"}}})
		}, Grants{"ktw-a": {Types: []string{"note"}}}, nil},
		{"a hook on every object", func(d *Declarations) {
			d.Hooks = append(d.Hooks, Hook{Name: "h", Type: HookOnTransition, URI: "x:y/z", Provider: "ktw-a"})
		}, nil, []string{"hook/h"}},
		{"an after hook on every object is not scoped", func(d *Declarations) {
			d.Hooks = append(d.Hooks, Hook{Name: "h", Type: HookOnUpdate, URI: "x:y/z", Provider: "ktw-a"})
		}, nil, nil},
		{"a hook on its own capability", func(d *Declarations) {
			d.Hooks = append(d.Hooks, Hook{Name: "h", Type: HookBeforeCreate, URI: "x:y/z", Provider: "ktw-a", Filter: ObjectFilter{Capabilities: []string{"signed"}}})
		}, nil, nil},
		{"a capability attaching to every type with a gate entry", func(d *Declarations) {
			d.Capabilities = append(d.Capabilities, Capability{Name: "watch", Provider: "ktw-a", Hooks: gate, Attach: &ShapeFilter{Types: []string{"*"}}})
		}, nil, []string{"capability/watch"}},
		{"a capability attaching with only after entries", func(d *Declarations) {
			d.Capabilities = append(d.Capabilities, Capability{Name: "watch", Provider: "ktw-a", Hooks: after, Attach: &ShapeFilter{Types: []string{"*"}}})
		}, nil, nil},
		{"a capability attaching to its own type", func(d *Declarations) {
			d.Capabilities = append(d.Capabilities, Capability{Name: "watch", Provider: "ktw-a", Hooks: gate, Attach: &ShapeFilter{Types: []string{"doc"}}})
		}, nil, nil},
		{"a capability carried by choice is not a finding", func(d *Declarations) {
			d.Capabilities = append(d.Capabilities, Capability{Name: "watch", Provider: "ktw-a", Hooks: gate})
		}, nil, nil},
		{"a policy with a gate entry on a foreign type", func(d *Declarations) {
			d.Policies = append(d.Policies, Policy{Name: "p", Provider: "ktw-a", Hooks: gate, Applies: ShapeFilter{Types: []string{"note"}}})
		}, nil, []string{"policy/p"}},
		{"an extension's own gate point is scoped", func(d *Declarations) {
			d.HookTypes = append(d.HookTypes, HookType{Name: "publish_check", Timing: HookTimingBefore, Answer: HookAnswerAccept})
			d.Hooks = append(d.Hooks, Hook{Name: "h", Type: "publish_check", URI: "x:y/z", Provider: "ktw-a", Filter: ObjectFilter{Types: []string{"note"}}})
		}, nil, []string{"hook/h"}},
	} {
		d := scopeDecl()
		row.add(&d)
		var got []string
		for _, f := range ScopeOf(d, row.grants) {
			got = append(got, f.Kind+"/"+f.Name)
			if f.Rule != FindingScope || !strings.Contains(f.Message, "grants: {gate:") {
				t.Errorf("%s: finding %s does not name the grant", row.name, f)
			}
		}
		if strings.Join(got, ",") != strings.Join(row.want, ",") {
			t.Errorf("%s: ScopeOf = %v, want %v", row.name, got, row.want)
		}
	}
}
