// SPDX-License-Identifier: Apache-2.0

package sidecar

import (
	"context"
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

func samplePubs() []publish.PublishedService {
	return []publish.PublishedService{
		{
			Name:        "DocService",
			ConfigFiles: []string{"DocService.yaml", "Settings.yaml"},
			Tools: []publish.ToolDef{
				{Name: "pandoc", Commands: []string{"pandoc"}, ConfigKey: "pandoc_path", InstallSkill: "ktw-docs:/skill/install-pandoc"},
				{Name: "typst", Commands: []string{"typst"}, VersionFlag: "--version"},
			},
			APIKeys: []publish.APIKeyDef{
				{EnvName: "ANTHROPIC_API_KEY", Label: "Anthropic API key", Required: true, Description: "Used for Claude calls"},
			},
		},
		{
			Name:        "OtherService",
			ConfigFiles: []string{"Settings.yaml"}, // duplicate — should be deduped
			Tools: []publish.ToolDef{
				{Name: "pandoc", Commands: []string{"pandoc"}}, // duplicate
				{Name: "graphviz", Commands: []string{"dot"}},
			},
			APIKeys: []publish.APIKeyDef{
				{EnvName: "ANTHROPIC_API_KEY"}, // duplicate
				{EnvName: "OPENAI_API_KEY", Label: "OpenAI API key", Required: false},
			},
		},
	}
}

func TestConfigResourceProvider_Type_Provider(t *testing.T) {
	p := &ConfigResourceProvider{ProviderName: "ktw-docs", ResourceType: mbapi.ResourceTypeConfigFile}
	if p.Type() != "config-file" {
		t.Errorf("Type() = %q, want config-file", p.Type())
	}
	if p.Provider() != "ktw-docs" {
		t.Errorf("Provider() = %q, want ktw-docs", p.Provider())
	}
}

func TestConfigResourceProvider_EnumerateConfigFiles(t *testing.T) {
	efs := fstest.MapFS{
		"embed/config/DocService.yaml": &fstest.MapFile{Data: []byte("port: 8890\n")},
		// Settings.yaml has no embedded default
	}
	p := &ConfigResourceProvider{
		ProviderName: "ktw-docs",
		ResourceType: mbapi.ResourceTypeConfigFile,
		EmbeddedFS:   efs,
	}
	got, err := p.enumerateConfigFiles(samplePubs())
	if err != nil {
		t.Fatalf("enumerateConfigFiles: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (deduped); entries: %+v", len(got), got)
	}

	want := map[string]bool{"DocService.yaml": true, "Settings.yaml": false}
	for _, e := range got {
		if e.Provider != "ktw-docs" || e.Type != "config-file" {
			t.Errorf("entry %s: provider/type wrong: %+v", e.ID, e)
		}
		var body mbapi.ConfigFileBody
		if err := json.Unmarshal(e.Data(), &body); err != nil {
			t.Fatalf("unmarshal body for %s: %v", e.ID, err)
		}
		expected, ok := want[e.ID]
		if !ok {
			t.Errorf("unexpected ID %q", e.ID)
			continue
		}
		if body.HasDefault != expected {
			t.Errorf("%s: HasDefault = %v, want %v", e.ID, body.HasDefault, expected)
		}
		delete(want, e.ID)
	}
	if len(want) != 0 {
		t.Errorf("missing entries: %v", want)
	}
}

func TestConfigResourceProvider_EnumerateConfigPrograms(t *testing.T) {
	p := &ConfigResourceProvider{ProviderName: "ktw-docs", ResourceType: mbapi.ResourceTypeConfigProgram}
	got, err := p.enumerateConfigPrograms(samplePubs())
	if err != nil {
		t.Fatalf("enumerateConfigPrograms: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (pandoc/typst/graphviz, deduped)", len(got))
	}

	byName := map[string]mbapi.ConfigProgramBody{}
	for _, e := range got {
		var body mbapi.ConfigProgramBody
		if err := json.Unmarshal(e.Data(), &body); err != nil {
			t.Fatalf("unmarshal %s: %v", e.ID, err)
		}
		byName[e.ID] = body
	}
	pandoc := byName["pandoc"]
	if pandoc.ConfigKey != "pandoc_path" {
		t.Errorf("pandoc.ConfigKey = %q, want pandoc_path", pandoc.ConfigKey)
	}
	if pandoc.InstallSkill != "ktw-docs:/skill/install-pandoc" {
		t.Errorf("pandoc.InstallSkill = %q", pandoc.InstallSkill)
	}
	if byName["typst"].VersionFlag != "--version" {
		t.Errorf("typst.VersionFlag = %q", byName["typst"].VersionFlag)
	}
	if got, want := byName["graphviz"].Commands, []string{"dot"}; len(got) != 1 || got[0] != want[0] {
		t.Errorf("graphviz.Commands = %v, want %v", got, want)
	}
}

func TestConfigResourceProvider_EnumerateEnv(t *testing.T) {
	p := &ConfigResourceProvider{ProviderName: "ktw-docs", ResourceType: mbapi.ResourceTypeEnv}
	got, err := p.enumerateEnv(samplePubs())
	if err != nil {
		t.Fatalf("enumerateEnv: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (ANTHROPIC + OPENAI, deduped)", len(got))
	}

	byName := map[string]mbapi.EnvBody{}
	for _, e := range got {
		var body mbapi.EnvBody
		if err := json.Unmarshal(e.Data(), &body); err != nil {
			t.Fatalf("unmarshal %s: %v", e.ID, err)
		}
		byName[e.ID] = body
	}
	anth := byName["ANTHROPIC_API_KEY"]
	if !anth.Required || anth.Label != "Anthropic API key" || anth.Description != "Used for Claude calls" {
		t.Errorf("ANTHROPIC_API_KEY body wrong: %+v", anth)
	}
	openai := byName["OPENAI_API_KEY"]
	if openai.Required {
		t.Errorf("OPENAI_API_KEY: Required = true, want false")
	}
}

func TestConfigResourceProvider_Enumerate_UnknownType(t *testing.T) {
	p := &ConfigResourceProvider{ProviderName: "ktw-docs", ResourceType: "bogus-type"}
	// Bypass the application.UseExtensions path by hitting the switch directly.
	_, err := p.Enumerate(context.Background())
	if err == nil {
		t.Fatal("expected error for unknown ResourceType, got nil")
	}
}

func TestConfigResourceProvider_URIs(t *testing.T) {
	// Verify the canonical URI shape per Contribution.URI().
	p := &ConfigResourceProvider{ProviderName: "ktw-docs", ResourceType: mbapi.ResourceTypeConfigFile}
	got, err := p.enumerateConfigFiles(samplePubs())
	if err != nil {
		t.Fatalf("enumerateConfigFiles: %v", err)
	}
	for _, e := range got {
		want := "ktw-docs:/config-file/" + e.ID
		if e.ResolvedURI() != want {
			t.Errorf("URI = %q, want %q", e.ResolvedURI(), want)
		}
	}
}

func TestConfigResourceProviders_GroupHelper(t *testing.T) {
	got := ConfigResourceProviders("ktw-docs", nil)
	// Four kinds: what the binary NEEDS (config-file, config-program, env)
	// plus the SHAPE of each typed config file, published as an ordinary
	// data-type so a consumer can render a form rather than only a report.
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	wantTypes := []string{"config-file", "config-program", "env", "data-type"}
	for i, p := range got {
		if p.Provider() != "ktw-docs" {
			t.Errorf("[%d].Provider() = %q", i, p.Provider())
		}
		if p.Type() != wantTypes[i] {
			t.Errorf("[%d].Type() = %q, want %q", i, p.Type(), wantTypes[i])
		}
	}
}
