// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// objectToolsFixture is a real CWM service on a fresh project, reached the way
// an agent reaches it: through the tools, with the service resolved from ctx.
func objectToolsFixture(t *testing.T) (context.Context, string) {
	t.Helper()
	project := t.TempDir()
	root := t.TempDir()
	if err := storage.WriteWorkspaceRaw(root, []byte("version: \"1\"\n")); err != nil {
		t.Fatalf("workspace.yaml: %v", err)
	}
	cfg := &core.CWMServiceConfig{WorkspaceRoot: root, ProjectRoot: project,
		ObjectTypeValidation: api.TypeValidationError}
	cfg.CatalogTypes.Objects = []api.ObjectType{
		{Name: "doc", Capabilities: []string{"container", "content", "memory"}},
	}
	cfg.CatalogTypes.Capabilities = []api.Capability{
		{Name: "container"}, {Name: "content"},
		{Name: "memory", Files: map[string]api.NamedFile{"notes": {Path: "memory/${id}.md"}}},
	}
	svc, err := core.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("core.New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	app := application.New(application.Config())
	application.PutService[api.CWMService](app, "default", svc)
	return application.WithApplication(context.Background(), app), project
}

func objectTool(t *testing.T, name string) toolsapi.Tool {
	t.Helper()
	for _, tool := range CWMTools() {
		if tool.ToolDef().Name == name {
			return tool
		}
	}
	t.Fatalf("tool %s is not registered", name)
	return nil
}

// call runs a tool with a JSON input — exactly what the agent loop sends — and
// decodes its result into out.
func call(t *testing.T, ctx context.Context, name, input string, out any) {
	t.Helper()
	res, err := objectTool(t, name).ToolCall(ctx, json.RawMessage(input))
	if err != nil {
		t.Fatalf("%s(%s): %v", name, input, err)
	}
	b, err := json.Marshal(res.Data)
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatalf("%s: decode %s: %v", name, b, err)
		}
	}
}

// TestObjectToolsRoundTrip: an agent creates an object, finds it by list and
// search, reads it back, updates it, writes and reads a named file as TEXT, and
// deletes it — every step through a tool, against a real service.
func TestObjectToolsRoundTrip(t *testing.T) {
	ctx, project := objectToolsFixture(t)

	call(t, ctx, "cwm-object-create",
		`{"module":"billing","object":{"id":"billing.spec","type":"doc","name":"The spec"}}`, nil)
	if _, err := os.Stat(filepath.Join(project, "modules", "billing")); err != nil {
		t.Fatalf("the object was not written into its module: %v", err)
	}

	var list api.ObjectListOutput
	call(t, ctx, "cwm-object-list", `{"module":"billing","type":"doc"}`, &list)
	if len(list.Objects) != 1 || list.Objects[0].ID != "billing.spec" {
		t.Fatalf("cwm-object-list = %+v, want billing.spec", list.Objects)
	}

	var found api.ObjectSearchOutput
	call(t, ctx, "cwm-object-search", `{"query":"the spec"}`, &found)
	if len(found.Objects) != 1 {
		t.Fatalf("cwm-object-search = %+v, want one hit", found)
	}

	var got api.ObjectGetOutput
	call(t, ctx, "cwm-object-get", `{"module":"billing","id":"billing.spec"}`, &got)
	if got.Object.Type != "doc" {
		t.Fatalf("cwm-object-get type = %q", got.Object.Type)
	}

	// Update wholesale: get, change, send back.
	got.Object.Name = "The revised spec"
	upd, _ := json.Marshal(api.ObjectUpdateInput{Module: "billing", Object: got.Object, Revision: got.Revision})
	call(t, ctx, "cwm-object-update", string(upd), nil)
	call(t, ctx, "cwm-object-get", `{"module":"billing","id":"billing.spec"}`, &got)
	if got.Object.Name != "The revised spec" {
		t.Fatalf("update not stored: %+v", got.Object.Capabilities["content"])
	}

	// Named files travel as text in both directions.
	call(t, ctx, "cwm-object-named-file-write",
		`{"id":"billing.spec","capability":"memory","name":"notes","content":"remember this\n"}`, nil)
	var read namedFileReadToolOutput
	call(t, ctx, "cwm-object-named-file-read", `{"id":"billing.spec","capability":"memory","name":"notes"}`, &read)
	if read.Content != "remember this\n" || !read.File.Exists {
		t.Fatalf("cwm-object-named-file-read = %+v, want the text written", read)
	}

	call(t, ctx, "cwm-object-delete", `{"module":"billing","id":"billing.spec"}`, nil)
	call(t, ctx, "cwm-object-list", `{"module":"billing","type":"doc"}`, &list)
	if len(list.Objects) != 0 {
		t.Fatalf("after cwm-object-delete, list = %+v", list.Objects)
	}
}

// TestObjectToolsRefuseWhatTheServiceRefuses: a tool is a thin forwarder — a
// write the service refuses (an unknown type under strict validation) comes
// back as an error, never as a silent success.
func TestObjectToolsRefuseWhatTheServiceRefuses(t *testing.T) {
	ctx, _ := objectToolsFixture(t)
	_, err := objectTool(t, "cwm-object-create").ToolCall(ctx,
		json.RawMessage(`{"module":"billing","object":{"id":"billing.x","type":"no-such-type","name":"X"}}`))
	if err == nil {
		t.Fatal("creating an object of an undeclared type succeeded through the tool")
	}
}

// TestObjectUpdateToolNamesItsRevision: through the agent tool, an update that
// names no revision is refused, saying what to do; force overwrites; a stale
// revision (the file was edited outside CWM since it was read) is a conflict;
// the fresh revision writes.
func TestObjectUpdateToolNamesItsRevision(t *testing.T) {
	ctx, project := objectToolsFixture(t)
	call(t, ctx, "cwm-object-create",
		`{"module":"billing","object":{"id":"billing.spec","type":"doc","name":"The spec"}}`, nil)
	var got api.ObjectGetOutput
	call(t, ctx, "cwm-object-get", `{"module":"billing","id":"billing.spec"}`, &got)
	update := func(in api.ObjectUpdateInput) error {
		b, _ := json.Marshal(in)
		_, err := objectTool(t, "cwm-object-update").ToolCall(ctx, b)
		return err
	}

	got.Object.Name = "Blind"
	err := update(api.ObjectUpdateInput{Module: "billing", Object: got.Object})
	if err == nil || !strings.Contains(err.Error(), "read the object first and pass its revision (or --force to overwrite)") {
		t.Fatalf("an update without a revision: err = %v, want the refusal naming what to do", err)
	}
	if err := update(api.ObjectUpdateInput{Module: "billing", Object: got.Object, Force: true}); err != nil {
		t.Fatalf("a forced update: %v", err)
	}

	call(t, ctx, "cwm-object-get", `{"module":"billing","id":"billing.spec"}`, &got)
	stale := got.Revision
	// An edit made outside CWM: the object file rewritten by hand.
	var file string
	_ = filepath.WalkDir(project, func(p string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() && strings.Contains(filepath.Base(p), "spec") {
			file = p
		}
		return nil
	})
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("the object file: %v", err)
	}
	if err := os.WriteFile(file, []byte(strings.Replace(string(raw), "Blind", "Edited by hand", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	got.Object.Name = "Over the hand edit"
	if err := update(api.ObjectUpdateInput{Module: "billing", Object: got.Object, Revision: stale}); err == nil || !strings.Contains(err.Error(), api.ErrConflict.Error()) {
		t.Fatalf("an update over an outside edit with the stale revision: err = %v, want a conflict", err)
	}

	call(t, ctx, "cwm-object-get", `{"module":"billing","id":"billing.spec"}`, &got)
	if got.Object.Name != "Edited by hand" {
		t.Fatalf("the hand edit was lost: name = %q", got.Object.Name)
	}
	got.Object.Name = "After reading again"
	if err := update(api.ObjectUpdateInput{Module: "billing", Object: got.Object, Revision: got.Revision}); err != nil {
		t.Fatalf("an update with the fresh revision: %v", err)
	}
}

// TestObjectListToolCapsAndSelects: an agent's list is cut at
// api.ObjectListSurfaceLimit unless it asks for more, says it was cut (total,
// truncated), keeps only the parts named in fields (id and type always) -
// while the service, called by Go code with no limit, still answers all.
func TestObjectListToolCapsAndSelects(t *testing.T) {
	ctx, _ := objectToolsFixture(t)
	svc, err := application.UseService[api.CWMService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	n := api.ObjectListSurfaceLimit + 5
	for i := 0; i < n; i++ {
		o := api.Object{ID: fmt.Sprintf("billing.d%03d", i), Type: "doc", Name: fmt.Sprintf("Doc %d", i),
			Description: "long text an agent did not ask for", Body: "a body",
			Capabilities: map[string]map[string]any{"memory": {"note": "kept", "other": "dropped"}}}
		if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}

	var list api.ObjectListOutput
	call(t, ctx, "cwm-object-list", `{"module":"billing"}`, &list)
	if len(list.Objects) != api.ObjectListSurfaceLimit || list.Total != n || !list.Truncated {
		t.Fatalf("default tool list = %d objects, total %d, truncated %v; want %d of %d, truncated",
			len(list.Objects), list.Total, list.Truncated, api.ObjectListSurfaceLimit, n)
	}
	call(t, ctx, "cwm-object-list", `{"module":"billing","limit":3}`, &list)
	if len(list.Objects) != 3 || list.Objects[0].ID != "billing.d000" || !list.Truncated {
		t.Fatalf("limit 3 = %+v, want the first 3 by id, truncated", list.Objects)
	}
	list = api.ObjectListOutput{}
	call(t, ctx, "cwm-object-list", `{"module":"billing","limit":-1}`, &list)
	if len(list.Objects) != n || list.Truncated {
		t.Fatalf("limit -1 = %d objects, truncated %v; want all %d", len(list.Objects), list.Truncated, n)
	}

	list = api.ObjectListOutput{}
	call(t, ctx, "cwm-object-list", `{"module":"billing","limit":1,"fields":["name","capabilities.memory.note"]}`, &list)
	got := list.Objects[0]
	if got.ID != "billing.d000" || got.Type != "doc" || got.Name != "Doc 0" {
		t.Fatalf("selected object = %+v, want id, type and name kept", got)
	}
	if got.Description != "" || got.Body != "" {
		t.Fatalf("selected object kept parts not asked for: %+v", got)
	}
	if mem := got.Capabilities["memory"]; mem["note"] != "kept" || mem["other"] != nil {
		t.Fatalf("selected capabilities = %v, want memory.note only", got.Capabilities)
	}
	// The selection never writes the index's own maps.
	full, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "billing", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if full.Objects[0].Capabilities["memory"]["other"] != "dropped" {
		t.Fatalf("the index lost what a selection dropped: %+v", full.Objects[0])
	}

	// A Go caller with no limit still reads the whole list.
	all, err := svc.ObjectList(ctx, api.ObjectListInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Objects) != n || all.Truncated {
		t.Fatalf("service ObjectList with no limit = %d objects, truncated %v; want all %d", len(all.Objects), all.Truncated, n)
	}
}
