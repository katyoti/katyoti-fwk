// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// exportSender is ActionService answering an export's run: with a run and its
// panel (a long method), or - writing the file at once - with its value.
type exportSender struct {
	out   actionsapi.RunOutput
	write bool
	path  string
	in    actionsapi.RunInput
}

func (s *exportSender) Call(_ context.Context, path string, params any, _ ...ktsapi.CallOption) (json.RawMessage, error) {
	s.path = path
	s.in, _ = params.(actionsapi.RunInput)
	if s.write {
		var r docsapi.RenderInput
		_ = json.Unmarshal(s.in.Input, &r)
		_ = os.WriteFile(r.Output, []byte("%PDF-now"), 0o644)
	}
	return json.Marshal(s.out)
}

func (s *exportSender) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }

func markdownExport(t *testing.T) *exportJob {
	t.Helper()
	job, failed := newExportJob(render.Document{MediaType: render.MediaTypeMarkdown, Title: "note.md", Content: "# Note"}, render.MediaTypePDF, "")
	if job == nil {
		t.Fatalf("no export job: %s", failed.Error)
	}
	return job
}

// TestTheDocumentExportRunsThroughTheActionSurface -: an export
// runs its ktw-docs conversion through ActionService, the document's object
// as the run's target; the long method answers at once with its run and the
// panel it reports into, and the file is read once the run completes; a
// conversion that answers its value gives the document at once; a discarded
// export leaves nothing behind.
func TestTheDocumentExportRunsThroughTheActionSurface(t *testing.T) {
	app := &App{}

	// A long method: the run and its panel answered, the file not yet there.
	job := markdownExport(t)
	s := &exportSender{out: actionsapi.RunOutput{URI: job.method, TaskID: "run-1", Panel: "dp-1"}}
	got := runExport(context.Background(), s, job, "m.note")
	if s.path != "ktw-agents:action/run" {
		t.Errorf("sent to %q, want ActionService.Run", s.path)
	}
	if s.in.URI != "ktw-docs:docs/render" || s.in.Context["_target"] != "m.note" {
		t.Errorf("run input = %+v, want the Render method and the object as _target", s.in)
	}
	var sent docsapi.RenderInput
	if err := json.Unmarshal(s.in.Input, &sent); err != nil || sent.Format != "pdf" || sent.Output != job.outPath {
		t.Errorf("input = %s, want the conversion's input (pdf to the job's output)", s.in.Input)
	}
	if got.Error != "" || got.Panel != "dp-1" || got.TaskID != "run-1" || got.ExportID == "" || got.Document != nil {
		t.Fatalf("a long export's run = %+v, want its run, its panel and an export id, no document yet", got)
	}

	// The run completes: the file it wrote is read as the document to save.
	if err := os.WriteFile(job.outPath, []byte("%PDF-1.7"), 0o644); err != nil {
		t.Fatal(err)
	}
	read := app.DocumentExportRead(got.ExportID)
	if read.Error != "" || read.Document.MediaType != render.MediaTypePDF || read.Document.Title != "note.pdf" || read.Document.Encoding != "base64" {
		t.Fatalf("read = %+v, want the exported pdf, base64", read)
	}
	if b, _ := base64.StdEncoding.DecodeString(read.Document.Content.(string)); string(b) != "%PDF-1.7" {
		t.Errorf("content = %q, want the file the run wrote", b)
	}
	if _, err := os.Stat(job.dir); !os.IsNotExist(err) {
		t.Errorf("the export's folder is kept after its read (%v)", err)
	}
	if again := app.DocumentExportRead(got.ExportID); again.Error == "" {
		t.Errorf("an export read twice must be refused")
	}

	// A conversion that answers its value: the document at once.
	job = markdownExport(t)
	s = &exportSender{out: actionsapi.RunOutput{URI: job.method, Value: json.RawMessage(`{"outputPath":""}`)}, write: true}
	got = runExport(context.Background(), s, job, "")
	if got.Error != "" || got.Panel != "" || got.Document == nil || got.Document.Title != "note.pdf" {
		t.Fatalf("a short export's run = %+v, want the document directly", got)
	}
	if s.in.Context != nil {
		t.Errorf("no object: the run must name no target, got %v", s.in.Context)
	}

	// A run that fails or is cancelled: discarded, its folder removed.
	job = markdownExport(t)
	got = runExport(context.Background(), &exportSender{out: actionsapi.RunOutput{TaskID: "run-2", Panel: "dp-2"}}, job, "")
	app.DocumentExportDiscard(got.ExportID)
	if _, err := os.Stat(job.dir); !os.IsNotExist(err) {
		t.Errorf("a discarded export's folder is kept (%v)", err)
	}
}
