// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"testing"
	"time"
)

// TestTheFieldsRoundTripAsJSON: the type's fields set from JSON read back as
// the same JSON; anything but an object, or empty, clears them.
func TestTheFieldsRoundTripAsJSON(t *testing.T) {
	var o Object
	o.SetFieldsJSON(json.RawMessage(`{"claim":"it holds","level":2}`))
	var back map[string]any
	if err := json.Unmarshal(o.FieldsJSON(), &back); err != nil || back["claim"] != "it holds" {
		t.Fatalf("fields: %v %s", err, o.FieldsJSON())
	}
	o.SetFieldsJSON(json.RawMessage(`"## Why"`))
	if o.Fields != nil || o.FieldsJSON() != nil {
		t.Errorf("a string is prose, not fields: %v", o.Fields)
	}
}

// TestARecordTakesProseOrFields: NewRecordObject's content is the body when it
// is a string, the type's fields when it is an object.
func TestARecordTakesProseOrFields(t *testing.T) {
	p := NewRecordObject("m", "r1", "note", "m", "R", json.RawMessage(`"## Why\n\nbecause"`))
	if p.Body != "## Why\n\nbecause" || p.Fields != nil {
		t.Errorf("prose: body %q fields %v", p.Body, p.Fields)
	}
	f := NewRecordObject("m", "r2", "note", "m", "R", json.RawMessage(`{"k":"v"}`))
	if f.Body != "" || f.Fields["k"] != "v" {
		t.Errorf("fields: body %q fields %v", f.Body, f.Fields)
	}
}

// TestIdentityRoundTrip: the editable core the Identity tab shows and saves.
func TestIdentityRoundTrip(t *testing.T) {
	o := Object{ID: "m.x", Parent: "m", Name: "X", Description: "about", Tags: []string{"a"}, Body: "says", Fields: map[string]any{"k": "v"}}
	id := o.Identity()
	id.Name, id.Icon = "Renamed", "star"
	o.SetIdentity(id)
	if o.Name != "Renamed" || o.Icon != "star" || o.Description != "about" || o.Parent != "m" || o.Body != "says" || o.Fields["k"] != "v" {
		t.Errorf("identity: %+v", o)
	}
}

// TestACapabilityRoundTrips pins that a typed capability value survives being
// stored as a field-group and read back.
func TestACapabilityRoundTrips(t *testing.T) {
	type sizing struct {
		Unit string    `json:"unit"`
		At   time.Time `json:"at"`
	}
	var o Object
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	if err := o.SetCapability("sizing", sizing{Unit: "days", At: now}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := DecodeCapability[sizing](o, "sizing")
	if !ok || err != nil || got.Unit != "days" || !got.At.Equal(now) {
		t.Errorf("round trip: %+v ok=%v err=%v", got, ok, err)
	}
}

// TestCapabilityAbsentIsNotAnError pins that a missing capability reads as
// absent — not as a zero value that looks present — and a malformed one says
// so.
func TestCapabilityAbsentIsNotAnError(t *testing.T) {
	o := Object{ID: "cwm.bare"}
	if _, ok, err := DecodeCapability[map[string]any](o, CapWant); ok || err != nil {
		t.Errorf("want should be absent on a bare object: ok=%v err=%v", ok, err)
	}
	if got := o.ParentID(); got != "" {
		t.Errorf("ParentID on a bare object = %q, want empty", got)
	}
	o.Capabilities = map[string]map[string]any{"sizing": {"unit": 3}}
	if _, ok, err := DecodeCapability[struct {
		Unit string `json:"unit"`
	}](o, "sizing"); !ok || err == nil {
		t.Errorf("a malformed capability must say so: ok=%v err=%v", ok, err)
	}
}
