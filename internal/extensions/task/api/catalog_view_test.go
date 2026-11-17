// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

func TestTaskSummaryFromCatalog(t *testing.T) {
	body := mbapi.TaskBody{
		Name:        "doc-create",
		Type:        "agent",
		Description: "Create documents",
		Capability:  "smart-best",
		Icon:        "ðŸ“„",
		IconName:    "file",
		Hashtags:    []string{"writing"},
		Category:    "docs",
		Tags:        []string{"quick-action"},
		Kind:        "object",
		Types:       []string{"document"},
		Filter:      `node.state == "active"`,
	}
	s := TaskSummaryFromCatalog("ktw-docs", body)
	if s.Name != "doc-create" || s.Provider != "ktw-docs" {
		t.Errorf("Name/Provider mismatch: %+v", s)
	}
	if s.URI != "ktw-docs:doc-create" {
		t.Errorf("URI = %q, want ktw-docs:doc-create", s.URI)
	}
	if s.Type != "agent" {
		t.Errorf("Type = %q", s.Type)
	}
	if s.Capability != "smart-best" {
		t.Errorf("Capability = %q", s.Capability)
	}
	if s.Kind != "object" {
		t.Errorf("Kind = %q", s.Kind)
	}
	if len(s.Types) != 1 || s.Types[0] != "document" {
		t.Errorf("Types = %v", s.Types)
	}
	if s.Filter != `node.state == "active"` {
		t.Errorf("Filter mismatch: %q", s.Filter)
	}
}

type fakeTaskCatalog struct {
	manifests map[string]*mbapi.Manifest
}

func (c *fakeTaskCatalog) Get(_ context.Context, name string) (*mbapi.Manifest, error) {
	return c.manifests[name], nil
}

func (c *fakeTaskCatalog) List(_ context.Context) ([]*mbapi.Manifest, error) {
	out := make([]*mbapi.Manifest, 0, len(c.manifests))
	for _, m := range c.manifests {
		out = append(out, m)
	}
	return out, nil
}

func (c *fakeTaskCatalog) Names(_ context.Context) ([]string, error) {
	out := make([]string, 0, len(c.manifests))
	for n := range c.manifests {
		out = append(out, n)
	}
	return out, nil
}

func (c *fakeTaskCatalog) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, m := range c.manifests {
		for _, e := range m.Contributions {
			if e.Type == t {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

func (c *fakeTaskCatalog) GetBody(_ context.Context, uri string) ([]byte, error) {
	pu, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return nil, err
	}
	for _, m := range c.manifests {
		for _, e := range m.Contributions {
			prov := e.Provider
			if prov == "" {
				prov = m.Provider
			}
			if e.Type == pu.Type && e.ID == pu.ID && prov == pu.Provider {
				return e.Data(), nil
			}
		}
	}
	return nil, mbapi.ErrSidecarNotInCatalog
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestTaskSummariesFromCatalog(t *testing.T) {
	a := mbapi.TaskBody{Name: "task-a", Type: "agent"}
	b := mbapi.TaskBody{Name: "task-b", Type: "workflow"}
	cat := &fakeTaskCatalog{
		manifests: map[string]*mbapi.Manifest{
			"ktw-x": {
				Provider: "ktw-x",
				Contributions: []contribapi.Contribution{
					{Provider: "ktw-x", Type: mbapi.ResourceTypeTask, ID: "task-a", Layers: contribapi.InlineData(mustJSON(t, a))},
					{Provider: "ktw-x", Type: "skill", ID: "skill-z"}, // mixed: skill should be excluded
				},
			},
			"ktw-y": {
				Provider: "ktw-y",
				Contributions: []contribapi.Contribution{
					{Provider: "ktw-y", Type: mbapi.ResourceTypeTask, ID: "task-b", Layers: contribapi.InlineData(mustJSON(t, b))},
				},
			},
		},
	}

	out, err := TaskSummariesFromCatalog(context.Background(), cat)
	if err != nil {
		t.Fatalf("TaskSummariesFromCatalog: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d tasks, want 2 (skill-z excluded by type filter); %+v", len(out), out)
	}
	var sawA, sawB bool
	for _, s := range out {
		if s.URI == "ktw-x:task-a" {
			sawA = true
		}
		if s.URI == "ktw-y:task-b" {
			sawB = true
		}
	}
	if !sawA {
		t.Error("missing ktw-x:task-a")
	}
	if !sawB {
		t.Error("missing ktw-y:task-b")
	}
}

func TestTaskSummariesFromCatalog_NilCatalog(t *testing.T) {
	out, err := TaskSummariesFromCatalog(context.Background(), nil)
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if out != nil {
		t.Errorf("out = %v, want nil for nil catalog", out)
	}
}

// Two-phase bootstrap protocol stubs (hub-boot-order ADR).
func (*fakeTaskCatalog) SetSchema(_ mbapi.CatalogSchemaService) error { return nil }
func (*fakeTaskCatalog) Reindex(_ context.Context, _ []string) error  { return nil }
func (*fakeTaskCatalog) MarkReady(_ context.Context) error            { return nil }
func (*fakeTaskCatalog) Ready() bool                                  { return true }
