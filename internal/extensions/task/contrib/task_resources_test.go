// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

const validTaskMD = `---
name: doc-create
type: agent
description: Create documents with an LLM
hashtags:
  - documents
category: writing
tags:
  - quick-action
config:
  capability: smart-best
  plugins: [search]
---
# Doc Create Prompt

You are a document creation assistant.
`

const malformedTaskMD = `---
name: malformed
# missing required 'type' field
---
body
`

func TestTaskProviders_Enumerate(t *testing.T) {
	fs := fstest.MapFS{
		"embed/tasks/doc-create/TASK.md": &fstest.MapFile{Data: []byte(validTaskMD)},
		"embed/tasks/broken/TASK.md":     &fstest.MapFile{Data: []byte(malformedTaskMD)},
		"embed/tasks/README.txt":         &fstest.MapFile{Data: []byte("not a task")},
	}
	p := TaskProviders("ktw-test", loader.NewEmbedFSLoader(fs, ""), "embed/tasks")[0]

	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate: %v", err)
	}
	// A file that reads as a task is published; what it lacks (`type`) is the
	// task lint's finding at authoring (the census), not a silent skip. The
	// readme is no task (match *.md).
	byID := map[string]contribapi.Contribution{}
	for _, c := range entries {
		byID[c.ID] = c
	}
	if len(entries) != 2 || byID["malformed"].ID == "" {
		t.Fatalf("got %d entries, want doc-create and malformed (readme skipped); entries: %+v", len(entries), entries)
	}
	e := byID["doc-create"]
	if e.Provider != "ktw-test" {
		t.Errorf("Provider = %q, want ktw-test", e.Provider)
	}
	if e.Type != mbapi.ResourceTypeTask {
		t.Errorf("Type = %q, want %q", e.Type, mbapi.ResourceTypeTask)
	}
	if e.ID != "doc-create" {
		t.Errorf("ID = %q, want doc-create", e.ID)
	}
	var body mbapi.TaskBody
	if err := json.Unmarshal(e.Data(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Name != "doc-create" {
		t.Errorf("Body.Name = %q", body.Name)
	}
	if body.Type != "agent" {
		t.Errorf("Body.Type = %q, want agent", body.Type)
	}
	if body.Description != "Create documents with an LLM" {
		t.Errorf("Body.Description mismatch: %q", body.Description)
	}
	if !strings.Contains(string(body.Config), `"capability":"smart-best"`) {
		t.Errorf("Body.Config = %s, want the capability in config (the readers derive TaskSummary.Capability from it)", body.Config)
	}
	if body.Category != "writing" {
		t.Errorf("Body.Category = %q", body.Category)
	}
	if len(body.Hashtags) != 1 || body.Hashtags[0] != "documents" {
		t.Errorf("Body.Hashtags = %v", body.Hashtags)
	}
	if len(body.Tags) != 1 || body.Tags[0] != "quick-action" {
		t.Errorf("Body.Tags = %v", body.Tags)
	}
	if body.Body == "" {
		t.Error("Body.Body empty — should carry the markdown body text")
	}
	if len(body.Config) == 0 {
		t.Error("Body.Config empty — should carry the config block JSON")
	}
}

func TestTaskProviders_Open(t *testing.T) {
	fs := fstest.MapFS{
		"embed/tasks/doc-create/TASK.md": &fstest.MapFile{Data: []byte(validTaskMD)},
	}
	p := TaskProviders("ktw-test", loader.NewEmbedFSLoader(fs, ""), "embed/tasks")[0]

	rc, ref, err := p.Open(context.Background(), "doc-create", contribapi.LayerSource)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != validTaskMD || ref.MediaType != "text/markdown" {
		t.Errorf("Open(source) = %q (%d bytes), want the raw TASK.md as text/markdown", ref.MediaType, len(b))
	}
	if _, _, err := p.Open(context.Background(), "doc-create", contribapi.LayerData); err != nil {
		t.Errorf("Open(data): %v", err)
	}

	var notFound contribapi.ErrContributionNotFound
	if _, _, err := p.Open(context.Background(), "nonexistent", contribapi.LayerSource); !errors.As(err, &notFound) {
		t.Errorf("Open(nonexistent): want ErrContributionNotFound, got %v", err)
	}

	providertest.Run(t, p)
}

// The task providers — the tasks and their contracts — hold the provider
// contract over a REAL shipped pack (ktw-agents' tasks, several of which have
// input/output contract files), not a fixture that might carry nothing to open.
func TestTaskProviders_HoldTheProviderContract(t *testing.T) {
	long.Test(t)
	root := filepath.Join("..", "..", "..", "..", "app", "ktw-agents")
	ldr := loader.NewEmbedFSLoader(os.DirFS(root), ".")
	for _, p := range TaskProviders("ktw-agents", ldr, "embed/tasks") {
		entries, err := p.Enumerate(context.Background())
		if err != nil || len(entries) == 0 {
			t.Fatalf("%s: Enumerate = %d entries, err %v — the contract would be checked over nothing", p.Type(), len(entries), err)
		}
		providertest.Run(t, p)
	}
}

func TestTaskProviders_TypeAndProvider(t *testing.T) {
	p := TaskProviders("x", loader.NewEmbedFSLoader(fstest.MapFS{}, ""), "embed/tasks")[0]
	if p.Type() != mbapi.ResourceTypeTask {
		t.Errorf("Type() = %q", p.Type())
	}
	if p.Provider() != "x" {
		t.Errorf("Provider() = %q", p.Provider())
	}
}

func TestTaskProviders_NilLoaderErrors(t *testing.T) {
	p := TaskProviders("x", nil, "embed/tasks")[0]
	_, err := p.Enumerate(context.Background())
	if err == nil {
		t.Error("Enumerate with nil Loader should error")
	}
}
