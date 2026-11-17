// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// exportRoute says how a document of one media type becomes another: which
// ktw-docs method converts it, from which file extension, to which format.
type exportRoute struct {
	method string // "render" (Pandoc) or "diagram" (Kroki / draw.io CLI)
	inExt  string // the extension the source file is written with
	format string // the method's output format
}

// exportRoutes is the one table of server-side conversions: source media type
// → target media type → how. An engine declares the formats it offers; those
// it cannot produce in the browser come here.
var exportRoutes = map[string]map[string]exportRoute{
	render.MediaTypeMarkdown: {
		render.MediaTypePDF:  {"render", ".md", "pdf"},
		render.MediaTypeDOCX: {"render", ".md", "docx"},
		render.MediaTypePPTX: {"render", ".md", "pptx"},
		render.MediaTypeHTML: {"render", ".md", "html"},
	},
	render.MediaTypeMermaid: {
		render.MediaTypeSVG: {"diagram", ".mmd", "svg"},
		render.MediaTypePNG: {"diagram", ".mmd", "png"},
		render.MediaTypePDF: {"diagram", ".mmd", "pdf"},
	},
	render.MediaTypeDrawio: {
		render.MediaTypeSVG: {"diagram", ".drawio", "svg"},
		render.MediaTypePNG: {"diagram", ".drawio", "png"},
		render.MediaTypePDF: {"diagram", ".drawio", "pdf"},
	},
}

// exportRouteFor finds the conversion of a document to a target media type.
func exportRouteFor(from, to string) (exportRoute, bool) {
	routes, ok := exportRoutes[render.BaseMediaType(from)]
	if !ok {
		return exportRoute{}, false
	}
	r, ok := routes[render.BaseMediaType(to)]
	return r, ok
}

// exportTitle is the exported file's name: the source title with the target
// media type's extension.
func exportTitle(title, from, to string) string {
	base := strings.TrimSuffix(title, render.ExtensionFor(from))
	if base == "" {
		base = "document"
	}
	return base + render.ExtensionFor(to)
}

// exportJob is one export under way: the folder its input is written to and
// its output read from, the ktw-docs method that converts it, and the
// document it gives once read.
type exportJob struct {
	dir       string
	outPath   string
	method    string // the method's URI: ktw-docs:docs/render or docs/diagram
	call      any
	title     string
	mediaType string
}

// prepareExport opens the document, finds its conversion and writes its
// input to a fresh folder - removed when the job ends (read or discarded).
// A document given with its content and its source is the content as shown
// (it may hold edits); its source only says which folder its relative images
// and diagrams resolve against, since the converter reads a copy.
func (a *App) prepareExport(universeName, nodeID string, doc render.Document, to string) (*exportJob, DocumentOpenResult) {
	baseDir := exportBaseDir(doc.Source, nodeID, func(path string) (string, error) {
		sender, err := a.universeSender(universeName)
		if err != nil {
			return "", err
		}
		return objectFileAbs(a.ctx, sender, a.projectRootsOf(universeName), universeName, nodeID, path)
	})
	if doc.Content != nil {
		doc.Source = nil
	}
	opened := a.DocumentOpen(universeName, nodeID, doc)
	if opened.Error != "" {
		return nil, opened
	}
	return newExportJob(opened.Document, to, baseDir)
}

// exportBaseDir is the folder of the file a document was read from - one of
// its object's module files (located by objectFile), or an absolute path -
// where the relative references of its copy resolve. Empty for a document
// with no file, or one that cannot be located: the converter then resolves
// against the copy's folder.
func exportBaseDir(src *render.Source, nodeID string, objectFile func(path string) (string, error)) string {
	switch {
	case src == nil || src.URI != "" || src.Path == "":
		return ""
	case nodeID != "":
		abs, err := objectFile(src.Path)
		if err != nil || !filepath.IsAbs(abs) {
			return ""
		}
		return filepath.Dir(abs)
	case filepath.IsAbs(src.Path):
		return filepath.Dir(src.Path)
	}
	return ""
}

// newExportJob writes an opened document's input for its conversion to `to`;
// baseDir, when known, is where its relative references resolve.
func newExportJob(src render.Document, to, baseDir string) (*exportJob, DocumentOpenResult) {
	route, ok := exportRouteFor(src.MediaType, to)
	if !ok {
		return nil, DocumentOpenResult{Document: src, Error: fmt.Sprintf("no export from %s to %s", render.BaseMediaType(src.MediaType), to)}
	}
	dir, err := os.MkdirTemp("", "ktw-export-*")
	if err != nil {
		return nil, DocumentOpenResult{Error: err.Error()}
	}
	in := filepath.Join(dir, "document"+route.inExt)
	if err := os.WriteFile(in, []byte(src.Text()), 0o644); err != nil {
		os.RemoveAll(dir)
		return nil, DocumentOpenResult{Error: err.Error()}
	}
	job := &exportJob{
		dir:       dir,
		outPath:   filepath.Join(dir, "export."+route.format),
		method:    "ktw-docs:docs/" + route.method,
		title:     exportTitle(src.Title, src.MediaType, to),
		mediaType: to,
	}
	switch route.method {
	case "render":
		job.call = docsapi.RenderInput{Input: in, Output: job.outPath, Format: route.format, BaseDir: baseDir}
	default:
		job.call = docsapi.DiagramInput{Source: in, Output: job.outPath, Format: route.format}
	}
	return job, DocumentOpenResult{}
}

// read reads the exported file - where the converter's answer says, when it
// names one - as the document to save, and removes the job's folder.
func (j *exportJob) read(answer json.RawMessage) DocumentOpenResult {
	defer j.discard()
	outPath := j.outPath
	// The converter may write elsewhere than asked; its answer says where.
	var said struct {
		OutputPath string `json:"outputPath"`
	}
	if json.Unmarshal(answer, &said) == nil && said.OutputPath != "" {
		outPath = said.OutputPath
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		return DocumentOpenResult{Error: fmt.Sprintf("export to %s: read the result: %v", j.mediaType, err)}
	}
	return DocumentOpenResult{Document: render.Document{
		MediaType: j.mediaType,
		Title:     j.title,
		Content:   base64.StdEncoding.EncodeToString(b),
		Encoding:  "base64",
	}}
}

func (j *exportJob) discard() { os.RemoveAll(j.dir) }

// DocumentExportRunResult is an export run through the action surface: the
// conversion, a long method, answers at once with its run and the display
// panel it reports each step into - drawn where the export was asked, the
// file read by DocumentExportRead(exportId) once the panel completes. A
// conversion that answers its value at once gives the document directly.
type DocumentExportRunResult struct {
	ExportID string           `json:"exportId,omitempty"`
	TaskID   string           `json:"taskId,omitempty"`
	Panel    string           `json:"panel,omitempty"`
	Document *render.Document `json:"document,omitempty"`
	Error    string           `json:"error,omitempty"`
}

// DocumentExportRun converts a document to another media type through
// ktw-docs (Pandoc for markdown; Kroki or the draw.io CLI for diagrams),
// through ActionService: it answers with the run's panel instead of
// waiting for the conversion - no bus fallback, no busy badge while it runs.
// The object the document belongs to is the run's target.
//
// Wails-bound: window.go.main.App.DocumentExportRun(universeName, nodeId, doc, to).
func (a *App) DocumentExportRun(universeName, nodeID string, doc render.Document, to string) DocumentExportRunResult {
	job, failed := a.prepareExport(universeName, nodeID, doc, to)
	if job == nil {
		return DocumentExportRunResult{Error: failed.Error}
	}
	sender, err := a.universeSender(universeName)
	if err != nil {
		job.discard()
		return DocumentExportRunResult{Error: fmt.Sprintf("cannot reach universe %q: %v", universeName, err)}
	}
	ctx, cancel := a.actionRunContext(universeName, job.method)
	defer cancel()
	return runExport(ctx, sender, job, nodeID)
}

// runExport runs the job's conversion through ActionService.Run: a run with
// its panel keeps the job until DocumentExportRead or DocumentExportDiscard;
// a value is the converted file, read at once.
func runExport(ctx context.Context, sender ktsp.Sender, job *exportJob, nodeID string) DocumentExportRunResult {
	raw, err := json.Marshal(job.call)
	if err != nil {
		job.discard()
		return DocumentExportRunResult{Error: err.Error()}
	}
	in := actionsapi.RunInput{URI: job.method, Input: raw}
	if nodeID != "" {
		in.Context = map[string]any{"_target": nodeID}
	}
	r := runActionCall(ctx, sender, job.method, in)
	switch {
	case r.Error != "":
		job.discard()
		return DocumentExportRunResult{Error: fmt.Sprintf("export to %s: %s", job.mediaType, r.Error)}
	case r.Panel != "":
		return DocumentExportRunResult{ExportID: exportJobs.keep(job), TaskID: r.TaskID, Panel: r.Panel}
	}
	read := job.read(json.RawMessage(r.Value))
	if read.Error != "" {
		return DocumentExportRunResult{Error: read.Error}
	}
	return DocumentExportRunResult{Document: &read.Document}
}

// DocumentExportRead reads the file of an export whose run completed, as the
// document to save, and ends the export.
//
// Wails-bound: window.go.main.App.DocumentExportRead(exportId).
func (a *App) DocumentExportRead(exportID string) DocumentOpenResult {
	job := exportJobs.take(exportID)
	if job == nil {
		return DocumentOpenResult{Error: fmt.Sprintf("no export %q under way", exportID)}
	}
	return job.read(nil)
}

// DocumentExportDiscard ends an export whose run failed or was cancelled;
// one already ended is ignored.
//
// Wails-bound: window.go.main.App.DocumentExportDiscard(exportId).
func (a *App) DocumentExportDiscard(exportID string) {
	if job := exportJobs.take(exportID); job != nil {
		job.discard()
	}
}

// exportJobs holds the exports whose run is under way, by id.
var exportJobs = &exportRegistry{jobs: map[string]*exportJob{}}

type exportRegistry struct {
	mu   sync.Mutex
	next int
	jobs map[string]*exportJob
}

func (r *exportRegistry) keep(j *exportJob) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	id := fmt.Sprintf("export-%d", r.next)
	r.jobs[id] = j
	return id
}

func (r *exportRegistry) take(id string) *exportJob {
	r.mu.Lock()
	defer r.mu.Unlock()
	j := r.jobs[id]
	delete(r.jobs, id)
	return j
}
