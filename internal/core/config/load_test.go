// SPDX-License-Identifier: Apache-2.0

package config_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

type TestPluginsConfig struct {
	Plugins []string `yaml:"plugins"`
}

type TestSettings struct {
	Port int    `yaml:"port" default:"8080"`
	Host string `yaml:"host" default:"localhost"`
}

func TestDecode_YAML(t *testing.T) {
	data := []byte("plugins:\n  - a\n  - b\n")
	cfg, err := config.Decode[TestPluginsConfig](data, marshal.YAML, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Plugins) != 2 || cfg.Plugins[0] != "a" {
		t.Errorf("got %v", cfg.Plugins)
	}
}

func TestDecode_JSON(t *testing.T) {
	data := []byte(`{"plugins":["x","y"]}`)
	cfg, err := config.Decode[TestPluginsConfig](data, marshal.JSON, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Plugins) != 2 || cfg.Plugins[0] != "x" {
		t.Errorf("got %v", cfg.Plugins)
	}
}

func TestDecodeBytes_ProcessConfig(t *testing.T) {
	// Empty YAML — defaults should be applied by ProcessConfig.
	data := []byte("{}")
	var cfg TestSettings
	if err := config.DecodeBytes(data, marshal.YAML, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host = %q, want localhost", cfg.Host)
	}
}

func TestDecodeBytes_FromLoader(t *testing.T) {
	ml := loader.NewMapLoader(map[string]string{
		"settings.yaml": "port: 9090\nhost: example.com\n",
	})
	data, err := ml.ReadFile("settings.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg TestSettings
	if err := config.DecodeBytes(data, marshal.YAML, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.Host != "example.com" {
		t.Errorf("Host = %q, want example.com", cfg.Host)
	}
}

func TestDecodeBytes_MissingFile(t *testing.T) {
	ml := loader.NewMapLoader(map[string]string{})
	data, _ := ml.ReadFile("missing.yaml")
	if data != nil {
		t.Fatal("expected nil data for missing file")
	}
}

func TestLoadMap_YAML(t *testing.T) {
	data := []byte("key: value\nnested:\n  a: 1\n")
	m, err := config.LoadMap(data, marshal.YAML)
	if err != nil {
		t.Fatal(err)
	}
	if m["key"] != "value" {
		t.Errorf("key = %v", m["key"])
	}
}

func TestLoadMap_JSON(t *testing.T) {
	data := []byte(`{"key":"value"}`)
	m, err := config.LoadMap(data, marshal.JSON)
	if err != nil {
		t.Fatal(err)
	}
	if m["key"] != "value" {
		t.Errorf("key = %v", m["key"])
	}
}

func TestDecode_WithDefaults(t *testing.T) {
	// Partial YAML — defaults should be applied to missing fields.
	data := []byte("port: 3000\n")
	cfg, err := config.Decode[TestSettings](data, marshal.YAML, "test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host = %q, want localhost", cfg.Host)
	}
}
