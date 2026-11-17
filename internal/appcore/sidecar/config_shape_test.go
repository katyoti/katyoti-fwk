// SPDX-License-Identifier: Apache-2.0

package sidecar

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// config_shape_test.go — a config file becomes schema-addressable.
//
// The point is that a consumer can render a FORM. Before this, the only
// affordance was a health report: it could say a key was missing and could not
// let anyone fix it, because with no schema there are no fields.
//
// The other half is the degradation. A config that is not a typed struct has no
// derivable shape, and a consumer must fall back to the health report rather
// than render an empty or broken form — so the absence is asserted as
// deliberately as the presence.

// WeatherServiceConfig stands in for a sidecar's typed config.
type WeatherServiceConfig struct {
	APIKey  string `json:"apiKey" jsonschema:"API key for the upstream provider"`
	Units   string `json:"units,omitempty" jsonschema:"metric or imperial"`
	Retries int    `json:"retries,omitempty"`
}

// typedPublication is what PublishService produces for a typed config.
func typedPublication(t *testing.T) publish.PublishedService {
	t.Helper()
	schema, err := ktwreflect.JSONSchema[WeatherServiceConfig]()
	if err != nil {
		t.Fatalf("derive schema: %v", err)
	}
	ps := publish.PublishedService{Name: "WeatherService", ConfigFiles: []string{"WeatherService.yaml"}}
	publish.WithConfigShape(schema)(&ps)
	return ps
}

// TestConfigShape_DerivedSchemaMatchesTheGoStruct asserts the published shape
// is a real JSON Schema whose fields are the struct's fields — what a form is
// built from.
func TestConfigShape_DerivedSchemaMatchesTheGoStruct(t *testing.T) {
	ps := typedPublication(t)
	raw, ok := shapeFor([]publish.PublishedService{ps}, "WeatherService.yaml")
	if !ok {
		t.Fatal("no shape recorded for a typed config")
	}
	var schema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("derived shape is not JSON Schema: %v", err)
	}
	if schema.Type != "object" {
		t.Errorf("schema type = %q, want object", schema.Type)
	}
	for _, field := range []string{"apiKey", "units", "retries"} {
		if _, has := schema.Properties[field]; !has {
			t.Errorf("schema has no field %q — a form built from it would be missing it", field)
		}
	}
}

// TestConfigShape_ReferencedByURIFromTheConfigFile asserts the link a consumer
// actually follows: the config-file contribution names its own data-type.
func TestConfigShape_ReferencedByURIFromTheConfigFile(t *testing.T) {
	p := &ConfigResourceProvider{ProviderName: "ktw-weather", ResourceType: mbapi.ResourceTypeConfigFile}
	cs, err := p.enumerateConfigFiles([]publish.PublishedService{typedPublication(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 {
		t.Fatalf("got %d config-file contributions, want 1", len(cs))
	}
	var body mbapi.ConfigFileBody
	if err := json.Unmarshal(cs[0].Data(), &body); err != nil {
		t.Fatal(err)
	}
	const want = "ktw-weather:/data-type/config-WeatherService"
	if body.Schema != want {
		t.Fatalf("config-file schema reference = %q, want %q", body.Schema, want)
	}
	// And the URI it names is the one the shape provider publishes — a
	// reference to a data-type nobody publishes would be worse than none.
	sp := &ConfigShapeProvider{ProviderName: "ktw-weather"}
	shapes, err := sp.enumerateFrom([]publish.PublishedService{typedPublication(t)})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, s := range shapes {
		if s.ResolvedURI() == want {
			found = true
			// The schema IS the contribution's content — its source layer,
			// not a body wrapping it.
			src := string(s.Source())
			if !strings.Contains(src, "apiKey") {
				t.Errorf("published shape does not carry the struct's fields: %s", src)
			}
		}
	}
	if !found {
		t.Errorf("config-file points at %s but no data-type publishes it", want)
	}
}

// TestConfigShape_UntypedConfigDegradesGracefully asserts the fallback: no
// derivable shape means no data-type and no reference, so a consumer renders
// the health report instead of an empty form.
func TestConfigShape_UntypedConfigDegradesGracefully(t *testing.T) {
	// A service published with no typed config declares the file and records
	// no shape — exactly what PublishSimpleService produces.
	ps := publish.PublishedService{Name: "Legacy", ConfigFiles: []string{"Legacy.yaml"}}

	if _, ok := shapeFor([]publish.PublishedService{ps}, "Legacy.yaml"); ok {
		t.Fatal("a shape was recorded for an untyped config")
	}

	cp := &ConfigResourceProvider{ProviderName: "ktw-legacy", ResourceType: mbapi.ResourceTypeConfigFile}
	cs, err := cp.enumerateConfigFiles([]publish.PublishedService{ps})
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 {
		t.Fatalf("got %d config-file contributions, want 1 — the file must still be advertised", len(cs))
	}
	var body mbapi.ConfigFileBody
	if err := json.Unmarshal(cs[0].Data(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Schema != "" {
		t.Errorf("untyped config carries a schema reference %q — a consumer would follow it and find nothing", body.Schema)
	}

	sp := &ConfigShapeProvider{ProviderName: "ktw-legacy"}
	shapes, err := sp.enumerateFrom([]publish.PublishedService{ps})
	if err != nil {
		t.Fatal(err)
	}
	if len(shapes) != 0 {
		t.Errorf("published %d shapes for an untyped config, want 0", len(shapes))
	}
}

// TestConfigShape_IDIsNamespaced asserts the prefix that keeps a config's shape
// out of the way of a domain data-type with the same name: a sidecar with a
// `Weather.yaml` config and a `weather` data-type of its own would otherwise
// publish two different shapes at one URI, and the later would silently win.
//
// It namespaces rather than eliminates — a domain data-type literally named
// `config-Weather` would still collide. That is narrow enough to accept and
// wide enough to say out loud.
func TestConfigShape_IDIsNamespaced(t *testing.T) {
	if got := configShapeID("Weather.yaml"); got == "Weather" || got == "weather" {
		t.Fatalf("configShapeID = %q — collides with a domain data-type of the same name", got)
	}
	if got := configShapeID("Weather.yaml"); !strings.HasPrefix(got, "config-") {
		t.Errorf("configShapeID = %q, want a config- prefix", got)
	}
}
