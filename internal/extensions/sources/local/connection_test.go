// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// : a scope, a hit and a source location carry the connection
// they are reached through - empty, the universe's one configuration of the
// connector (the seam IN-083 fills). The location keeps it under the reserved
// key read only through ConnectionOf, and a location form saving its own
// fields never drops it nor the carrier's added scopes.
func TestAScopeAndALocationCarryTheirConnection(t *testing.T) {
	for _, c := range []struct {
		name string
		v    any
		into any
		want string
	}{
		{"scope", api.Scope{Key: "AUP", Connection: "conf-2"}, &api.Scope{}, `"connection":"conf-2"`},
		{"hit", api.PickItem{Key: "p1", Title: "P", Connection: "conf-2"}, &api.PickItem{}, `"connection":"conf-2"`},
	} {
		raw, err := json.Marshal(c.v)
		if err != nil || !strings.Contains(string(raw), c.want) {
			t.Fatalf("a %s carries its connection: %s %v", c.name, raw, err)
		}
		if err := json.Unmarshal(raw, c.into); err != nil || !reflect.DeepEqual(reflect.ValueOf(c.into).Elem().Interface(), c.v) {
			t.Errorf("a %s round-trips its connection: %+v %v", c.name, c.into, err)
		}
	}
	for _, v := range []any{api.Scope{Key: "AUP"}, api.PickItem{Key: "p1", Title: "P"}} {
		if raw, _ := json.Marshal(v); strings.Contains(string(raw), "connection") {
			t.Errorf("the universe's configuration (no connection) writes none: %s", raw)
		}
	}

	for _, c := range []struct {
		location map[string]any
		want     string
	}{
		{nil, ""},
		{map[string]any{"region": "Nord"}, ""},
		{map[string]any{api.LocationConnection: 7}, ""},
		{map[string]any{api.LocationConnection: " conf-2 "}, "conf-2"},
	} {
		if got := api.ConnectionOf(c.location); got != c.want {
			t.Errorf("ConnectionOf(%v) = %q, want %q", c.location, got, c.want)
		}
	}

	ctx, cwm, svc, _ := pickWorldWith(t, map[string]string{"places.yaml": places})
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "places"})
	if err != nil {
		t.Fatal(err)
	}
	stored := func() map[string]any {
		t.Helper()
		module, err := cwmapi.ModuleOfObjectID(used.Source)
		if err != nil {
			t.Fatal(err)
		}
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: used.Source})
		if err != nil {
			t.Fatal(err)
		}
		var f api.InformationSourceFields
		if err := json.Unmarshal(got.Object.FieldsJSON(), &f); err != nil {
			t.Fatal(err)
		}
		return f.Location
	}
	if got := api.ConnectionOf(stored()); got != "" {
		t.Fatalf("a source used with no connection reads the configuration: %q", got)
	}
	save := func(location map[string]any) {
		t.Helper()
		if out, err := svc.LocationSave(ctx, api.LocationSaveInput{ID: used.Source, Location: location}); err != nil || !out.OK {
			t.Fatalf("save %v: %+v %v", location, out, err)
		}
	}
	save(map[string]any{"region": "Nord", api.LocationConnection: "conf-2", api.LocationScopes: []any{"ARC"}})
	save(map[string]any{"region": "Sud"})
	loc := stored()
	if got := api.ConnectionOf(loc); got != "conf-2" {
		t.Errorf("a form saving its own fields keeps the stored connection: %+v", loc)
	}
	if !reflect.DeepEqual(loc[api.LocationScopes], []any{"ARC"}) || loc["region"] != "Sud" {
		t.Errorf("the form's fields are saved and the added scopes kept: %+v", loc)
	}
	save(map[string]any{"region": "Sud", api.LocationConnection: ""})
	if got := api.ConnectionOf(stored()); got != "" {
		t.Errorf("a form naming the connection sets it - empty, back to the configuration: %q", got)
	}
}
