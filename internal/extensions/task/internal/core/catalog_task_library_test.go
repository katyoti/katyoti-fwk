// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

type fakeCatalog struct {
	manifests map[string]*mbapi.Manifest
}

func (c *fakeCatalog) Get(_ context.Context, name string) (*mbapi.Manifest, error) {
	m, ok := c.manifests[name]
	if !ok {
		return nil, mbapi.ErrSidecarNotInCatalog
	}
	return m, nil
}

func (c *fakeCatalog) List(_ context.Context) ([]*mbapi.Manifest, error) {
	out := make([]*mbapi.Manifest, 0, len(c.manifests))
	for _, m := range c.manifests {
		out = append(out, m)
	}
	return out, nil
}

func (c *fakeCatalog) Names(_ context.Context) ([]string, error) {
	out := make([]string, 0, len(c.manifests))
	for n := range c.manifests {
		out = append(out, n)
	}
	return out, nil
}

func (c *fakeCatalog) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
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

// GetBody resolves a body from the inline manifests (full-body stub).
func (c *fakeCatalog) GetBody(_ context.Context, uri string) ([]byte, error) {
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
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func buildCatalog(t *testing.T, entries map[string][]mbapi.TaskBody) *fakeCatalog {
	t.Helper()
	manifests := map[string]*mbapi.Manifest{}
	for provider, bodies := range entries {
		m := &mbapi.Manifest{Provider: provider}
		for _, b := range bodies {
			m.Contributions = append(m.Contributions, contribapi.Contribution{
				Provider: provider,
				Type:     mbapi.ResourceTypeTask,
				ID:       b.Name,
				Layers:   contribapi.InlineData(mustJSON(t, b)),
			})
		}
		manifests[provider] = m
	}
	return &fakeCatalog{manifests: manifests}
}

func TestCatalogTaskLibrary_ListTasks(t *testing.T) {
	cat := buildCatalog(t, map[string][]mbapi.TaskBody{
		"ktw-x": {
			{Name: "task-a", Type: "agent", Description: "A"},
			{Name: "task-b", Type: "workflow", Description: "B"},
		},
		"ktw-y": {
			{Name: "task-c", Type: "agent", Description: "C"},
		},
	})
	lib := NewCatalogTaskLibrary(cat)

	resp, err := lib.ListTasks(context.Background(), taskapi.ListTasksRequest{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(resp.Tasks) != 3 {
		t.Errorf("got %d tasks, want 3 (a + b + c); %+v", len(resp.Tasks), resp.Tasks)
	}
	// Each task should have URI = provider:name
	urisSeen := map[string]bool{}
	for _, ts := range resp.Tasks {
		urisSeen[ts.URI] = true
	}
	for _, want := range []string{"ktw-x:task-a", "ktw-x:task-b", "ktw-y:task-c"} {
		if !urisSeen[want] {
			t.Errorf("missing task %q in output (have %v)", want, urisSeen)
		}
	}
}

func TestCatalogTaskLibrary_GetTask_URI(t *testing.T) {
	cat := buildCatalog(t, map[string][]mbapi.TaskBody{
		"ktw-docs": {
			{Name: "doc-create", Type: "agent", Description: "Create docs", Body: "# Prompt"},
		},
	})
	lib := NewCatalogTaskLibrary(cat)

	info, err := lib.GetTask(context.Background(), taskapi.GetTaskRequest{Name: "ktw-docs:doc-create"})
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if info.Name != "doc-create" || info.Provider != "ktw-docs" || info.URI != "ktw-docs:doc-create" {
		t.Errorf("identity mismatch: %+v", info)
	}
	if info.Type != "agent" {
		t.Errorf("Type = %q", info.Type)
	}
	if info.Body != "# Prompt" {
		t.Errorf("Body = %q", info.Body)
	}
}

func TestCatalogTaskLibrary_GetTask_ConfigDecoded(t *testing.T) {
	cfg, _ := json.Marshal(map[string]any{"capability": "smart-best", "max_turns": 5})
	cat := buildCatalog(t, map[string][]mbapi.TaskBody{
		"ktw-x": {
			{Name: "t1", Type: "agent", Config: cfg},
		},
	})
	lib := NewCatalogTaskLibrary(cat)

	info, err := lib.GetTask(context.Background(), taskapi.GetTaskRequest{Name: "ktw-x:t1"})
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if info.Config == nil {
		t.Fatal("Config nil after decode")
	}
	if info.Config["capability"] != "smart-best" {
		t.Errorf("Config[capability] = %v, want smart-best", info.Config["capability"])
	}
}

func TestCatalogTaskLibrary_GetTask_BareNameUnique(t *testing.T) {
	cat := buildCatalog(t, map[string][]mbapi.TaskBody{
		"ktw-x": {{Name: "alpha", Type: "agent"}},
		"ktw-y": {{Name: "beta", Type: "agent"}},
	})
	lib := NewCatalogTaskLibrary(cat)

	info, err := lib.GetTask(context.Background(), taskapi.GetTaskRequest{Name: "alpha"})
	if err != nil {
		t.Fatalf("GetTask(alpha): %v", err)
	}
	if info.URI != "ktw-x:alpha" {
		t.Errorf("URI = %q, want ktw-x:alpha", info.URI)
	}
}

func TestCatalogTaskLibrary_GetTask_BareNameAmbiguous(t *testing.T) {
	cat := buildCatalog(t, map[string][]mbapi.TaskBody{
		"ktw-x": {{Name: "shared", Type: "agent"}},
		"ktw-y": {{Name: "shared", Type: "workflow"}},
	})
	lib := NewCatalogTaskLibrary(cat)

	_, err := lib.GetTask(context.Background(), taskapi.GetTaskRequest{Name: "shared"})
	if err == nil {
		t.Fatal("ambiguous bare name should error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("err = %v, want one mentioning 'ambiguous'", err)
	}
}

func TestCatalogTaskLibrary_GetTask_URIUnknownProvider(t *testing.T) {
	cat := buildCatalog(t, map[string][]mbapi.TaskBody{
		"ktw-x": {{Name: "alpha", Type: "agent"}},
	})
	lib := NewCatalogTaskLibrary(cat)

	_, err := lib.GetTask(context.Background(), taskapi.GetTaskRequest{Name: "ktw-missing:foo"})
	if err == nil {
		t.Fatal("unknown provider should error")
	}
}

func TestCatalogTaskLibrary_GetTask_NotFound(t *testing.T) {
	cat := buildCatalog(t, map[string][]mbapi.TaskBody{
		"ktw-x": {{Name: "alpha", Type: "agent"}},
	})
	lib := NewCatalogTaskLibrary(cat)

	_, err := lib.GetTask(context.Background(), taskapi.GetTaskRequest{Name: "nonexistent"})
	if err == nil {
		t.Fatal("unknown task should error")
	}
}

func TestCatalogTaskLibrary_NilCatalog(t *testing.T) {
	lib := NewCatalogTaskLibrary(nil)

	// ListTasks with nil cat â†’ empty output, no error
	resp, err := lib.ListTasks(context.Background(), taskapi.ListTasksRequest{})
	if err != nil {
		t.Errorf("ListTasks(nil cat) err = %v", err)
	}
	if len(resp.Tasks) != 0 {
		t.Errorf("ListTasks(nil cat) tasks = %v", resp.Tasks)
	}

	// GetTask with nil cat â†’ clear error
	_, err = lib.GetTask(context.Background(), taskapi.GetTaskRequest{Name: "ktw-x:y"})
	if err == nil {
		t.Error("GetTask(nil cat) should error")
	}
}

// Two-phase bootstrap protocol stubs (hub-boot-order ADR).
func (*fakeCatalog) SetSchema(_ mbapi.CatalogSchemaService) error { return nil }
func (*fakeCatalog) Reindex(_ context.Context, _ []string) error  { return nil }
func (*fakeCatalog) MarkReady(_ context.Context) error            { return nil }
func (*fakeCatalog) Ready() bool                                  { return true }
